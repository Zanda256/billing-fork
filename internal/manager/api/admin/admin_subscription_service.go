package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/manager/api/email"
	"github.com/doujins-org/doujins-billing/internal/manager/api/entitlement"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	subs "github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/api/user"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Additional sentinel errors for admin operations
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleGrantNotFound = errors.New("role grant not found")
)

// AdminSubscriptionService handles administrative subscription operations
type AdminSubscriptionService struct {
	SubscriptionService      *subs.SubscriptionService
	ProductService           *product.ProductService
	PriceService             *price.PriceService
	EntitlementService       *entitlement.EntitlementService
	NotificationQueueService *notification.NotificationQueueService
	PaymentService           *payment.PaymentService
	// No user directory enrichment; IdP subject is stored on subscription
}

// AdminSubscriptionResponse represents a subscription with enriched admin data
type AdminSubscriptionResponse struct {
	*models2.Subscription
	Product *models2.Product `json:"product,omitempty"`
	Price   *models2.Price   `json:"price,omitempty"`
}

// GetAllSubscriptions retrieves all subscriptions with filtering (admin)
func (s *AdminSubscriptionService) GetAllSubscriptions(ctx context.Context, queryOpts *query.QueryOptions[subs.GetSubscriptionsFilters]) ([]*AdminSubscriptionResponse, int64, error) {
	subscriptions, total, err := s.SubscriptionService.GetSubscribers(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get subscriptions: %w", err)
	}

	responses := make([]*AdminSubscriptionResponse, len(subscriptions))
	for i, sub := range subscriptions {
		responses[i] = &AdminSubscriptionResponse{
			Subscription: sub,
		}

		// No user enrichment (IdP-managed)

		// Enrich with price and product data if available
		if price, err := s.PriceService.GetByID(ctx, sub.PriceID); err == nil {
			responses[i].Price = price

			if product, err := s.ProductService.GetByID(ctx, price.ProductID); err == nil {
				responses[i].Product = product
			}
		}
	}

	return responses, total, nil
}

// GetSubscriptionByID retrieves a specific subscription with full details (admin)
func (s *AdminSubscriptionService) GetSubscriptionByID(ctx context.Context, subscriptionID uuid.UUID) (*AdminSubscriptionResponse, error) {
	subscription, err := s.SubscriptionService.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("subscription not found: %w", err)
	}

	response := &AdminSubscriptionResponse{
		Subscription: subscription,
	}

	// Enrich with price and product data if available
	if price, err := s.PriceService.GetByID(ctx, subscription.PriceID); err == nil {
		response.Price = price

		if product, err := s.ProductService.GetByID(ctx, price.ProductID); err == nil {
			response.Product = product
		}
	}

	// No user enrichment (IdP-managed)

	return response, nil
}

// UpdateSubscription updates a subscription (admin)
func (s *AdminSubscriptionService) UpdateSubscription(ctx context.Context, subscriptionID uuid.UUID, updates map[string]any) error {
	subscription, err := s.SubscriptionService.GetByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	// Apply allowed updates
	for field, value := range updates {
		switch field {
		case "status":
			if status, ok := value.(models2.SubscriptionStatus); ok {
				subscription.Status = status
			}
		case "notes":
			if notes, ok := value.(string); ok {
				// Store notes in Metadata JSONB field which is designed for additional metadata
				var responseData map[string]any
				if subscription.Metadata != nil {
					if err := json.Unmarshal(subscription.Metadata, &responseData); err != nil {
						responseData = make(map[string]any)
					}
				} else {
					responseData = make(map[string]any)
				}
				responseData["admin_notes"] = notes
				if newData, err := json.Marshal(responseData); err == nil {
					subscription.Metadata = newData
				}
			}
		}
	}

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}

// CancelSubscription cancels a subscription (admin)
func (s *AdminSubscriptionService) CancelSubscription(ctx context.Context, subscriptionID uuid.UUID, reason string) error {
	subscription, err := s.SubscriptionService.GetByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	if subscription.Status != models2.StatusActive {
		return fmt.Errorf("subscription is not active")
	}

	now := time.Now()
	cancelType := models2.CancelTypeMerchant
	subscription.Status = models2.StatusCancelled
	subscription.CancelledAt = &now
	subscription.CancelType = &cancelType
	if reason != "" {
		subscription.CancelFeedback = &reason
	}

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// End entitlements for this subscription now
	if s.EntitlementService != nil {
		reason := models2.EntitlementRevokeAdmin
		if err := s.EntitlementService.EndActiveBySubscription(ctx, subscription.ID, now, &reason); err != nil {
			log.WithFields(log.Fields{
				"subscription_id": subscription.ID,
				"user_id":         subscription.UserID,
				"error":           err.Error(),
			}).Error("Failed to end entitlements during admin subscription operation")
		}
	}

	// Add notification
	notification := &models2.NotificationQueue{
		ID:        uuid.New(),
		UserID:    subscription.UserID,
		EventType: models2.NotificationPremiumEnded,
		Data:      map[string]any{"reason": string(email.PremiumEndReasonAdmin)},
	}
	if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
		log.WithFields(log.Fields{
			"subscription_id":   subscription.ID,
			"user_id":           subscription.UserID,
			"notification_type": notification.EventType,
			"error":             err.Error(),
		}).Error("Failed to create notification during admin subscription operation")
	}

	return nil
}

