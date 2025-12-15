package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/entitlement"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"github.com/doujins-org/doujins-billing/pkg/nmi"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Sentinel errors for subscription operations
var (
	ErrSubscriptionNotFound     = errors.New("subscription not found")
	ErrSubscriptionNotActive    = errors.New("subscription is not active")
	ErrNotificationNotFound     = errors.New("notification not found")
	ErrNotificationAccessDenied = errors.New("notification does not belong to user")
)

// UserSubscriptionService handles user-facing subscription operations
type UserSubscriptionService struct {
	SubscriptionService      *subscription.SubscriptionService
	ProductService           *product.ProductService
	PriceService             *price.PriceService
	PaymentService           *payment.PaymentService
	NotificationQueueService *notification.NotificationQueueService
	EntitlementService       *entitlement.EntitlementService
	NMIClients               map[string]*nmi.NMIClient
}

// UserSubscriptionResponse represents a user's subscription with enriched data
type UserSubscriptionResponse struct {
	*models2.Subscription
	Product *models2.Product `json:"product,omitempty"`
	Price   *models2.Price   `json:"price,omitempty"`
	Access  *UserAccessGrant `json:"access,omitempty"`
}

// UserAccessGrant summarizes how the user currently has premium access (subscription vs one-off entitlement).
type UserAccessGrant struct {
	Kind                    string                         `json:"kind"`
	Entitlement             string                         `json:"entitlement"`
	SourceType              *models2.EntitlementSourceType `json:"source_type,omitempty"`
	SourceID                *uuid.UUID                     `json:"source_id,omitempty"`
	SubscriptionID          *uuid.UUID                     `json:"subscription_id,omitempty"`
	Processor               string                         `json:"processor,omitempty"`
	ProcessorSubscriptionID *string                        `json:"processor_subscription_id,omitempty"`
	StartAt                 time.Time                      `json:"start_at"`
	EndAt                   *time.Time                     `json:"end_at,omitempty"`
}

// GetUserSubscription retrieves the current subscription for a user with enriched data
func (s *UserSubscriptionService) GetUserSubscription(ctx context.Context, userID string) (*UserSubscriptionResponse, error) {
	subscription, err := s.SubscriptionService.GetActiveSubscription(ctx, userID)
	switch {
	case err == nil:
		resp := &UserSubscriptionResponse{Subscription: subscription, Access: accessFromSubscription(subscription)}
		// Enrich with price and product data if available
		if subscription.PriceID != uuid.Nil {
			if price, err := s.PriceService.GetByID(ctx, subscription.PriceID); err == nil {
				resp.Price = price

				if product, err := s.ProductService.GetByID(ctx, price.ProductID); err == nil {
					resp.Product = product
				}
			}
		}
		return resp, nil
	case errors.Is(err, sql.ErrNoRows):
		access, accessErr := s.activeEntitlementAccess(ctx, userID)
		if accessErr != nil {
			return nil, accessErr
		}
		if access != nil {
			return &UserSubscriptionResponse{Access: access}, nil
		}
		return nil, sql.ErrNoRows
	default:
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}
}

// GetUserAccessStatus composes all active access grants (subscriptions + entitlements) for a user.
func (s *UserSubscriptionService) GetUserAccessStatus(ctx context.Context, userID string) ([]*UserAccessGrant, error) {
	grants := make([]*UserAccessGrant, 0, 2)
	skipSubscriptionIDs := make(map[uuid.UUID]struct{})
	if s.SubscriptionService != nil {
		if sub, err := s.SubscriptionService.GetActiveSubscription(ctx, userID); err == nil {
			grants = append(grants, accessFromSubscription(sub))
			skipSubscriptionIDs[sub.ID] = struct{}{}
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("failed to fetch subscription access: %w", err)
		}
	}
	ents, err := s.entitlementAccessGrants(ctx, userID, skipSubscriptionIDs)
	if err != nil {
		return nil, err
	}
	grants = append(grants, ents...)
	if len(grants) == 0 {
		return nil, sql.ErrNoRows
	}
	return grants, nil
}

