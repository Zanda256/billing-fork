package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/billing"
	"github.com/doujins-org/doujins-billing/internal/manager/api/dead-letter"
	"github.com/doujins-org/doujins-billing/internal/manager/api/deduplication"
	"github.com/doujins-org/doujins-billing/internal/manager/api/lifecycle"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/api/types"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"github.com/doujins-org/doujins-billing/pkg/nmi"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const NMIProcessorName string = "NMI"

type NMIWebhookService struct {
	DB                           *db.DB
	PriceService                 *price.PriceService
	ProductService *product.ProductService
	Data           types.NMIWebhookEvent
	Provider       string
	DeadLetterService            *dead_letter.DeadLetterService
	NMIClient                    *nmi.NMIClient
	BillingEventService          *billing.BillingEventService
	SubscriptionService          *subscription.SubscriptionService
	PaymentService               *payment.PaymentService
	DeduplicationService         *deduplication.DeduplicationService
	NotificationQueueService     *notification.NotificationQueueService
	SubscriptionLifecycleService *lifecycle.SubscriptionLifecycleService
}

type NMIWebhookEventType = string

const (
	// Subscription lifecycle events
	EventTypeNMIAddSubscription    NMIWebhookEventType = "recurring.subscription.add"
	EventTypeNMIUpdateSubscription NMIWebhookEventType = "recurring.subscription.update"
	EventTypeNMIDeleteSubscription NMIWebhookEventType = "recurring.subscription.delete"

	// Transaction events
	EventTypeNMITransactionSuccess NMIWebhookEventType = "transaction.sale.success"
	EventTypeNMITransactionFailure NMIWebhookEventType = "transaction.sale.failure"

	// Automatic Card Updater (ACU) events
	EventTypeNMIACUUpdated         NMIWebhookEventType = "acu.summary.automaticallyupdated"
	EventTypeNMIACUContactCustomer NMIWebhookEventType = "acu.summary.contactcustomer"
	EventTypeNMIACUClosedAccount   NMIWebhookEventType = "acu.summary.closedaccount"

	// Chargeback events
	EventTypeNMIChargebackComplete NMIWebhookEventType = "chargeback.batch.complete"
)

func (s *NMIWebhookService) parseRecurringEventBody() (*types.NMIRecurringEventBody, error) {
	var body types.NMIRecurringEventBody
	if err := json.Unmarshal(s.Data.EventBody, &body); err != nil {
		return nil, fmt.Errorf("failed to parse recurring event body: %w", err)
	}
	return &body, nil
}

func (s *NMIWebhookService) parseTransactionEventBody() (*types.NMITransactionEventBody, error) {
	var body types.NMITransactionEventBody
	if err := json.Unmarshal(s.Data.EventBody, &body); err != nil {
		return nil, fmt.Errorf("failed to parse transaction event body: %w", err)
	}
	return &body, nil
}

func (s *NMIWebhookService) parseACUEventBody() (*types.NMIACUEventBody, error) {
	var body types.NMIACUEventBody
	if err := json.Unmarshal(s.Data.EventBody, &body); err != nil {
		return nil, fmt.Errorf("failed to parse ACU event body: %w", err)
	}
	return &body, nil
}

func transactionSubscriptionID(body *types.NMITransactionEventBody) string {
	if body == nil {
		return ""
	}

	candidates := []string{}
	if body.Subscription != nil {
		candidates = append(candidates, body.Subscription.SubscriptionID.Trimmed())
	}
	if body.TransactionDetail != nil && body.TransactionDetail.Subscription != nil {
		candidates = append(candidates, body.TransactionDetail.Subscription.SubscriptionID.Trimmed())
	}
	candidates = append(candidates, body.OrderID.Trimmed())
	if body.TransactionDetail != nil {
		candidates = append(candidates, body.TransactionDetail.OrderID.Trimmed())
	}
	candidates = append(candidates, body.PONumber.Trimmed())
	if body.TransactionDetail != nil {
		candidates = append(candidates, body.TransactionDetail.PONumber.Trimmed())
	}
	candidates = append(candidates, body.CustomerID.Trimmed())
	if body.TransactionDetail != nil {
		candidates = append(candidates, body.TransactionDetail.CustomerID.Trimmed())
	}

	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}

	return ""
}

