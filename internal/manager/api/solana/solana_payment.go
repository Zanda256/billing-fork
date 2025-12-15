package solana

import (
	"context"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/entitlement"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

var (
	ErrInvalidToken  = errors.New("invalid or unsupported token")
	ErrPriceNotFound = errors.New("price not found")
)

type SolanaPaymentService struct {
	db                 *db.DB
	cfg                *config.Config
	priceService       *price.PriceService
	paymentSvc         *payment.PaymentService
	productService     *product.ProductService
	entitlementService *entitlement.EntitlementService
	notificationSvc    *notification.NotificationService
}

func NewSolanaPaymentService(db *db.DB, cfg *config.Config, price *price.PriceService, payment *payment.PaymentService, product *product.ProductService, entitlement *entitlement.EntitlementService, notification *notification.NotificationService) *SolanaPaymentService {
	return &SolanaPaymentService{
		db:                 db,
		cfg:                cfg,
		priceService:       price,
		paymentSvc:         payment,
		productService:     product,
		entitlementService: entitlement,
		notificationSvc:    notification,
	}
}

// SetNotificationService wires the notification service after SolanaPaymentService construction
func (s *SolanaPaymentService) SetNotificationService(notification *notification.NotificationService) {
	s.notificationSvc = notification
}

// Generate returns payment calculation for client-side transaction building.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Generate(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, userWallet string) (amount float64, currency string, tokenAmount uint64, expiresAt time.Time, err error) {
	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return 0, "", 0, time.Time{}, fmt.Errorf("%w: %v", ErrPriceNotFound, err)
	}
	if !price.IsActive {
		return 0, "", 0, time.Time{}, fmt.Errorf("price %s is not available", price.ID)
	}
	if s.productService != nil {
		product, err := s.productService.GetByID(ctx, price.ProductID)
		if err != nil {
			return 0, "", 0, time.Time{}, fmt.Errorf("failed to load product: %w", err)
		}
		if !product.IsActive {
			return 0, "", 0, time.Time{}, fmt.Errorf("product %s is not available", product.ID)
		}
	}
	if s.cfg.Solana == nil {
		return 0, "", 0, time.Time{}, fmt.Errorf("solana not configured")
	}
	tok, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tok.Enabled {
		return 0, "", 0, time.Time{}, ErrInvalidToken
	}
	mainnetMint := tok.MainnetMint
	if mainnetMint == "" {
		mainnetMint = tok.Mint
	}
	if mainnetMint == "" {
		return 0, "", 0, time.Time{}, fmt.Errorf("token %s missing mint configuration", tokenSymbol)
	}

	tokenUnits, _, err := calculateTokenQuote(ctx, tok, price.Amount)
	if err != nil {
		return 0, "", 0, time.Time{}, err
	}
	exp := time.Now().Add(10 * time.Minute)

	// Disallow one-off if a subscription entitlement is already active (indefinite)
	if userID != "" && s.entitlementService != nil {
		exists, err := s.entitlementService.HasActiveIndefinite(ctx, userID, "premium", time.Now())
		if err != nil {
			return 0, "", 0, time.Time{}, fmt.Errorf("failed entitlement check: %w", err)
		}
		if exists {
			return 0, "", 0, time.Time{}, fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
		}
	}

	return price.Amount, price.Currency, tokenUnits, exp, nil
}

