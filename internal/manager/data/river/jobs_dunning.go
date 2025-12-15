package riverjobs

import (
	"context"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/entitlement"
	"github.com/doujins-org/doujins-billing/internal/manager/api/lifecycle"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"github.com/doujins-org/doujins-billing/pkg/nmi"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	log "github.com/sirupsen/logrus"
)

const (
	QueueBilling       = "billing"
	KindDunningAttempt = "billing.dunning_attempt"
	KindDunningScan    = "billing.dunning_scan"
)

// DunningAttemptArgs schedules a single dunning attempt for a subscription.
type DunningAttemptArgs struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
}

func (DunningAttemptArgs) Kind() string { return KindDunningAttempt }

const defaultNMIProvider = "mobius"

// DunningAttemptWorker performs a dunning attempt against an NMI provider (e.g. Mobius) for a single subscription.
type DunningAttemptWorker struct {
	river.WorkerDefaults[DunningAttemptArgs]
	DB         *db.DB
	NMIClients map[string]*nmi.NMIClient
}

func (DunningAttemptWorker) Kind() string { return KindDunningAttempt }

func (w *DunningAttemptWorker) Work(ctx context.Context, job *river.Job[DunningAttemptArgs]) error {
	if w.DB == nil {
		return fmt.Errorf("db is required")
	}
	if w.NMIClients == nil {
		log.WithContext(ctx).Warn("NMI clients not configured; skipping dunning attempt")
		return nil
	}

	// Load subscription with relations required
	var sub models2.Subscription
	if err := w.DB.GetDB().NewSelect().
		Model(&sub).
		Where("id = ?", job.Args.SubscriptionID).
		Relation("Price").
		Relation("PaymentMethod").
		Scan(ctx); err != nil {
		return fmt.Errorf("load subscription: %w", err)
	}

	priceSvc := price.NewPriceService(w.DB)
	productSvc := product.NewProductService(w.DB)
	entitlementSvc := entitlement.NewEntitlementService(w.DB)
	notifQueueSvc := notification.NewNotificationQueueService(w.DB)
	lifecycle := lifecycle.NewSubscriptionLifecycleService(w.DB, productSvc, priceSvc, entitlementSvc, notifQueueSvc)
	paymentSvc := payment.NewPaymentService(w.DB)

	if sub.Processor != models2.ProcessorNMI {
		log.WithContext(ctx).WithFields(log.Fields{
			"subscription_id": sub.ID,
			"processor":       sub.Processor,
		}).Warn("Skipping dunning attempt for non-NMI subscription")
		return nil
	}

	provider := resolveSubscriptionProvider(&sub)
	client := w.NMIClients[provider]
	if client == nil {
		log.WithContext(ctx).WithFields(log.Fields{
			"subscription_id": sub.ID,
			"provider":        provider,
		}).Warn("NMI client not configured for provider; skipping dunning attempt")
		return nil
	}

	// Validate payment method
	pm := sub.PaymentMethod
	if pm == nil || !pm.IsActive || pm.VaultID == "" || pm.BillingID == nil || *pm.BillingID == "" {
		reason := "payment method unavailable for rebill"
		if err := lifecycle.FailMembership(ctx, &lifecycle.FailMembershipParams{
			Processor:               models2.ProcessorNMI,
			ProcessorProvider:       provider,
			ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
			FailureReason:           &reason,
		}); err != nil {
			log.WithContext(ctx).WithError(err).WithField("subscription_id", sub.ID).Warn("fail-membership after missing payment method")
		}
		return nil
	}

	// Attempt manual rebill via configured NMI provider
	rebillResp, err := client.AttemptManualRebill(nmi.ManualRebillParams{
		VaultID:        pm.VaultID,
		BillingID:      *pm.BillingID,
		SubscriptionID: sub.ProcessorSubscriptionID,
	})
	if err != nil {
		msg := fmt.Sprintf("manual rebill request failed: %v", err)
		if err2 := lifecycle.FailMembership(ctx, &lifecycle.FailMembershipParams{
			Processor:               models2.ProcessorNMI,
			ProcessorProvider:       provider,
			ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
			FailureReason:           &msg,
		}); err2 != nil {
			log.WithContext(ctx).WithError(err2).WithField("subscription_id", sub.ID).Warn("record rebill failure")
		}
		return nil
	}

	if rebillResp == nil || !rebillResp.Success {
		reason := "rebill declined"
		if rebillResp != nil && rebillResp.ErrorMessage != "" {
			reason = rebillResp.ErrorMessage
		}
		if err := lifecycle.FailMembership(ctx, &lifecycle.FailMembershipParams{
			Processor:               models2.ProcessorNMI,
			ProcessorProvider:       provider,
			ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
			FailureReason:           &reason,
		}); err != nil {
			log.WithContext(ctx).WithError(err).WithField("subscription_id", sub.ID).Warn("apply failure policy after declined rebill")
		}
		return nil
	}

	// Success: renew membership window and create a payment record
	if err := lifecycle.RenewMembership(ctx, &lifecycle.RenewMembershipParams{
		Processor:               models2.ProcessorNMI,
		ProcessorProvider:       provider,
		ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
	}); err != nil {
		log.WithContext(ctx).WithError(err).WithField("subscription_id", sub.ID).Error("renew membership after successful rebill")
		return nil
	}

	// Create payment record
	amount := 0.0
	currency := "USD"
	if sub.Price != nil {
		amount = sub.Price.Amount
		currency = sub.Price.Currency
	} else if p, err := priceSvc.GetByID(ctx, sub.PriceID); err == nil {
		amount = p.Amount
		currency = p.Currency
	}

	pay := &models2.Payment{
		ID:             uuid.New(),
		UserID:         sub.UserID,
		PriceID:        sub.PriceID,
		SubscriptionID: &sub.ID,
		Processor:      models2.ProcessorNMI,
		TransactionID:  rebillResp.TransactionID,
		Amount:         amount,
		Currency:       currency,
		PurchasedAt:    time.Now(),
		CreatedAt:      time.Now(),
	}
	if provider != "" {
		providerCopy := provider
		pay.ProcessorProvider = &providerCopy
	}
	if err := paymentSvc.Create(ctx, pay); err != nil {
		log.WithContext(ctx).WithError(err).WithField("subscription_id", sub.ID).Warn("create payment record for rebill")
	}
	return nil
}