func transactionActionSource(body *types.NMITransactionEventBody) string {
	if body == nil {
		return ""
	}
	if body.Action != nil && body.Action.Source != "" {
		return strings.ToLower(strings.TrimSpace(body.Action.Source))
	}
	if body.TransactionDetail != nil && body.TransactionDetail.Action != nil && body.TransactionDetail.Action.Source != "" {
		return strings.ToLower(strings.TrimSpace(body.TransactionDetail.Action.Source))
	}
	return ""
}

func isRecurringSource(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "recurring", "retry":
		return true
	default:
		return false
	}
}

func transactionPlanID(body *types.NMITransactionEventBody) string {
	if body == nil {
		return ""
	}
	if body.Subscription != nil && !body.Subscription.PlanID.IsEmpty() {
		return body.Subscription.PlanID.Trimmed()
	}
	if body.TransactionDetail != nil && body.TransactionDetail.Subscription != nil {
		return body.TransactionDetail.Subscription.PlanID.Trimmed()
	}
	return ""
}

func transactionAmount(body *types.NMITransactionEventBody) (float64, error) {
	if body == nil {
		return 0, fmt.Errorf("transaction body is nil")
	}
	if amt, err := body.Amount.Float64(); err == nil {
		return amt, nil
	}
	if body.TransactionDetail != nil {
		if amt, err := body.TransactionDetail.Amount.Float64(); err == nil {
			return amt, nil
		}
	}
	return 0, fmt.Errorf("amount not provided")
}

func transactionCurrency(body *types.NMITransactionEventBody) string {
	if body == nil {
		return ""
	}
	if curr := body.Currency.Trimmed(); curr != "" {
		return curr
	}
	if body.TransactionDetail != nil {
		return body.TransactionDetail.Currency.Trimmed()
	}
	return ""
}

type NMIBillingError struct {
	Type    string                 `json:"type"`
	Message string                 `json:"message"`
	Context map[string]interface{} `json:"context"`
	Err     error                  `json:"-"`
}

func (be *NMIBillingError) Error() string {
	if be.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", be.Type, be.Message, be.Err)
	}
	return fmt.Sprintf("%s: %s", be.Type, be.Message)
}

func (be *NMIBillingError) Unwrap() error {
	return be.Err
}

const (
	ErrorTypeNMIValidation    = "validation_error"
	ErrorTypeNMIAmount        = "amount_mismatch"
	ErrorTypeNMIDuplicate     = "duplicate_transaction"
	ErrorTypeNMIStatusChange  = "invalid_status_change"
	ErrorTypeNMIBusinessLogic = "business_logic_error"
	ErrorTypeNMIDatabase      = "db_error"
	ErrorTypeNMINotFound      = "not_found"
)

func newNMIBillingError(errorType string, message string, context map[string]interface{}, err error) *NMIBillingError {
	return &NMIBillingError{
		Type:    errorType,
		Message: message,
		Context: context,
		Err:     err,
	}
}

func (s *NMIWebhookService) HandleNMIWebhook(ctx context.Context) error {
	// Use deduplication service if available
	if s.DeduplicationService != nil {
		return s.DeduplicationService.ProcessWebhook(
			ctx,
			s.Data.EventID,
			s.Data.EventType,
			models2.ProcessorNMI,
			s.Data,
			s.handleWebhook,
		)
	}

	return s.handleWebhook(ctx)
}


func (s *NMIWebhookService) handleWebhook(ctx context.Context) error {
	switch s.Data.EventType {
	// Subscription lifecycle events
	case types.EventTypeNMIAddSubscription:
		return s.handleAddSubscription(ctx)
	case types.EventTypeNMIUpdateSubscription:
		return s.handleUpdateSubscription(ctx)
	case types.EventTypeNMIDeleteSubscription:
		return s.handleDeleteSubscription(ctx)
	case types.EventTypeNMITransactionSuccess:
		return s.handleTransactionSaleSuccess(ctx)
	case types.EventTypeNMITransactionFailure:
		return s.handleTransactionSaleFailure(ctx)
	case types.EventTypeNMIACUUpdated, types.EventTypeNMIACUContactCustomer, types.EventTypeNMIACUClosedAccount:
		return s.handleACUEvent(ctx)

	case types.EventTypeNMIChargebackComplete:
		return s.handleChargebackComplete(ctx)

	default:
		// Log unknown event to dead letter queue if service is available
		if s.DeadLetterService != nil {
			dataJSON, err := json.Marshal(s.Data)
			if err == nil {
				s.DeadLetterService.LogUnknownEvent(ctx, "nmi", s.Data.EventType, json.RawMessage(dataJSON), nil, "")
			}
		}
		return fmt.Errorf("unsupported event type: %s", s.Data.EventType)
	}
}