// CancelUserSubscription cancels a user's subscription by user ID (admin)
func (s *AdminSubscriptionService) CancelUserSubscription(ctx context.Context, userID string, reason string) error {
	subscription, err := s.SubscriptionService.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("%w: %w", user.ErrSubscriptionNotFound, err)
	}

	if subscription.Status != models2.StatusActive {
		return user.ErrSubscriptionNotActive
	}

	now := time.Now()
	cancelType := models2.CancelTypeMerchant // Admin cancellation
	subscription.Status = models2.StatusCancelled
	subscription.CancelledAt = &now
	subscription.CancelType = &cancelType
	if reason != "" {
		subscription.CancelFeedback = &reason
	}

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// End entitlements now
	if s.EntitlementService != nil {
		reason := models2.EntitlementRevokeAdmin
		if err := s.EntitlementService.EndActiveBySubscription(ctx, subscription.ID, now, &reason); err != nil {
			log.WithFields(log.Fields{
				"subscription_id": subscription.ID,
				"user_id":         subscription.UserID,
				"error":           err.Error(),
			}).Error("Failed to end entitlements during admin subscription operation")
		}
	}

	// Add notification
	notification := &models2.NotificationQueue{
		ID:        uuid.New(),
		UserID:    subscription.UserID,
		EventType: models2.NotificationPremiumEnded,
		Data:      map[string]any{"reason": string(email.PremiumEndReasonAdmin)},
	}
	if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
		log.WithFields(log.Fields{
			"subscription_id":   subscription.ID,
			"user_id":           subscription.UserID,
			"notification_type": notification.EventType,
			"error":             err.Error(),
		}).Error("Failed to create notification during admin subscription operation")
	}

	return nil
}

// ExtendSubscription extends a subscription period (admin)
func (s *AdminSubscriptionService) ExtendSubscription(ctx context.Context, subscriptionID uuid.UUID, days int, reason string) error {
	subscription, err := s.SubscriptionService.GetByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	if subscription.Status != models2.StatusActive {
		return fmt.Errorf("subscription is not active")
	}

	// Extend the subscription period
	extension := time.Duration(days) * 24 * time.Hour

	if subscription.CurrentPeriodEndsAt != nil {
		newEndTime := subscription.CurrentPeriodEndsAt.Add(extension)
		subscription.CurrentPeriodEndsAt = &newEndTime
	} else {
		newEndTime := time.Now().Add(extension)
		subscription.CurrentPeriodEndsAt = &newEndTime
		startTime := time.Now()
		subscription.CurrentPeriodStartsAt = &startTime
	}

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Admin actions don't generate user notifications - this is purely for admin operations

	return nil
}

// CreateProduct creates a new product (admin)
func (s *AdminSubscriptionService) CreateProduct(ctx context.Context, product *models2.Product) error {
	product.ID = uuid.New()
	return s.ProductService.Create(ctx, product)
}

// UpdateProduct updates a product (admin)
func (s *AdminSubscriptionService) UpdateProduct(ctx context.Context, productID uuid.UUID, updates map[string]any) error {
	product, err := s.ProductService.GetByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("product not found: %w", err)
	}

	// Apply allowed updates
	for field, value := range updates {
		switch field {
		case "name":
			if name, ok := value.(string); ok {
				product.DisplayName = name
			}
		case "description":
			if desc, ok := value.(string); ok {
				product.Description = desc
			}
		case "is_active":
			if active, ok := value.(bool); ok {
				product.IsActive = active
			}
		}
	}

	return s.ProductService.Update(ctx, product)
}

// CreatePrice creates a new price (admin)
func (s *AdminSubscriptionService) CreatePrice(ctx context.Context, price *models2.Price) error {
	price.ID = uuid.New()
	return s.PriceService.Create(ctx, price)
}

// GetAllUserRoleGrants retrieves all user role grants with filtering (admin)
// Deprecated: role grants endpoints removed; use entitlements instead.

// CreateManualRoleGrant creates a manual role grant (admin)
func (s *AdminSubscriptionService) CreateManualRoleGrant(ctx context.Context, userID string, roleID uuid.UUID, durationDays *int) error {
	// Deprecated: role grants. Use entitlement 'premium' instead.
	if s.EntitlementService == nil {
		return nil
	}
	now := time.Now()
	var endAt *time.Time
	if durationDays != nil && *durationDays > 0 {
		e := now.Add(time.Duration(*durationDays) * 24 * time.Hour)
		endAt = &e
	}
	_, err := s.EntitlementService.GrantWindow(ctx, userID, "premium", now, endAt, models2.EntitlementSourceAdmin, nil)
	return err
}

