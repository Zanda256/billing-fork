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
	"github.com/doujins-org/doujins-billing/internal/manager/api/idempotency"
	"github.com/doujins-org/doujins-billing/internal/manager/api/lifecycle"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/api/solana"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/api/user"
	"github.com/doujins-org/doujins-billing/internal/manager/api/vault"
	"github.com/doujins-org/doujins-billing/internal/manager/data/migrations/clickhouse"
	"github.com/doujins-org/doujins-billing/internal/manager/data/migrations/postgres"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/internal/manager/web/webhook"
	ccbill2 "github.com/doujins-org/doujins-billing/pkg/ccbill"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/nmi"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	redis "github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"

	authkitPostgres "github.com/PaulFidika/authkit/migrations/postgres"
	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/migratekit"
)

func buildRuntime(cfg *config.Config) (*Runtime, error) {
	database, err := createDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create db: %w", err)
	}

	redisClient, err := createRedisClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}

	ccbillClient := createCCBillClient(cfg)
	ccbillRESTClient := createCCBillRESTClient(cfg)
	ccbillDataLinkClient := createCCBillDataLinkClient(cfg)
	nmiClients, err := createNMIClients(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create nmi clients: %w", err)
	}

	serviceInstances := createServices(database, cfg, ccbillRESTClient, nmiClients)

	var emailService *email.EmailService
	var subscriptionEmailService *subscription.SubscriptionEmailService
	if cfg.SendGrid != nil {
		if es, err := email.NewEmailService(cfg.SendGrid); err != nil {
			log.WithError(err).Warn("EmailService init failed; email disabled")
		} else {
			emailService = es
			subscriptionEmailService = subscription.NewSubscriptionEmailService(
				emailService,
				serviceInstances.SubscriptionService,
				serviceInstances.ProductService,
				serviceInstances.PriceService,
				repo.NewProfileRepo(database),
			)
		}
	}

	notificationService := notification.NewNotificationService(
		serviceInstances.NotificationQueueService,
		subscriptionEmailService,
		emailService,
	)
	serviceInstances.WebhookDispatcher.NotificationService = notificationService
	serviceInstances.SubscriptionLifecycleService.SetNotificationService(notificationService)
	serviceInstances.SolanaPaymentService.SetNotificationService(notificationService)

	runtime := &Runtime{
		DB:                 database,
		RedisClient:        redisClient,
		Config:             cfg,
		CCBillClient:       ccbillClient,
		CCBillRESTClient:   ccbillRESTClient,
		CCBillDataLink:     ccbillDataLinkClient,
		CCBillAliasService: serviceInstances.CCBillAliasService,
		NMIClients:         nmiClients,

		SubscriptionService:        serviceInstances.SubscriptionService,
		UserService:                serviceInstances.UserService,
		ProductService:             serviceInstances.ProductService,
		PriceService:               serviceInstances.PriceService,
		NotificationQueueService:   serviceInstances.NotificationQueueService,
		NotificationService:        notificationService,
		PaymentMethodService:       serviceInstances.PaymentMethodService,
		PaymentService:             serviceInstances.PurchaseService,
		EntitlementService:         serviceInstances.EntitlementService,
		VaultService:               serviceInstances.VaultService,
		SolanaWalletService:        serviceInstances.SolanaWalletService,
		SolanaPaymentService:       serviceInstances.SolanaPaymentService,
		SolanaPaymentIntentService: serviceInstances.SolanaPaymentIntentService,

		UserSubscriptionService:   serviceInstances.UserSubscriptionService,
		PublicSubscriptionService: serviceInstances.PublicSubscriptionService,
		AdminSubscriptionService:  serviceInstances.AdminSubscriptionService,

		EmailService:                 emailService,
		SubscriptionEmailService:     subscriptionEmailService,
		SubscriptionLifecycleService: serviceInstances.SubscriptionLifecycleService,
		WebhookEventService:          serviceInstances.WebhookEventService,
		WebhookDispatcher:            serviceInstances.WebhookDispatcher,
		DeduplicationService:         serviceInstances.DeduplicationService,
	}
	runtime.WebhookProcessor = &webhook.WebhookProcessor{
		Events:     runtime.WebhookEventService,
		Dispatcher: runtime.WebhookDispatcher,
	}

	// River client will be initialized later in StartWorkers with proper worker registration
	// if client, err := buildRiverClient(cfg); err != nil {
	// 	log.WithError(err).Warn("River client init failed; workers disabled")
	// } else {
	// 	runtime.RiverClient = client
	// }

	if cfg.ClickHouse != nil {
		if bes, err := billing.NewBillingEventService(cfg.ClickHouse); err != nil {
			log.WithError(err).Warn("BillingEventService init failed; analytics disabled")
		} else {
			runtime.BillingEventService = bes
		}
	}

	runtime.WebhookDispatcher.BillingEventService = runtime.BillingEventService

	return runtime, nil
}