func (s *NMIWebhookService) handleAddSubscription(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing NMI subscription add notification")

	body, err := s.parseRecurringEventBody()
	if err != nil {
		return err
	}

	nmiSubID := body.SubscriptionID.Trimmed()
	if nmiSubID == "" {
		return newNMIBillingError(ErrorTypeNMIValidation, "Missing subscription ID", map[string]interface{}{}, nil)
	}

	provider := strings.TrimSpace(strings.ToLower(s.Provider))
	if provider == "" {
		provider = "mobius"
	}

	var nmiPlanID string
	if body.Plan != nil {
		nmiPlanID = body.Plan.ID.Trimmed()
	}
	if nmiPlanID == "" {
		return newNMIBillingError(ErrorTypeNMIValidation, "Missing plan ID", map[string]interface{}{
			"subscription_id": nmiSubID,
		}, nil)
	}

	price, err := s.PriceService.GetByNMIPlan(ctx, provider, nmiPlanID)
	if err != nil {
		return fmt.Errorf("failed to find price for NMI plan ID %s: %w", nmiPlanID, err)
	}

	subscription, err := s.SubscriptionService.GetByProcessorSubscriptionID(ctx, subscription.ProcessorNMI, provider, nmiSubID)
	if err != nil {
		return fmt.Errorf("failed to load subscription for processor ID %s: %w", nmiSubID, err)
	}

	if subscription.Status != models2.StatusPending {
		log.WithContext(ctx).
			WithField("subscription_id", subscription.ID).
			WithField("processor_subscription_id", nmiSubID).
			Info("Subscription already activated; skipping add event")
		return nil
	}

	_, err = s.SubscriptionLifecycleService.CreateMembership(ctx, &lifecycle.CreateMembershipParams{
		PriceID:                 price.ID,
		UserID:                  subscription.UserID,
		Processor:               models2.ProcessorNMI,
		ProcessorSubscriptionID: &subscription.ProcessorSubscriptionID,
		UserEmail:               subscription.UserEmail,
		ProcessorProvider:       provider,
	})
	if err != nil {
		return fmt.Errorf("failed to create membership: %w", err)
	}

	return nil
}

func (s *NMIWebhookService) handleUpdateSubscription(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing NMI subscription update notification")

	body, err := s.parseRecurringEventBody()
	if err != nil {
		return err
	}

	nmiSubID := body.SubscriptionID.Trimmed()
	if nmiSubID == "" {
		return newNMIBillingError(ErrorTypeNMIValidation, "Missing subscription ID", map[string]interface{}{}, nil)
	}

	var nmiPlanID string
	if body.Plan != nil {
		nmiPlanID = body.Plan.ID.Trimmed()
	}

	fields := log.Fields{
		"subscription_id": nmiSubID,
	}
	if nmiPlanID != "" {
		fields["plan_id"] = nmiPlanID
	}
	if body.NextChargeDate.Trimmed() != "" {
		fields["next_charge_date"] = body.NextChargeDate.Trimmed()
	}

	log.WithContext(ctx).WithFields(fields).Info("NMI subscription metadata updated; no action required")
	return nil
}

func (s *NMIWebhookService) handleDeleteSubscription(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing NMI subscription delete notification")

	body, err := s.parseRecurringEventBody()
	if err != nil {
		return err
	}

	provider := strings.TrimSpace(strings.ToLower(s.Provider))
	if provider == "" {
		provider = "mobius"
	}

	nmiSubID := body.SubscriptionID.Trimmed()
	if nmiSubID == "" {
		return newNMIBillingError(ErrorTypeNMIValidation, "Missing subscription ID", map[string]interface{}{}, nil)
	}

	subscription, err := s.SubscriptionService.GetByProcessorSubscriptionID(ctx, string(models2.ProcessorNMI), provider, nmiSubID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.WithContext(ctx).
				WithField("processor_subscription_id", nmiSubID).
				Warn("Received NMI delete for unknown subscription; ignoring")
			return nil
		}
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	if subscription.Status == models2.StatusCancelled {
		log.WithContext(ctx).
			WithField("subscription_id", subscription.ID).
			WithField("processor_subscription_id", nmiSubID).
			Info("Subscription already cancelled; skipping NMI delete event")
		return nil
	}

	cancelFeedback := "Cancelled via NMI webhook"
	processor := models2.ProcessorNMI
	if err := s.SubscriptionLifecycleService.CancelMembership(ctx, &lifecycle.CancelMembershipParams{
		ImmediateCancellation:   false,
		Processor:               &processor,
		ProcessorSubscriptionID: &nmiSubID,
		CancelFeedback:          &cancelFeedback,
		SubscriptionID:          &subscription.ID,
		CancelType:              models2.CancelTypeMerchant,
		ProcessorProvider:       provider,
	}); err != nil {
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}

	return nil
}

