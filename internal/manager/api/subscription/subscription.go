package subscription

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/email"
	"github.com/doujins-org/doujins-billing/internal/manager/api/idempotency"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/api/user"
	"github.com/doujins-org/doujins-billing/internal/manager/api/vault"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/ccbill"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"github.com/doujins-org/doujins-billing/pkg/nmi"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type SubscribeData struct {
	Email           string `json:"email"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Address1        string `json:"address1"`
	City            string `json:"city"`
	State           string `json:"state"`
	Zip             string `json:"zip"`
	Country         string `json:"country"`
	PriceID         string `json:"price_id"`
	Processor       string `json:"processor"`
	Provider        string `json:"provider,omitempty"`
	PaymentToken    string `json:"payment_token,omitempty"`
	PaymentMethodID string `json:"payment_method_id,omitempty"`
}

type GetSubscriptionsFilters struct {
	UserID    string    `form:"user_id"`
	Status    string    `form:"status"`
	PriceID   uuid.UUID `form:"price_id"`
	Processor string    `form:"processor"`
}

type SubscriptionService struct {
	subscriptionRepo         *repo.SubscriptionRepo
	PriceService             *price.PriceService
	ProductService           *product.ProductService
	NotificationQueueService *notification.NotificationQueueService
	CCBillRESTClient         *ccbill.RESTClient
	NMIClients               map[string]*nmi.NMIClient
	PaymentMethodService     *payment.PaymentMethodService
	VaultService             *vault.VaultService
	IdempotencyService       *idempotency.IdempotencyService
}

func (s *SubscriptionService) nmiClientForProvider(provider string) (*nmi.NMIClient, error) {
	providerKey := strings.TrimSpace(strings.ToLower(provider))
	if providerKey == "" {
		providerKey = "mobius"
	}
	if s.NMIClients == nil {
		return nil, fmt.Errorf("nmi provider '%s' is not configured", providerKey)
	}
	client, ok := s.NMIClients[providerKey]
	if !ok {
		return nil, fmt.Errorf("nmi provider '%s' is not configured", providerKey)
	}
	return client, nil
}

type PaymentProcessor = int

const (
	ProcessorCCBill = "ccbill"
	ProcessorNMI    = "nmi"

	CurrencyUSD = "USD"
	CurrencyEUR = "EUR"

	BillingCycleMonthly = 30

	WebhookSourceCCBill = "ccbill_webhook"
	WebhookSourceNMI    = "nmi_webhook"
	WebhookSourceSystem = "system"

	EventReasonSubscriptionExpired        = "subscription_expired"
	EventReasonSubscriptionDeletedWebhook = "subscription_deleted_via_webhook"
	EventReasonPaymentDeclined            = "payment_declined"

	StatusMessagePending = "pending"
)

const (
	CCBill PaymentProcessor = iota
	NMI
)

var (
	PaymentProcessors = map[string]PaymentProcessor{
		ProcessorCCBill: CCBill,
		ProcessorNMI:    NMI,
	}
)

type SubscribeResponse struct {
	URL            string `json:"url,omitempty"`
	Status         string `json:"status,omitempty"`
	Message        string `json:"message,omitempty"`
	SubscriptionID string `json:"subscription_id,omitempty"`
}

func (s *SubscriptionService) Subscribe(ctx context.Context, data *SubscribeData, user *user.UserIdentity) (any, error) {
	priceID, err := uuid.Parse(data.PriceID)
	if err != nil {
		return nil, fmt.Errorf("invalid price ID: %w", err)
	}

	price, err := s.PriceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("price not found: %w", err)
	}
	if !price.IsActive {
		return nil, errors.New("price is not available for purchase")
	}
	if s.ProductService != nil {
		product, err := s.ProductService.GetByID(ctx, price.ProductID)
		if err != nil {
			return nil, fmt.Errorf("product not found: %w", err)
		}
		if !product.IsActive {
			return nil, errors.New("product is not available for purchase")
		}
	}

	existingSub, err := s.GetByUserID(ctx, user.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing subscription: %w", err)
	}

	if existingSub != nil && existingSub.Status == models2.StatusActive {
		return nil, errors.New("user already has an active subscription")
	}

	processor := data.Processor
	if processor == "" {
		processor = ProcessorCCBill
	}

	switch processor {
	case ProcessorCCBill:
		// CCBill now uses FlexForm integration instead of direct payment processing
		return map[string]any{
			"status":  "redirect_required",
			"message": "CCBill payments now use FlexForm integration",
			"instructions": map[string]string{
				"step1": "Generate FlexForm URL using POST /v1/subscriptions/ccbill/flexform-url",
				"step2": "Embed the returned iframe_url in your frontend",
				"step3": "User completes payment in the embedded CCBill form",
				"step4": "Subscription will be activated via webhook upon successful payment",
			},
			"flexform_endpoint": "/v1/subscriptions/ccbill/flexform-url",
		}, nil
	case ProcessorNMI:
		provider := strings.TrimSpace(strings.ToLower(data.Provider))
		if price.NMIProvider != nil {
			expected := strings.TrimSpace(strings.ToLower(*price.NMIProvider))
			if provider != "" && provider != expected {
				return nil, fmt.Errorf("provider %s does not match price provider %s", provider, expected)
			}
			provider = expected
		}

		if provider == "" {
			provider = "mobius"
		}

		client, err := s.nmiClientForProvider(provider)
		if err != nil {
			return nil, err
		}

		if price.NMIPlanID == nil || strings.TrimSpace(*price.NMIPlanID) == "" {
			return nil, fmt.Errorf("price %s is missing an NMI plan configuration", price.ID)
		}

		email := strings.TrimSpace(data.Email)

		subscriptionID := uuid.New()

		var customerVaultID string
		var createdPaymentMethod *models2.PaymentMethod
		paymentMethodID := strings.TrimSpace(data.PaymentMethodID)
		if paymentMethodID != "" {
			if s.PaymentMethodService == nil {
				return nil, errors.New("payment method service unavailable")
			}

			pmID, err := uuid.Parse(paymentMethodID)
			if err != nil {
				return nil, fmt.Errorf("invalid payment method ID: %w", err)
			}

			paymentMethod, err := s.PaymentMethodService.ValidatePaymentMethodOperation(ctx, pmID, user.ID)
			if err != nil {
				return nil, fmt.Errorf("unable to use saved payment method: %w", err)
			}

			if paymentMethod.Processor != models2.ProcessorNMI {
				return nil, errors.New("saved payment method is not compatible with NMI subscriptions")
			}

			customerVaultID = paymentMethod.VaultID
			if paymentMethod.Provider != nil {
				provider = strings.TrimSpace(strings.ToLower(*paymentMethod.Provider))
			}
		}

		trimmedToken := strings.TrimSpace(data.PaymentToken)
		if trimmedToken == "" && customerVaultID == "" {
			return nil, errors.New("payment token or payment method ID is required")
		}

		var idemReq *models2.IdempotencyRequest
		if s.IdempotencyService != nil {
			key := buildNMIIdempotencyKey(user.ID, priceID, trimmedToken, paymentMethodID)
			req, exists, err := s.IdempotencyService.Begin(ctx, "nmi_subscription", key, &user.ID)
			if err != nil {
				return nil, fmt.Errorf("start idempotency window: %w", err)
			}
			if exists {
				return nil, errors.New("duplicate subscription request detected")
			}
			idemReq = req
		}

		params := nmi.RecurringPaymentData{
			CardUserData: nmi.CardUserData{
				FirstName: data.FirstName,
				LastName:  data.LastName,
				Address1:  data.Address1,
				City:      data.City,
				State:     data.State,
				Zip:       data.Zip,
				Country:   data.Country,
			},
			PlanID:     strings.TrimSpace(*price.NMIPlanID),
			Amount:     price.Amount,
			Currency:   price.Currency,
			Email:      email,
			OrderID:    subscriptionID.String(),
			PONumber:   subscriptionID.String(),
			CustomerID: user.ID,
		}

		if strings.TrimSpace(params.FirstName) == "" {
			params.FirstName = user.Username
		}
		if strings.TrimSpace(params.LastName) == "" {
			params.LastName = "Member"
		}
		if strings.TrimSpace(params.Address1) == "" {
			params.Address1 = "N/A"
		}
		if strings.TrimSpace(params.City) == "" {
			params.City = "N/A"
		}
		if strings.TrimSpace(params.State) == "" {
			params.State = "N/A"
		}
		if strings.TrimSpace(params.Zip) == "" {
			params.Zip = "00000"
		}
		if strings.TrimSpace(params.Country) == "" {
			params.Country = "US"
		}

		if trimmedToken != "" && customerVaultID == "" {
			if s.VaultService == nil {
				return nil, errors.New("vault service unavailable")
			}

			vaultReq := &vault.CreateVaultRequest{
				PaymentToken: trimmedToken,
				Provider:     provider,
				FirstName:    params.FirstName,
				LastName:     params.LastName,
				Address1:     params.Address1,
				City:         params.City,
				State:        params.State,
				Zip:          params.Zip,
				Country:      params.Country,
				Email:        email,
			}

			pm, err := s.VaultService.CreateVault(ctx, user, vaultReq)
			if err != nil {
				return nil, fmt.Errorf("failed to create payment method: %w", err)
			}

			customerVaultID = strings.TrimSpace(pm.VaultID)
			if customerVaultID == "" {
				return nil, errors.New("failed to create payment method vault: empty customer vault ID")
			}
			createdPaymentMethod = pm
			if pm.Provider != nil {
				provider = strings.TrimSpace(strings.ToLower(*pm.Provider))
			}
		}

		if customerVaultID != "" {
			params.CustomerVaultID = customerVaultID
		} else if trimmedToken != "" {
			params.PaymentToken = trimmedToken
		}

		resp, err := client.AddRecurringSubscription(params)
		if err != nil {
			if idemReq != nil {
				_ = s.IdempotencyService.Fail(ctx, idemReq.ID, err)
			}
			if createdPaymentMethod != nil && s.VaultService != nil {
				cleanupErr := s.VaultService.DeleteVault(ctx, createdPaymentMethod)
				if cleanupErr != nil {
					log.WithError(cleanupErr).WithFields(log.Fields{
						"vault_id": customerVaultID,
						"user_id":  user.ID,
					}).Warn("failed to cleanup payment method after subscription error")
				}
			}
			return nil, err
		}
		if idemReq != nil {
			payload, marshalErr := json.Marshal(resp)
			if marshalErr != nil {
				_ = s.IdempotencyService.Fail(ctx, idemReq.ID, marshalErr)
			} else {
				_ = s.IdempotencyService.Complete(ctx, idemReq.ID, payload)
			}
		}

		var emailCopy *string
		if email != "" {
			emailCopy = &email
		}
		subscription := &models2.Subscription{
			UserID:                  user.ID,
			PriceID:                 priceID,
			ID:                      subscriptionID,
			ProcessorSubscriptionID: resp.SubscriptionID,
			Status:                  models2.StatusPending,
			Processor:               models2.Processor(processor),
			ProcessorProvider: func() *string {
				if provider == "" {
					return nil
				}
				pp := provider
				return &pp
			}(),
			UserEmail: emailCopy,
		}
		if strings.TrimSpace(data.PaymentMethodID) != "" {
			pmUUID, _ := uuid.Parse(strings.TrimSpace(data.PaymentMethodID))
			subscription.PaymentMethodID = &pmUUID
		} else if createdPaymentMethod != nil {
			subscription.PaymentMethodID = &createdPaymentMethod.ID
		}

		if err := s.Create(ctx, subscription); err != nil {
			return nil, fmt.Errorf("failed to create subscription: %w", err)
		}
		return resp, nil
	default:
		return nil, errors.New("invalid payment processor")
	}
}

// GetUserSubscription retrieves the current subscription for a user
func (s *SubscriptionService) GetUserSubscription(ctx context.Context, userID string) (*models2.Subscription, error) {
	return s.GetByUserID(ctx, userID)
}

// CancelUserSubscription cancels a user's subscription
func (s *SubscriptionService) CancelUserSubscription(ctx context.Context, userID string, feedback string) error {
	subscription, err := s.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	if subscription.Status != models2.StatusActive {
		return errors.New("subscription is not active")
	}

	now := time.Now()
	cancelType := models2.CancelTypeUser
	subscription.Status = models2.StatusCancelled
	subscription.CancelledAt = &now
	subscription.CancelType = &cancelType
	if feedback != "" {
		subscription.CancelFeedback = &feedback
	}

	if err := s.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Entitlements are managed in lifecycle and user flows

	// Add notification
	notification := &models2.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: models2.NotificationPremiumEnded,
		Data: map[string]any{
			"reason": string(email.PremiumEndReasonUserCancel),
		},
	}
	if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
		log.WithError(err).Error("failed to create cancellation notification")
	}

	return nil
}

func buildNMIIdempotencyKey(userID string, priceID uuid.UUID, paymentToken string, paymentMethodID string) string {
	token := strings.TrimSpace(paymentToken)
	method := strings.TrimSpace(paymentMethodID)
	if token == "" {
		token = "none"
	}
	if method == "" {
		method = "none"
	}
	return fmt.Sprintf("user:%s:price:%s:token:%s:method:%s", strings.TrimSpace(userID), priceID.String(), token, method)
}

// GetAvailableProducts returns all active products with their prices
func (s *SubscriptionService) GetAvailableProducts(ctx context.Context) ([]*models2.Product, error) {
	products, err := s.ProductService.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active products: %w", err)
	}

	// Load prices for each product
	for _, product := range products {
		prices, err := s.PriceService.GetActiveByProductID(ctx, product.ID)
		if err != nil {
			log.WithFields(log.Fields{
				"product_id": product.ID,
				"error":      err.Error(),
			}).Warn("Failed to load prices for product")
			continue
		}
		product.Prices = prices
	}

	return products, nil
}

func NewSubscriptionService(
	db *db.DB,
	priceService *price.PriceService,
	productService *product.ProductService,
	notificationQueueService *notification.NotificationQueueService,
	ccbillRESTClient *ccbill.RESTClient,
	nmiClients map[string]*nmi.NMIClient,
	paymentMethodService *payment.PaymentMethodService,
) *SubscriptionService {
	return &SubscriptionService{
		subscriptionRepo:         repo.NewSubscriptionRepo(db),
		PriceService:             priceService,
		ProductService:           productService,
		NotificationQueueService: notificationQueueService,
		CCBillRESTClient:         ccbillRESTClient,
		NMIClients:               nmiClients,
		PaymentMethodService:     paymentMethodService,
	}
}

func (s *SubscriptionService) Create(ctx context.Context, subscription *models2.Subscription) error {
	return s.subscriptionRepo.Create(ctx, subscription)
}

func (s *SubscriptionService) GetByID(ctx context.Context, id uuid.UUID) (*models2.Subscription, error) {
	return s.subscriptionRepo.GetByID(ctx, id)
}

func (s *SubscriptionService) GetByUserID(ctx context.Context, id string) (*models2.Subscription, error) {
	return s.subscriptionRepo.GetLatestByUserID(ctx, id)
}

func (s *SubscriptionService) GetByUserIDAndPriceID(ctx context.Context, id string, priceID uuid.UUID) (*models2.Subscription, error) {
	return s.subscriptionRepo.GetByUserIDAndPriceID(ctx, id, priceID)
}

func (s *SubscriptionService) Update(ctx context.Context, subscription *models2.Subscription) error {
	return s.subscriptionRepo.Update(ctx, subscription)
}

func (s *SubscriptionService) GetSubscribers(ctx context.Context, params query.QueryOptions[GetSubscriptionsFilters]) ([]*models2.Subscription, int64, error) {
	repoParams := query.QueryOptions[repo.SubscriptionFilters]{
		Filters: repo.SubscriptionFilters{
			UserID:    params.Filters.UserID,
			Status:    params.Filters.Status,
			PriceID:   params.Filters.PriceID,
			Processor: params.Filters.Processor,
		},
		Limit:  params.Limit,
		Offset: params.Offset,
	}

	return s.subscriptionRepo.GetSubscribers(ctx, repoParams)
}

func (s *SubscriptionService) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]models2.Subscription, int, error) {
	return s.subscriptionRepo.GetPaginatedByUserID(ctx, userID, page, pageSize)
}

// GetSubscriptionsWithDetailsForUser retrieves subscriptions with related price information for billing history
func (s *SubscriptionService) GetSubscriptionsWithDetailsForUser(ctx context.Context, userID string, page, pageSize int) ([]models2.Subscription, int, error) {
	return s.subscriptionRepo.GetSubscriptionsWithDetailsForUser(ctx, userID, page, pageSize)
}

// GetActiveSubscriptionsByUserID retrieves only active subscriptions for a user
func (s *SubscriptionService) GetActiveSubscriptionsByUserID(ctx context.Context, userID string) ([]models2.Subscription, error) {
	return s.subscriptionRepo.GetActiveSubscriptionsByUserID(ctx, userID)
}

// GetSubscriptionsByProcessorAndUserID retrieves subscriptions filtered by processor
func (s *SubscriptionService) GetSubscriptionsByProcessorAndUserID(ctx context.Context, userID string, processor models2.Processor) ([]models2.Subscription, error) {
	return s.subscriptionRepo.GetSubscriptionsByProcessorAndUserID(ctx, userID, processor)
}

// GetActiveSubscription retrieves the active subscription for a user
func (s *SubscriptionService) GetActiveSubscription(ctx context.Context, userID string) (*models2.Subscription, error) {
	return s.subscriptionRepo.GetActiveSubscription(ctx, userID)
}

// GetByProcessorSubscriptionID finds a subscription by processor, provider, and processor_subscription_id
func (s *SubscriptionService) GetByProcessorSubscriptionID(ctx context.Context, processor, provider, processorSubscriptionID string) (*models2.Subscription, error) {
	return s.subscriptionRepo.GetByProcessorSubscriptionID(ctx, processor, provider, processorSubscriptionID)
}

// GetActiveSubscriptionsByProcessor gets all active subscriptions for a processor
func (s *SubscriptionService) GetActiveSubscriptionsByProcessor(ctx context.Context, processor string) ([]*models2.Subscription, error) {
	return s.subscriptionRepo.GetActiveSubscriptionsByProcessor(ctx, processor)
}

// Delete removes a subscription from the database permanently
func (s *SubscriptionService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.subscriptionRepo.Delete(ctx, id)
}