func createDatabase(cfg *config.Config) (*db.DB, error) {
	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return nil, err
	}

	// Validate that all migrations have been applied before starting
	bunDB := database.GetDB().(*bun.DB)
	sqlDB := bunDB.DB

	if err := migratekit.ValidatePostgresMigrations(context.Background(), sqlDB,
		migratekit.MigrationSource{App: "authkit", FS: authkitPostgres.FS},
		migratekit.MigrationSource{App: "billing", FS: postgresmigrations.FS},
	); err != nil {
		log.WithError(err).Fatal("Postgres migrations validation failed")
		return nil, err
	}

	// Validate ClickHouse migrations if ClickHouse is configured
	// ClickHouse is optional - warn if validation fails but continue running
	if cfg.ClickHouse != nil {
		log.Infof("Validating ClickHouse migrations for database %s at %s", cfg.ClickHouse.Database, cfg.ClickHouse.ClientAddr)
		if err := migratekit.ValidateClickHouseMigrations(
			context.Background(),
			&migratekit.ClickHouseConfig{
				ClientAddr: cfg.ClickHouse.ClientAddr,
				Database:   cfg.ClickHouse.Database,
				Username:   cfg.ClickHouse.Username,
				Password:   cfg.ClickHouse.Password,
				App:        "billing",
			},
			clickhousemigrations.FS,
		); err != nil {
			log.WithError(err).Warn("ClickHouse migrations validation failed - analytics disabled")
		}
	}

	return database, nil
}

func createNMIClients(cfg *config.Config) (map[string]*nmi.NMIClient, error) {
	clients := make(map[string]*nmi.NMIClient)
	if cfg == nil || cfg.NMI == nil {
		return clients, nil
	}

	for name := range cfg.NMI.Providers {
		settings, err := cfg.NMI.ProviderSettings(name)
		if err != nil {
			return nil, err
		}
		providerKey := strings.TrimSpace(strings.ToLower(settings.Name))
		if providerKey == "" {
			providerKey = "mobius"
		}

		if _, exists := clients[providerKey]; exists {
			return nil, fmt.Errorf("duplicate nmi provider '%s' detected in configuration", providerKey)
		}

		client, err := nmi.NewClient(providerKey, settings, cfg.Env == config.EnvProd)
		if err != nil {
			return nil, err
		}

		// Log test mode status for this provider
		if settings.TestMode {
			log.Warnf("⚠️  NMI provider '%s' TEST MODE is ENABLED - no real charges will be processed", providerKey)
		} else {
			log.Warnf("🔴 NMI provider '%s' TEST MODE is DISABLED - REAL CHARGES WILL BE PROCESSED!", providerKey)
		}

		clients[providerKey] = client
	}

	return clients, nil
}