// handleChargebackComplete processes chargeback batch completion notifications
func (s *NMIWebhookService) handleTransactionSaleSuccess(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing NMI transaction success notification")

	body, err := s.parseTransactionEventBody()
	if err != nil {
		return err
	}

	provider := strings.TrimSpace(strings.ToLower(s.Provider))
	if provider == "" {
		provider = "mobius"
	}

	nmiSubID := transactionSubscriptionID(body)
	if nmiSubID == "" {
		return newNMIBillingError(ErrorTypeNMIValidation, "Missing subscription reference", map[string]interface{}{}, nil)
	}

	actionSource := transactionActionSource(body)
	if !isRecurringSource(actionSource) {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"subscription_reference": nmiSubID,
				"action_source":          actionSource,
				"event_type":             s.Data.EventType,
			}).Info("Ignoring NMI transaction success without recurring source")
		return nil
	}

	subscription, err := s.SubscriptionService.GetByProcessorSubscriptionID(ctx, subscription.ProcessorNMI, provider, nmiSubID)
	if err != nil {
		return fmt.Errorf("failed to load subscription for transaction event: %w", err)
	}

	amount, amountErr := transactionAmount(body)
	currency := transactionCurrency(body)
	txnID := body.TransactionID.Trimmed()
	if txnID == "" && body.TransactionDetail != nil {
		txnID = body.TransactionDetail.TransactionID.Trimmed()
	}

	processed := false

	// Activate or renew subscription based on current status
	switch subscription.Status {
	case models2.StatusPending:
		_, err = s.SubscriptionLifecycleService.CreateMembership(ctx, &lifecycle.CreateMembershipParams{
			PriceID:                 subscription.PriceID,
			UserID:                  subscription.UserID,
			Processor:               models2.ProcessorNMI,
			ProcessorSubscriptionID: &subscription.ProcessorSubscriptionID,
			UserEmail:               subscription.UserEmail,
			ProcessorProvider:       provider,
		})
		if err != nil {
			return fmt.Errorf("failed to activate subscription: %w", err)
		}
		processed = true
	default:
		if err := s.SubscriptionLifecycleService.RenewMembership(ctx, &lifecycle.RenewMembershipParams{
			Processor:               models2.ProcessorNMI,
			ProcessorSubscriptionID: nmiSubID,
			ProcessorProvider:       provider,
		}); err != nil {
			return fmt.Errorf("failed to renew subscription: %w", err)
		}
		processed = true
	}

	if !processed {
		return nil
	}

	// Persist payment record if available
	if s.PaymentService != nil && txnID != "" {
		existing, err := s.PaymentService.GetByTransactionID(ctx, models2.ProcessorNMI, txnID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to check existing payment: %w", err)
		}
		if err == nil && existing != nil {
			log.WithContext(ctx).
				WithFields(log.Fields{
					"transaction_id":  txnID,
					"subscription_id": subscription.ID,
				}).Debug("NMI payment already recorded; skipping duplicate entry")
		} else {
			amountValue := amount
			if amountErr != nil {
				log.WithContext(ctx).
					WithField("transaction_id", txnID).
					WithError(amountErr).
					Warn("Failed to parse NMI transaction amount; falling back to price amount")
				if subscription.Price != nil && subscription.Price.Amount > 0 {
					amountValue = subscription.Price.Amount
				} else {
					amountValue = 0
				}
			}

			if amountValue > 0 {
				currencyValue := currency
				if strings.TrimSpace(currencyValue) == "" {
					if subscription.Price != nil && strings.TrimSpace(subscription.Price.Currency) != "" {
						currencyValue = subscription.Price.Currency
					} else {
						currencyValue = "USD"
					}
				}

				var providerPtr *string
				if provider != "" {
					p := provider
					providerPtr = &p
				}

				payment := &models2.Payment{
					ID:                uuid.New(),
					UserID:            subscription.UserID,
					PriceID:           subscription.PriceID,
					SubscriptionID:    &subscription.ID,
					Processor:         models2.ProcessorNMI,
					ProcessorProvider: providerPtr,
					TransactionID:     txnID,
					Amount:            amountValue,
					Currency:          currencyValue,
					PurchasedAt:       time.Now().UTC(),
					CreatedAt:         time.Now().UTC(),
				}

				if err := s.PaymentService.Create(ctx, payment); err != nil {
					log.WithContext(ctx).
						WithError(err).
						WithFields(log.Fields{
							"transaction_id":  txnID,
							"subscription_id": subscription.ID,
						}).Error("Failed to record NMI payment entry")
				}
			} else {
				log.WithContext(ctx).
					WithField("transaction_id", txnID).
					Warn("Skipping payment record for NMI transaction due to missing amount")
			}
		}
	}

	if s.BillingEventService != nil {
		metadata := map[string]interface{}{
			"event_type":      s.Data.EventType,
			"action_source":   actionSource,
			"previous_status": subscription.Status,
			"provider":        provider,
		}
		if txnID != "" {
			metadata["transaction_id"] = txnID
		}
		if planID := transactionPlanID(body); planID != "" {
			metadata["plan_id"] = planID
		}
		if body.CustomerID.Trimmed() != "" {
			metadata["customer_id"] = body.CustomerID.Trimmed()
		}
		if body.CustomerVaultID.Trimmed() != "" {
			metadata["customer_vault_id"] = body.CustomerVaultID.Trimmed()
		}
		if ord := body.OrderID.Trimmed(); ord != "" {
			metadata["order_id"] = ord
		}

		var amountPtr *float64
		if amountErr == nil {
			amountPtr = &amount
		}

		paymentEvent := billing.PaymentEventData{
			EventID:        uuid.New(),
			SubscriptionID: &subscription.ID,
			UserID:         subscription.UserID,
			EventType:      "charge_success",
			Processor:      "nmi",
			Amount:         amountPtr,
			Currency:       currency,
			BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{}),
			WebhookSource:  "webhook",
			Metadata:       billing.CreateMetadataJSON(metadata),
			Timestamp:      time.Now().UTC(),
		}
		if txnID != "" {
			paymentEvent.ProcessorTransactionID = &txnID
		}
		if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEvent); err != nil {
			log.WithContext(ctx).WithError(err).Error("Failed to log NMI payment event")
		}
	}

	return nil
}