// Submit records a confirmed payment for the given price and user, and grants associated entitlements.
// Callers are expected to verify the on-chain transaction (signature + contents) before invoking this method.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Submit(ctx context.Context, userID string, intentID uuid.UUID, priceID uuid.UUID, signature string, userEmail *string) (*models2.Payment, error) {
	var payment *models2.Payment
	var queuedNotifications []*models2.NotificationQueue

	err := s.db.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Create transactional services
		txDB := db.NewWithTx(tx)
		priceService := price.NewPriceService(txDB)
		paymentService := payment.NewPaymentService(txDB)
		productService := product.NewProductService(txDB)
		entitlementService := entitlement.NewEntitlementService(txDB)
		solanaTxnRepo := repo.NewSolanaTransactionRepo(txDB)
		notificationQueueService := notification.NewNotificationQueueService(txDB)

		price, err := priceService.GetByID(ctx, priceID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPriceNotFound, err)
		}

		if userID != "" {
			exists, err := entitlementService.HasActiveIndefinite(ctx, userID, "premium", time.Now())
			if err != nil {
				return fmt.Errorf("failed entitlement check: %w", err)
			}
			if exists {
				return fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
			}
		}

		now := time.Now()
		payment = &models2.Payment{
			ID:            uuid.New(),
			UserID:        userID,
			PriceID:       price.ID,
			Processor:     models2.ProcessorSolana,
			TransactionID: signature,
			Amount:        price.Amount,
			Currency:      price.Currency,
			PurchasedAt:   now,
		}
		if err := paymentService.Create(ctx, payment); err != nil {
			return fmt.Errorf("failed to create payment: %w", err)
		}

		product, err := productService.GetByID(ctx, price.ProductID)
		if err != nil {
			return fmt.Errorf("failed to get product: %w", err)
		}
		if !product.IsActive {
			return fmt.Errorf("product %s is not available", product.ID)
		}

		entNames := resolveEntitlementNames(product)
		defaultDays := resolvePriceDurationDays(price)
		for _, ent := range entNames {
			days := resolveEntitlementDuration(product, ent, defaultDays)
			if days <= 0 {
				days = defaultDays
			}
			if days <= 0 {
				days = defaultSolanaWindowDays
			}
			if _, err := entitlementService.AppendEntitlementDays(ctx, userID, ent, days, models2.EntitlementSourceOneOff, &payment.ID); err != nil {
				return fmt.Errorf("failed to grant entitlement %s: %w", ent, err)
			}
		}

		premiumStarted := &models2.NotificationQueue{
			ID:        uuid.New(),
			UserID:    userID,
			EventType: models2.NotificationPremiumStarted,
		}
		if err := notificationQueueService.Create(ctx, premiumStarted); err != nil {
			log.WithContext(ctx).WithError(err).WithField("user_id", userID).Warn("failed to queue premium started notification for solana purchase")
		} else {
			queuedNotifications = append(queuedNotifications, premiumStarted)
		}

		receiptData := map[string]any{
			"amount":         payment.Amount,
			"currency":       payment.Currency,
			"product_name":   product.DisplayName,
			"payment_method": "solana",
		}
		if userEmail != nil && strings.TrimSpace(*userEmail) != "" {
			receiptData["user_email"] = strings.TrimSpace(*userEmail)
		}
		receiptNotification := &models2.NotificationQueue{
			ID:        uuid.New(),
			UserID:    userID,
			EventType: models2.NotificationOneOffPurchaseCompleted,
			Data:      receiptData,
		}
		if err := notificationQueueService.Create(ctx, receiptNotification); err != nil {
			log.WithContext(ctx).WithError(err).WithField("user_id", userID).Warn("failed to queue solana purchase receipt notification")
		} else {
			queuedNotifications = append(queuedNotifications, receiptNotification)
		}

		if err := solanaTxnRepo.MarkConfirmedByUserAndAmount(ctx, userID, price.Amount, signature); err != nil {
			log.WithContext(ctx).WithError(err).WithFields(log.Fields{
				"user_id": userID,
				"amount":  price.Amount,
			}).Warn("failed to mark solana transactions as confirmed")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if s.notificationSvc != nil {
		for _, notification := range queuedNotifications {
			if err := s.notificationSvc.DeliverEmail(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).WithFields(log.Fields{
					"notification_id": notification.ID,
					"event_type":      notification.EventType,
				}).Warn("failed to deliver solana notification email")
			}
		}
	}

	return payment, nil
}

const defaultSolanaWindowDays = 30

func resolveEntitlementNames(product *models2.Product) []string {
	if product == nil {
		return []string{"premium"}
	}
	if len(product.EntitlementsSpec) == 0 {
		return []string{"premium"}
	}
	ents := make([]string, 0, len(product.EntitlementsSpec))
	for ent := range product.EntitlementsSpec {
		ents = append(ents, ent)
	}
	return ents
}

func resolvePriceDurationDays(price *models2.Price) int {
	if price != nil && price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
		return *price.BillingCycleDays
	}
	return defaultSolanaWindowDays
}

func resolveEntitlementDuration(product *models2.Product, entitlement string, fallback int) int {
	if product == nil || len(product.EntitlementsSpec) == 0 {
		return fallback
	}
	if daysPtr, ok := product.EntitlementsSpec[entitlement]; ok {
		if daysPtr != nil && *daysPtr > 0 {
			return *daysPtr
		}
	}
	return fallback
}