// VerifyPayPalPurchase verifies a PayPal payment and grants the associated role (admin)
// This is used when admins manually verify PayPal payments outside the automated system
func (s *AdminSubscriptionService) VerifyPayPalPurchase(ctx context.Context, userID string, priceID uuid.UUID, paypalTransactionID string) error {
	// Get price information
	price, err := s.PriceService.GetByID(ctx, priceID)
	if err != nil {
		return fmt.Errorf("failed to get price: %w", err)
	}

	// Get product information to determine role
	product, err := s.ProductService.GetByID(ctx, price.ProductID)
	if err != nil {
		return fmt.Errorf("failed to get product: %w", err)
	}

	// Create purchase record for audit trail
	// Only grant role if product has one (handle nullable RoleID)
	// Determine entitlements and durations
	type grantItem struct {
		name string
		days int
	}
	var grants []grantItem
	if len(product.EntitlementsSpec) > 0 {
		for name, d := range product.EntitlementsSpec {
			days := 0
			if d != nil {
				days = *d
			}
			if days <= 0 {
				days = 30
			}
			grants = append(grants, grantItem{name, days})
		}
	} else {
		grants = append(grants, grantItem{"premium", 30})
	}
	// Disallow one-off if any relevant entitlement has an active indefinite window
	if s.EntitlementService != nil {
		for _, g := range grants {
			exists, err := s.EntitlementService.HasActiveIndefinite(ctx, userID, g.name, time.Now())
			if err != nil {
				return fmt.Errorf("failed entitlement check: %w", err)
			}
			if exists {
				return fmt.Errorf("one-off purchase not allowed while subscription entitlement '%s' is active", g.name)
			}
		}
	}

	// Create purchase record (no role-grant linkage)
	purchase := &models2.Payment{
		ID:            uuid.New(),
		UserID:        userID,
		PriceID:       priceID,
		Processor:     models2.ProcessorPayPal,
		TransactionID: paypalTransactionID,
		Amount:        price.Amount,
		Currency:      price.Currency,
		PurchasedAt:   time.Now(),
		CreatedAt:     time.Now(),
	}
	if err := s.PaymentService.Create(ctx, purchase); err != nil {
		return fmt.Errorf("failed to create purchase record: %w", err)
	}
	// Grant entitlements by appending to avoid overlap
	if s.EntitlementService != nil {
		for _, g := range grants {
			if _, err := s.EntitlementService.AppendEntitlementDays(ctx, userID, g.name, g.days, models2.EntitlementSourceOneOff, &purchase.ID); err != nil {
				return fmt.Errorf("failed to grant entitlement %s: %w", g.name, err)
			}
		}
	}

	return nil
}

// RevokeRoleGrant revokes a role grant (admin)
// Deprecated: role grants endpoints removed; use entitlements instead.

// GetAllPurchases retrieves all purchases with filtering (admin)
func (s *AdminSubscriptionService) GetAllPurchases(ctx context.Context, queryOpts *query.QueryOptions[payment.GetPaymentsFilters]) ([]*models2.Payment, int64, error) {
	purchases, total, err := s.PaymentService.GetPayments(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
	}

	return purchases, total, nil
}

// GetAllNotifications retrieves all notifications with filtering (admin)
func (s *AdminSubscriptionService) GetAllNotifications(ctx context.Context, queryOpts *query.QueryOptions[notification.GetNotificationsFilters]) ([]*models2.NotificationQueue, int64, error) {
	notifications, total, err := s.NotificationQueueService.GetNotifications(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get notifications: %w", err)
	}

	return notifications, total, nil
}

// SendManualNotification sends a manual notification (admin)
func (s *AdminSubscriptionService) SendManualNotification(ctx context.Context, userID string, eventType models2.NotificationEventType, message string) error {
	notification := &models2.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: eventType,
		Data: map[string]any{
			"message": message,
			"source":  "admin_manual",
		},
	}

	return s.NotificationQueueService.Create(ctx, notification)
}

// NewAdminSubscriptionService creates a new AdminSubscriptionService
func NewAdminSubscriptionService(
	subscriptionService *subs.SubscriptionService,
	productService *product.ProductService,
	priceService *price.PriceService,
	entitlementService *entitlement.EntitlementService,
	notificationQueueService *notification.NotificationQueueService,
	paymentService *payment.PaymentService,
) *AdminSubscriptionService {
	return &AdminSubscriptionService{
		SubscriptionService:      subscriptionService,
		ProductService:           productService,
		PriceService:             priceService,
		EntitlementService:       entitlementService,
		NotificationQueueService: notificationQueueService,
		PaymentService:           paymentService,
	}
}