// GetUserSubscriptionHistory retrieves subscription history for a user
func (s *UserSubscriptionService) GetUserSubscriptionHistory(ctx context.Context, userID string, queryOpts *query.QueryOptions[subscription.GetSubscriptionsFilters]) ([]*UserSubscriptionResponse, int64, error) {
	// Set user filter
	if queryOpts.Filters.UserID == "" {
		queryOpts.Filters.UserID = userID
	}

	subscriptions, total, err := s.SubscriptionService.GetSubscribers(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get subscription history: %w", err)
	}
	queryOpts.SetTotal(total)

	responses := make([]*UserSubscriptionResponse, len(subscriptions))
	for i, sub := range subscriptions {
		responses[i] = &UserSubscriptionResponse{
			Subscription: sub,
		}

		// Enrich with price and product data if available
		if sub.PriceID != uuid.Nil {
			if price, err := s.PriceService.GetByID(ctx, sub.PriceID); err == nil {
				responses[i].Price = price

				if product, err := s.ProductService.GetByID(ctx, price.ProductID); err == nil {
					responses[i].Product = product
				}
			}
		}
	}

	return responses, total, nil
}

// GetUserPayments retrieves one-off purchases for a user
func (s *UserSubscriptionService) GetUserPayments(ctx context.Context, userID string, queryOpts *query.QueryOptions[payment.GetPaymentsFilters]) ([]*models2.Payment, int64, error) {
	// Set user filter
	if queryOpts.Filters.UserID == "" {
		queryOpts.Filters.UserID = userID
	}

	purchases, total, err := s.PaymentService.GetPayments(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
	}
	queryOpts.SetTotal(total)

	return purchases, total, nil
}

// GetUserNotifications retrieves notifications for a user
func (s *UserSubscriptionService) GetUserNotifications(ctx context.Context, userID string, queryOpts *query.QueryOptions[notification.GetNotificationsFilters]) ([]*models2.NotificationQueue, int64, error) {
	// Set user filter
	if queryOpts.Filters.UserID == "" {
		queryOpts.Filters.UserID = userID
	}

	notifications, total, err := s.NotificationQueueService.GetNotifications(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get notifications: %w", err)
	}
	queryOpts.SetTotal(total)

	return notifications, total, nil
}

// MarkNotificationRead marks a notification as read
func (s *UserSubscriptionService) MarkNotificationRead(ctx context.Context, userID string, notificationID uuid.UUID) error {
	notification, err := s.NotificationQueueService.GetByID(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotificationNotFound, err)
	}

	// Verify the notification belongs to the user
	if notification.UserID != userID {
		return ErrNotificationAccessDenied
	}

	notification.MarkAsSeen() // Mark as seen (new boolean field)
	return s.NotificationQueueService.Update(ctx, notification)
}

// CancelUserSubscription cancels a user's subscription
func (s *UserSubscriptionService) CancelUserSubscription(ctx context.Context, userID string, feedback string) error {
	subscription, err := s.SubscriptionService.GetActiveSubscription(ctx, userID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSubscriptionNotFound, err)
	}

	if subscription.Processor != models2.ProcessorNMI {
		return fmt.Errorf("unable to cancel subscription for processor %s", subscription.Processor)
	}

	// End entitlements for this subscription now
	// if s.EntitlementService != nil {
	// 	reason := models.EntitlementRevokeAdmin
	// 	if err := s.EntitlementService.EndActiveBySubscription(ctx, subscription.ID, now, &reason); err != nil {
	// 		log.WithFields(log.Fields{
	// 			"subscription_id": subscription.ID,
	// 			"user_id":         userID,
	// 			"error":           err.Error(),
	// 		}).Error("Failed to end entitlements during subscription cancellation")
	// 	}
	// }

	// Cancel subscription with NMI
	if s.NMIClients != nil {
		provider := ""
		if subscription.ProcessorProvider != nil {
			provider = strings.ToLower(strings.TrimSpace(*subscription.ProcessorProvider))
		}
		if provider == "" && subscription.Price != nil && subscription.Price.NMIProvider != nil {
			provider = strings.ToLower(strings.TrimSpace(*subscription.Price.NMIProvider))
		}
		if provider == "" {
			provider = "mobius"
		}

		if client, ok := s.NMIClients[provider]; ok && subscription.Price != nil && subscription.Price.NMIPlanID != nil {
			if err := client.DeleteRecurringSubscription(*subscription.Price.NMIPlanID); err != nil {
				return fmt.Errorf("failed to cancel subscription with NMI provider '%s': %w", provider, err)
			}
		}
	}

	// Add notification
	notification := &models2.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: models2.NotificationPremiumEnded,
		Data:      map[string]any{"reason": string(subscription.PremiumEndReasonUserCancel)},
	}
	if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
		log.WithFields(log.Fields{
			"subscription_id":   subscription.ID,
			"user_id":           userID,
			"notification_type": notification.EventType,
			"error":             err.Error(),
		}).Error("Failed to create notification during subscription cancellation")
	}

	return nil
}