func createRedisClient(cfg *config.Config) (*redis.Client, error) {
	if cfg.Redis == nil {
		return nil, nil
	}
	redisOpts := &redis.Options{
		Addr: cfg.Redis.Addr,
		DB:   cfg.Redis.DB,
	}
	if cfg.Redis.Password != "" && cfg.Env == config.EnvProd {
		redisOpts.Password = cfg.Redis.Password
		log.Info("Redis authentication enabled")
	} else {
		log.Info("Redis authentication disabled - connecting without credentials")
	}
	client := redis.NewClient(redisOpts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Ping(ctx).Result(); err != nil {
		log.Warnf("Redis connection test failed: %v - rate limiting will fall back to permissive mode", err)
	} else {
		log.Info("Redis connection successful - rate limiting enabled")
	}
	return client, nil
}

func createCCBillClient(cfg *config.Config) *ccbill2.CCBillClient {
	if cfg.CCBill != nil {
		if cfg.CCBill.TestMode {
			log.Warn("⚠️  CCBill TEST MODE is ENABLED - no real charges will be processed")
		} else {
			log.Warn("🔴 CCBill TEST MODE is DISABLED - REAL CHARGES WILL BE PROCESSED!")
		}
	}
	return ccbill2.NewClient(cfg.CCBill, cfg.Env == config.EnvProd)
}

func createCCBillRESTClient(cfg *config.Config) *ccbill2.RESTClient {
	return ccbill2.NewRESTClient(cfg.CCBill)
}

func createCCBillDataLinkClient(cfg *config.Config) *ccbill2.DataLinkClient {
	if cfg.CCBill == nil {
		return nil
	}
	if cfg.CCBill.DataLinkUsername == "" || cfg.CCBill.DataLinkPassword == "" || cfg.CCBill.ClientAccNum == "" {
		log.Info("CCBill DataLink credentials missing; DataLink worker disabled")
		return nil
	}

	client := ccbill2.NewDataLinkClient(cfg.CCBill)
	if err := client.ValidateConfig(); err != nil {
		log.WithError(err).Warn("Invalid CCBill DataLink configuration; worker disabled")
		return nil
	}
	return client
}

type servicesInstances struct {
	SubscriptionService *subscription.SubscriptionService
	UserService         *user.UserService
	CCBillAliasService  *cc_bill_alias.CCBillAliasService

	ProductService             *product.ProductService
	PriceService               *price.PriceService
	NotificationQueueService   *notification.NotificationQueueService
	PaymentMethodService       *payment.PaymentMethodService
	PurchaseService            *payment.PaymentService
	EntitlementService         *entitlement.EntitlementService
	VaultService               *vault.VaultService
	SolanaWalletService        *solana.SolanaWalletService
	SolanaPaymentService       *solana.SolanaPaymentService
	SolanaPaymentIntentService *solana.SolanaPaymentIntentService

	UserSubscriptionService   *user.UserSubscriptionService
	PublicSubscriptionService *subscription.PublicSubscriptionService
	AdminSubscriptionService  *admin.AdminSubscriptionService

	EmailService             *email.EmailService
	SubscriptionEmailService *subscription.SubscriptionEmailService

	SubscriptionLifecycleService *lifecycle.SubscriptionLifecycleService
	DeduplicationService         *deduplication.DeduplicationService
	WebhookEventService          *webhook.WebhookEventService
	WebhookDispatcher            *webhook.WebhookDispatcher
}

func createServices(database *db.DB, cfg *config.Config, ccbillRESTClient *ccbill2.RESTClient, nmiClients map[string]*nmi.NMIClient) *servicesInstances {
	userService := user.NewUserService(database)
	productService := product.NewProductService(database)
	priceService := price.NewPriceService(database)
	notificationQueueService := notification.NewNotificationQueueService(database)
	paymentMethodService := payment.NewPaymentMethodService(database)
	purchaseService := payment.NewPaymentService(database)
	entitlementService := entitlement.NewEntitlementService(database)
	aliasService := cc_bill_alias.NewCCBillAliasService(database)
	solanaWalletService := solana.NewSolanaWalletService(database)
	solanaPaymentService := solana.NewSolanaPaymentService(database, cfg, priceService, purchaseService, productService, entitlementService, nil)
	solanaPaymentIntentService := solana.NewSolanaPaymentIntentService(database, cfg, priceService)

	subscriptionLifecycleService := lifecycle.NewSubscriptionLifecycleService(
		database,
		productService,
		priceService,
		entitlementService,
		notificationQueueService,
	)

	subscriptionService := subscription.NewSubscriptionService(
		database,
		priceService,
		productService,
		notificationQueueService,
		ccbillRESTClient,
		nmiClients,
		paymentMethodService,
	)

	vaultService := vault.NewVaultService(paymentMethodService, subscriptionService, nmiClients, database)
	subscriptionService.VaultService = vaultService
	subscriptionService.IdempotencyService = idempotency.NewIdempotencyService(database)

	userSubscriptionService := user.NewUserSubscriptionService(
		subscriptionService,
		productService,
		priceService,
		purchaseService,
		notificationQueueService,
		entitlementService,
		nmiClients,
	)

	publicSubscriptionService := subscription.NewPublicSubscriptionService(
		productService,
		priceService,
	)

	adminSubscriptionService := admin.NewAdminSubscriptionService(
		subscriptionService,
		productService,
		priceService,
		entitlementService,
		notificationQueueService,
		purchaseService,
	)

	deduplicationService := deduplication.NewDeduplicationService(database)
	webhookEventService := webhook.NewWebhookEventService(database, cfg.GetWebhookRetryConfig())
	webhookDispatcher := &webhook.WebhookDispatcher{
		DB:                           database,
		PriceService:                 priceService,
		ProductService:               productService,
		NotificationQueueService:     notificationQueueService,
		NotificationService:          nil,
		SubscriptionService:          subscriptionService,
		PaymentService:               purchaseService,
		BillingEventService:          nil,
		SubscriptionLifecycleService: subscriptionLifecycleService,
		CCBillAliasService:           aliasService,
		DeduplicationService:         deduplicationService,
		CCBillRESTClient:             ccbillRESTClient,
		NMIClients:                   nmiClients,
	}

	return &servicesInstances{
		SubscriptionService:          subscriptionService,
		UserService:                  userService,
		CCBillAliasService:           aliasService,
		ProductService:               productService,
		PriceService:                 priceService,
		NotificationQueueService:     notificationQueueService,
		PaymentMethodService:         paymentMethodService,
		PurchaseService:              purchaseService,
		EntitlementService:           entitlementService,
		VaultService:                 vaultService,
		SolanaWalletService:          solanaWalletService,
		SolanaPaymentService:         solanaPaymentService,
		SolanaPaymentIntentService:   solanaPaymentIntentService,
		UserSubscriptionService:      userSubscriptionService,
		PublicSubscriptionService:    publicSubscriptionService,
		AdminSubscriptionService:     adminSubscriptionService,
		SubscriptionLifecycleService: subscriptionLifecycleService,
		DeduplicationService:         deduplicationService,
		WebhookEventService:          webhookEventService,
		WebhookDispatcher:            webhookDispatcher,
	}
}

func buildRiverClient(cfg *config.Config, workers *river.Workers) (*river.Client[pgx.Tx], *pgxpool.Pool, error) {
	if cfg.DB == nil {
		return nil, nil, fmt.Errorf("missing database configuration for River")
	}
	dbURL := cfg.DB.GetConnectionString()
	if dbURL == "" {
		return nil, nil, fmt.Errorf("missing database configuration for River (DB_URL or DB_HOST/DB_PORT/etc.)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating pgx pool for River: %w", err)
	}

	// Get schema for River tables (same as billing schema)
	schema := "billing" // Hardcoded schema

	drv := riverpgxv5.New(pool)
	client, err := river.NewClient[pgx.Tx](drv, &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
			"billing":          {MaxWorkers: 20},
		},
		Schema:  schema, // Use billing schema for River tables
		Workers: workers,
	})
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("failed creating River client: %w", err)
	}
	return client, pool, nil
}
