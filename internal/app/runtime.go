package app

import (
	"context"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/admin"
	"github.com/doujins-org/doujins-billing/internal/manager/api/billing"
	"github.com/doujins-org/doujins-billing/internal/manager/api/cc-bill-alias"
	"github.com/doujins-org/doujins-billing/internal/manager/api/deduplication"
	"github.com/doujins-org/doujins-billing/internal/manager/api/email"
	"github.com/doujins-org/doujins-billing/internal/manager/api/entitlement"
	"github.com/doujins-org/doujins-billing/internal/manager/api/lifecycle"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/api/solana"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/api/user"
	"github.com/doujins-org/doujins-billing/internal/manager/data/vault"
	"github.com/doujins-org/doujins-billing/internal/manager/web/webhook"
	ccbill2 "github.com/doujins-org/doujins-billing/pkg/ccbill"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/nmi"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	redis "github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
)

// Runtime aggregates infrastructure clients and application services.
type Runtime struct {
	DB                 *db.DB
	RedisClient        *redis.Client
	Config             *config.Config
	CCBillClient       *ccbill2.CCBillClient
	CCBillRESTClient   *ccbill2.RESTClient
	CCBillDataLink     *ccbill2.DataLinkClient
	CCBillAliasService *cc_bill_alias.CCBillAliasService
	NMIClients         map[string]*nmi.NMIClient
	RiverClient        *river.Client[pgx.Tx]
	riverPool          *pgxpool.Pool

	UserService              *user.UserService
	SubscriptionService      *subscription.SubscriptionService
	ProductService           *product.ProductService
	PriceService             *price.PriceService
	NotificationQueueService *notification.NotificationQueueService
	NotificationService      *notification.NotificationService
	PaymentMethodService     *payment.PaymentMethodService
	PaymentService           *payment.PaymentService
	VaultService             *vault.VaultService

	UserSubscriptionService   *user.UserSubscriptionService
	PublicSubscriptionService *subscription.PublicSubscriptionService
	AdminSubscriptionService  *admin.AdminSubscriptionService

	EmailService             *email.EmailService
	SubscriptionEmailService *subscription.SubscriptionEmailService

	BillingEventService *billing.BillingEventService
	EntitlementService  *entitlement.EntitlementService

	SolanaWalletService        *solana.SolanaWalletService
	SolanaPaymentService       *solana.SolanaPaymentService
	SolanaPaymentIntentService *solana.SolanaPaymentIntentService

	SubscriptionLifecycleService *lifecycle.SubscriptionLifecycleService
	WebhookEventService          *webhook.WebhookEventService
	WebhookDispatcher            *webhook.WebhookDispatcher
	DeduplicationService         *deduplication.DeduplicationService
	WebhookProcessor             *webhook.WebhookProcessor

	riverStarted bool
}

// Close gracefully shuts down runtime resources.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.RiverClient != nil && r.riverStarted {
		log.Info("Stopping River background workers...")
		if err := r.RiverClient.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop River client: %w", err))
		}
		r.riverStarted = false
	}
	if r.riverPool != nil {
		r.riverPool.Close()
		r.riverPool = nil
	}
	if r.DB != nil {
		if err := r.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close db: %w", err))
		}
	}
	if r.BillingEventService != nil {
		if err := r.BillingEventService.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close billing event service: %w", err))
		}
	}
	if r.RedisClient != nil {
		if err := r.RedisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Redis client: %w", err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("failed to close some resources: %v", errs)
}

// InitRiver initialises the River client for background workers.
func (r *Runtime) InitRiver(ctx context.Context) error {
	if r.RiverClient != nil {
		return nil
	}
	workers, err := r.buildRiverWorkers(ctx)
	if err != nil {
		return fmt.Errorf("build river workers: %w", err)
	}
	client, pool, err := buildRiverClient(r.Config, workers)
	if err != nil {
		return err
	}
	r.RiverClient = client
	r.riverPool = pool
	return nil
}

// StartWorkers spins up background workers using the River queue system.
func (r *Runtime) StartWorkers(ctx context.Context) {
	if r == nil {
		return
	}
	if !r.riverStarted {
		if err := r.InitRiver(ctx); err != nil {
			log.WithError(err).Error("Failed to initialize River client")
			return
		}
		if r.RiverClient != nil {
			// Build periodic jobs
			periodicJobs, err := r.buildRiverPeriodicJobs(ctx)
			if err != nil {
				log.WithError(err).Error("Failed to configure River periodic jobs")
				return
			}
			// Add periodic jobs to the client
			for _, job := range periodicJobs {
				r.RiverClient.PeriodicJobs().Add(job)
			}

			r.riverStarted = true
			go func() {
				log.Info("Starting River background workers in-server")
				if err := r.RiverClient.Start(ctx); err != nil {
					log.WithError(err).Error("River workers stopped with error")
				} else {
					log.Info("River workers stopped")
				}
			}()
		}
	}
}