func accessFromSubscription(sub *models2.Subscription) *UserAccessGrant {
	grant := &UserAccessGrant{
		Kind:        "subscription",
		Entitlement: "premium",
		Processor:   string(sub.Processor),
	}
	if subID := sub.ID; subID != uuid.Nil {
		grant.SubscriptionID = &subID
	}
	if sub.ProcessorSubscriptionID != "" {
		psid := sub.ProcessorSubscriptionID
		grant.ProcessorSubscriptionID = &psid
	}
	if sub.CurrentPeriodStartsAt != nil && !sub.CurrentPeriodStartsAt.IsZero() {
		grant.StartAt = *sub.CurrentPeriodStartsAt
	} else {
		grant.StartAt = sub.StartedAt
	}
	if sub.CurrentPeriodEndsAt != nil && !sub.CurrentPeriodEndsAt.IsZero() {
		grant.EndAt = sub.CurrentPeriodEndsAt
	}
	return grant
}

func (s *UserSubscriptionService) activeEntitlementAccess(ctx context.Context, userID string) (*UserAccessGrant, error) {
	grants, err := s.entitlementAccessGrants(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	if len(grants) > 0 {
		return grants[0], nil
	}
	return nil, nil
}

func (s *UserSubscriptionService) entitlementAccessGrants(ctx context.Context, userID string, skipSubs map[uuid.UUID]struct{}) ([]*UserAccessGrant, error) {
	if s.EntitlementService == nil {
		return nil, nil
	}
	ents, err := s.EntitlementService.ListActiveRecords(ctx, userID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to list entitlements: %w", err)
	}
	grants := make([]*UserAccessGrant, 0, len(ents))
	for _, ent := range ents {
		if ent.Entitlement == "" {
			continue
		}
		if ent.SourceType == models2.EntitlementSourceSubscription && ent.SourceID != nil {
			if _, ok := skipSubs[*ent.SourceID]; ok {
				continue
			}
		}
		grant := &UserAccessGrant{
			Kind:        "entitlement",
			Entitlement: ent.Entitlement,
			StartAt:     ent.StartAt,
			EndAt:       ent.EndAt,
		}
		if ent.SourceType != "" {
			src := ent.SourceType
			grant.SourceType = &src
			if ent.SourceType == models2.EntitlementSourceSubscription && ent.SourceID != nil {
				grant.SubscriptionID = ent.SourceID
			}
		}
		if ent.SourceID != nil {
			grant.SourceID = ent.SourceID
		}
		grants = append(grants, grant)
	}
	return grants, nil
}

// NewUserSubscriptionService creates a new UserSubscriptionService
func NewUserSubscriptionService(
	subscriptionService *subscription.SubscriptionService,
	productService *product.ProductService,
	priceService *price.PriceService,
	paymentService *payment.PaymentService,
	notificationQueueService *notification.NotificationQueueService,
	entitlementService *entitlement.EntitlementService,
	nmiClients map[string]*nmi.NMIClient,
) *UserSubscriptionService {
	return &UserSubscriptionService{
		NMIClients:               nmiClients,
		SubscriptionService:      subscriptionService,
		ProductService:           productService,
		PriceService:             priceService,
		PaymentService:           paymentService,
		NotificationQueueService: notificationQueueService,
		EntitlementService:       entitlementService,
	}
}
