package notification

import (
	"context"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/email"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/pkg/db/models"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/pkg/query"
)

// NotificationService handles both db storage and immediate external delivery
type NotificationService struct {
	notificationService  *NotificationQueueService
	subscriptionEmailSvc *subscription.SubscriptionEmailService
	emailService         *email.EmailService
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	notificationService *NotificationQueueService,
	subscriptionEmailSvc *subscription.SubscriptionEmailService,
	emailService *email.EmailService,
) *NotificationService {
	return &NotificationService{
		notificationService:  notificationService,
		subscriptionEmailSvc: subscriptionEmailSvc,
		emailService:         emailService,
	}
}

// CreateAndDeliver creates a notification in the db and immediately sends external notifications
func (s *NotificationService) CreateAndDeliver(ctx context.Context, notification *models.NotificationQueue) error {
	// 1. Clean up obsolete notifications before creating the new one
	if err := s.cleanupObsoleteNotifications(ctx, notification); err != nil {
		log.WithContext(ctx).WithError(err).Error("failed to cleanup obsolete notifications")
		// Continue with notification creation even if cleanup fails
	}

	// 2. Store in db
	if err := s.notificationService.Create(ctx, notification); err != nil {
		log.WithContext(ctx).WithError(err).Error("failed to create notification in db")
		return fmt.Errorf("failed to create notification: %w", err)
	}

	// 3. Immediately send external notifications (email, discord, etc.)
	if err := s.deliverExternalNotifications(ctx, notification); err != nil {
		// Log error but don't fail the entire operation since notification is already stored
		log.WithContext(ctx).WithError(err).WithFields(log.Fields{
			"notification_id": notification.ID,
			"user_id":         notification.UserID,
			"event_type":      notification.EventType,
		}).Error("failed to deliver external notifications")
	}

	return nil
}

// deliverExternalNotifications sends immediate external notifications (email, discord)
func (s *NotificationService) deliverExternalNotifications(ctx context.Context, notification *models.NotificationQueue) error {
	// Send email notification based on event type
	if err := s.sendEmailNotification(ctx, notification); err != nil {
		return fmt.Errorf("failed to send email notification: %w", err)
	}

	return nil
}

// DeliverEmail sends the appropriate email for an already-created notification
func (s *NotificationService) DeliverEmail(ctx context.Context, notification *models.NotificationQueue) error {
	return s.sendEmailNotification(ctx, notification)
}

// sendEmailNotification sends appropriate email based on notification type

func (s *NotificationService) sendEmailNotification(ctx context.Context, notification *models.NotificationQueue) error {
	switch notification.EventType {
	case models.NotificationPremiumStarted:
		if s.subscriptionEmailSvc == nil {
			log.WithContext(ctx).Debug("subscription email service not available - skipping subscription confirmation email")
			return nil
		}
		return s.subscriptionEmailSvc.SendSubscriptionConfirmed(ctx, notification.UserID)

	case models.NotificationPremiumRenewed:
		if s.subscriptionEmailSvc == nil {
			log.WithContext(ctx).Debug("subscription email service not available - skipping subscription renewal email")
			return nil
		}
		return s.subscriptionEmailSvc.SendSubscriptionRenewed(ctx, notification.UserID)

	case models.NotificationPremiumEnded:
		if s.subscriptionEmailSvc == nil {
			log.WithContext(ctx).Debug("subscription email service not available - skipping subscription cancellation email")
			return nil
		}
		reason := email.PremiumEndReasonUnknown
		if notification.Data != nil {
			if r, ok := notification.Data["reason"].(string); ok {
				reason = email.ParsePremiumEndReason(r)
			}
		}
		return s.subscriptionEmailSvc.SendPremiumEnded(ctx, notification.UserID, reason)

	case models.NotificationPaymentMethodFailed:
		if s.subscriptionEmailSvc == nil {
			log.WithContext(ctx).Debug("subscription email service not available - skipping payment failure email")
			return nil
		}
		return s.subscriptionEmailSvc.SendPaymentFailed(ctx, notification.UserID)

	case models.NotificationOneOffPurchaseCompleted:
		if s.emailService == nil {
			log.WithContext(ctx).Debug("email service not available - skipping one-off purchase receipt email")
			return nil
		}

		if notification.Data == nil {
			log.WithContext(ctx).WithField("user_id", notification.UserID).Warn("one-off purchase notification missing data payload")
			return nil
		}

		email, _ := notification.Data["user_email"].(string)
		if email == "" && s.subscriptionEmailSvc != nil {
			// Fallback to profiles lookup via subscription email service
			if uname, mail, err := s.subscriptionEmailSvc.getUserEmail(ctx, notification.UserID); err == nil && mail != "" {
				_ = uname
				email = mail
			}
		}
		if email == "" {
			log.WithContext(ctx).WithField("user_id", notification.UserID).Warn("one-off purchase notification missing user email and profile lookup failed")
			return nil
		}

		amount, _ := notification.Data["amount"].(float64)
		currency, _ := notification.Data["currency"].(string)
		productName, _ := notification.Data["product_name"].(string)
		paymentMethod, _ := notification.Data["payment_method"].(string)

		return s.emailService.SendOneOffPurchaseReceipt(ctx, email.OneOffPurchaseEmailData{
			UserEmail:     email,
			Amount:        amount,
			Currency:      currency,
			ProductName:   productName,
			PaymentMethod: paymentMethod,
			IsPremium:     true,
		})

	case models.NotificationPaymentMethodAutoUpdated:
		// No specific email for auto-updated payment methods - they're informational
		log.WithContext(ctx).Debug("payment method auto-updated - no email sent")
		return nil

	case models.NotificationPaymentMethodUpdateRequired:
		// This could trigger a payment method update email if we had one
		log.WithContext(ctx).Debug("payment method update required - no specific email template")
		return nil

	case models.NotificationSystemAlert:
		// System alerts are typically for admins, not user emails
		log.WithContext(ctx).Debug("system alert - no user email sent")
		return nil

	default:
		log.WithContext(ctx).WithField("event_type", notification.EventType).Warn("unknown notification event type for email delivery")
		return nil
	}
}