func (s *NMIWebhookService) handleTransactionSaleFailure(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Warn("Processing NMI transaction failure notification")

	body, err := s.parseTransactionEventBody()
	if err != nil {
		return err
	}

	provider := strings.TrimSpace(strings.ToLower(s.Provider))
	if provider == "" {
		provider = "mobius"
	}

	nmiSubID := transactionSubscriptionID(body)
	if nmiSubID == "" {
		return newNMIBillingError(ErrorTypeNMIValidation, "Missing subscription reference", map[string]interface{}{}, nil)
	}

	actionSource := transactionActionSource(body)
	if !isRecurringSource(actionSource) {
		log.WithContext(ctx).
			WithFields(log.Fields{
				"subscription_reference": nmiSubID,
				"action_source":          actionSource,
				"event_type":             s.Data.EventType,
			}).Info("Ignoring NMI transaction failure without recurring source")
		return nil
	}

	var (
		subscription *models2.Subscription
		fetchErr     error
	)
	if s.SubscriptionService != nil {
		subscription, fetchErr = s.SubscriptionService.GetByProcessorSubscriptionID(ctx, subscription.ProcessorNMI, provider, nmiSubID)
		if fetchErr != nil && !errors.Is(fetchErr, sql.ErrNoRows) {
			return fmt.Errorf("failed to load subscription for transaction event: %w", fetchErr)
		}
		if errors.Is(fetchErr, sql.ErrNoRows) {
			log.WithContext(ctx).
				WithField("processor_subscription_id", nmiSubID).
				Warn("Received NMI failure event for unknown subscription; ignoring")
			return nil
		}
	}

	var failureReason, failureCode *string
	if body.Action != nil {
		if txt := strings.TrimSpace(body.Action.ResponseText); txt != "" {
			failureReason = &txt
		}
		if code := strings.TrimSpace(body.Action.ResponseCode.String()); code != "" {
			failureCode = &code
		}
	} else if body.TransactionDetail != nil && body.TransactionDetail.Action != nil {
		if txt := strings.TrimSpace(body.TransactionDetail.Action.ResponseText); txt != "" {
			failureReason = &txt
		}
		if code := strings.TrimSpace(body.TransactionDetail.Action.ResponseCode.String()); code != "" {
			failureCode = &code
		}
	}

	if err := s.SubscriptionLifecycleService.FailMembership(ctx, &lifecycle.FailMembershipParams{
		Processor:               models2.ProcessorNMI,
		ProcessorSubscriptionID: nmiSubID,
		FailureReason:           failureReason,
		FailureCode:             failureCode,
		ProcessorProvider:       provider,
	}); err != nil {
		return fmt.Errorf("failed to mark subscription as failed: %w", err)
	}

	if s.BillingEventService != nil && subscription != nil {
		metadata := map[string]interface{}{
			"event_type":      s.Data.EventType,
			"previous_status": subscription.Status,
			"provider":        provider,
		}
		if failureReason != nil && *failureReason != "" {
			metadata["failure_reason"] = *failureReason
		}
		if failureCode != nil && *failureCode != "" {
			metadata["failure_code"] = *failureCode
		}
		if txnID := body.TransactionID.Trimmed(); txnID != "" {
			metadata["transaction_id"] = txnID
		}

		var amountPtr *float64
		if amt, amtErr := transactionAmount(body); amtErr == nil {
			amountPtr = &amt
		}

		paymentEvent := billing.PaymentEventData{
			EventID:        uuid.New(),
			SubscriptionID: &subscription.ID,
			UserID:         subscription.UserID,
			EventType:      "charge_failure",
			Processor:      "nmi",
			Amount:         amountPtr,
			Currency:       transactionCurrency(body),
			BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{}),
			WebhookSource:  "webhook",
			Metadata:       billing.CreateMetadataJSON(metadata),
			Timestamp:      time.Now().UTC(),
		}
		if txnID := body.TransactionID.Trimmed(); txnID != "" {
			paymentEvent.ProcessorTransactionID = &txnID
		}
		if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEvent); err != nil {
			log.WithContext(ctx).WithError(err).Error("Failed to log NMI payment failure event")
		}
	} else if subscription == nil {
		log.WithContext(ctx).
			WithField("processor_subscription_id", nmiSubID).
			Warn("Unable to log NMI payment failure event because subscription record was not found")
	}

	return nil
}