// JobInserter allows enqueuing additional River jobs from within a worker.
type JobInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// DunningSweepArgs triggers a scan and inline processing of due dunning attempts.
type DunningSweepArgs struct{}

func (DunningSweepArgs) Kind() string { return KindDunningScan }

// DunningSweepWorker scans for due past_due NMI subscriptions and enqueues attempt jobs.
type DunningSweepWorker struct {
	river.WorkerDefaults[DunningSweepArgs]
	DB       *db.DB
	Inserter JobInserter
}

func (DunningSweepWorker) Kind() string { return KindDunningScan }

func (w *DunningSweepWorker) Work(ctx context.Context, job *river.Job[DunningSweepArgs]) error {
	if w.DB == nil {
		return fmt.Errorf("db is required")
	}
	if w.Inserter == nil {
		return fmt.Errorf("job inserter is required")
	}
	// query due subs
	due := []models2.Subscription{}
	if err := w.DB.GetDB().NewSelect().
		Model(&due).
		Where("processor = ?", models2.ProcessorNMI).
		Where("status = ?", models2.StatusPastDue).
		Where("next_retry_at IS NOT NULL AND next_retry_at <= NOW()").
		Column("id").
		Scan(ctx); err != nil {
		return fmt.Errorf("query due subscriptions: %w", err)
	}
	if len(due) == 0 {
		log.WithContext(ctx).Info("DunningScan: no attempts due")
		return nil
	}
	count := 0
	for _, sub := range due {
		if _, err := w.Inserter.Insert(ctx, DunningAttemptArgs{SubscriptionID: sub.ID}, &river.InsertOpts{Queue: QueueBilling}); err != nil {
			log.WithContext(ctx).WithError(err).WithField("subscription_id", sub.ID).Warn("enqueue dunning attempt failed")
			continue
		}
		count++
	}
	log.WithContext(ctx).WithField("count", count).Info("DunningScan: enqueued attempts")
	return nil
}

func resolveSubscriptionProvider(sub *models2.Subscription) string {
	if sub == nil {
		return defaultNMIProvider
	}

	if p := normalizeProvider(sub.ProcessorProvider); p != "" {
		return p
	}
	if sub.PaymentMethod != nil {
		if p := normalizeProvider(sub.PaymentMethod.Provider); p != "" {
			return p
		}
	}
	if sub.Price != nil {
		if p := normalizeProvider(sub.Price.NMIProvider); p != "" {
			return p
		}
	}
	return defaultNMIProvider
}

func normalizeProvider(value interface{}) string {
	switch v := value.(type) {
	case *string:
		if v == nil {
			return ""
		}
		return normalizeProvider(*v)
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(v))
		return trimmed
	default:
		return ""
	}
}