// cleanupObsoleteNotifications removes notifications that become irrelevant when user status improves
func (s *NotificationService) cleanupObsoleteNotifications(ctx context.Context, newNotification *models.NotificationQueue) error {
	var obsoleteEventTypes []models.NotificationEventType

	// Determine which notification types to clean up based on the new notification
	switch newNotification.EventType {
	case models.NotificationPremiumStarted, models.NotificationPremiumRenewed:
		// When premium starts or renews, remove any pending "premium ended" notifications
		obsoleteEventTypes = []models.NotificationEventType{
			models.NotificationPremiumEnded,
		}

	case models.NotificationPaymentMethodAutoUpdated:
		// When payment method is auto-updated, remove payment-related failure notifications
		obsoleteEventTypes = []models.NotificationEventType{
			models.NotificationPaymentMethodFailed,
			models.NotificationPaymentMethodUpdateRequired,
		}

	// Note: We could add more logic here for other success scenarios
	// For example, if we had a "payment succeeded" notification type, it could clean up payment failure notifications
	default:
		// No cleanup needed for this notification type
		return nil
	}

	// Remove obsolete unseen notifications for this user
	if len(obsoleteEventTypes) > 0 {
		cleanedCount, err := s.removeObsoleteNotifications(ctx, newNotification.UserID, obsoleteEventTypes)
		if err != nil {
			return fmt.Errorf("failed to remove obsolete notifications: %w", err)
		}

		if cleanedCount > 0 {
			log.WithContext(ctx).WithFields(log.Fields{
				"user_id":        newNotification.UserID,
				"new_event_type": newNotification.EventType,
				"cleaned_count":  cleanedCount,
				"obsolete_types": obsoleteEventTypes,
			}).Info("cleaned up obsolete notifications due to status improvement")
		}
	}

	return nil
}

// removeObsoleteNotifications removes unseen notifications of specified types for a user
func (s *NotificationService) removeObsoleteNotifications(ctx context.Context, userID string, eventTypes []models.NotificationEventType) (int, error) {
	// Get unseen notifications for this user that match the obsolete types
	falseVal := false
	notifications, _, err := s.notificationService.GetNotifications(ctx, query.QueryOptions[GetNotificationsFilters]{
		Filters: GetNotificationsFilters{
			UserID: userID,
			Seen:   &falseVal, // Only unseen notifications
		},
		Page:     1,
		PageSize: 1000, // Get a large batch to clean up
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get notifications for cleanup: %w", err)
	}

	cleanedCount := 0
	for _, notification := range notifications {
		// Check if this notification's type is in the obsolete list
		for _, obsoleteType := range eventTypes {
			if notification.EventType == obsoleteType {
				if err := s.notificationService.Delete(ctx, notification.ID); err != nil {
					log.WithContext(ctx).WithError(err).WithFields(log.Fields{
						"notification_id": notification.ID,
						"event_type":      notification.EventType,
					}).Error("failed to delete obsolete notification")
				} else {
					cleanedCount++
				}
				break
			}
		}
	}

	return cleanedCount, nil
}