func (s *NMIWebhookService) handleACUEvent(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing NMI ACU notification")

	body, err := s.parseACUEventBody()
	if err != nil {
		return err
	}

	vaultID := body.VaultID.Trimmed()
	fields := log.Fields{"vault_id": vaultID}
	if body.Subscription != nil && !body.Subscription.SubscriptionID.IsEmpty() {
		fields["subscription_id"] = body.Subscription.SubscriptionID.Trimmed()
	}
	if body.PaymentMethod != nil {
		fields["card_last4"] = body.PaymentMethod.LastFour.Trimmed()
		fields["card_type"] = body.PaymentMethod.CardType.Trimmed()
		fields["expiry"] = body.PaymentMethod.ExpiryDate.Trimmed()
	}

	log.WithContext(ctx).WithFields(fields).Info("Received NMI ACU event (no automatic vault update configured)")
	return nil
}

func (s *NMIWebhookService) handleChargebackComplete(ctx context.Context) error {
	// Chargeback events are typically batch operations containing multiple disputes
	// For now, we'll just log them for administrative purposes

	// Log chargeback batch completion to ClickHouse
	if s.BillingEventService != nil {
		metadata := map[string]interface{}{
			"batch_type":   "chargeback",
			"event_source": "nmi",
			"batch_status": "completed",
		}

		chargebackEventData := billing.ChargebackEventData{
			EventID:   uuid.New(),
			EventType: "batch_processed",
			Processor: "nmi",
			BatchID:   s.Data.EventID, // Use event ID as batch identifier
			Status:    "completed",
			Metadata:  billing.CreateMetadataJSON(metadata),
			Timestamp: time.Now(),
		}

		if err := s.BillingEventService.LogChargebackEvent(ctx, chargebackEventData); err != nil {
			log.WithError(err).Error("Failed to log chargeback batch event to ClickHouse")
		}
	}

	log.WithContext(ctx).WithFields(log.Fields{
		"eventID":   s.Data.EventID,
		"eventType": s.Data.EventType,
	}).Info("Chargeback batch processing completed")

	return nil
}
