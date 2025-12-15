package subscription

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	email_api "github.com/doujins-org/doujins-billing/internal/manager/api/email"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

var errUserEmailUnavailable = errors.New("user email unavailable")

// SubscriptionEmailService handles subscription-related email notifications
// This service is called directly by other services when subscription events occur
type SubscriptionEmailService struct {
	emailService        *email_api.EmailService
	subscriptionService *SubscriptionService
	productService      *product.ProductService
	priceService        *price.PriceService
	profiles            *repo.ProfileRepo
}

// NewSubscriptionEmailService creates a new subscription email service
func NewSubscriptionEmailService(
	emailService *email_api.EmailService,
	subscriptionService *SubscriptionService,
	productService *product.ProductService,
	priceService *price.PriceService,
	profiles *repo.ProfileRepo,
) *SubscriptionEmailService {
	return &SubscriptionEmailService{
		emailService:        emailService,
		subscriptionService: subscriptionService,
		productService:      productService,
		priceService:        priceService,
		profiles:            profiles,
	}
}

// SendSubscriptionConfirmed sends a subscription confirmation email
func (s *SubscriptionEmailService) SendSubscriptionConfirmed(ctx context.Context, userID string) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping subscription confirmation email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping subscription confirmation email", userID)
			return nil
		}
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendSubscriptionConfirmation(ctx, *emailData)
}

// SendSubscriptionRenewed sends a subscription renewal email
func (s *SubscriptionEmailService) SendSubscriptionRenewed(ctx context.Context, userID string) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping subscription renewal email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping subscription renewal email", userID)
			return nil
		}
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendSubscriptionRenewal(ctx, *emailData)
}

// SendPremiumEnded sends the appropriate email when a premium entitlement ends.
func (s *SubscriptionEmailService) SendPremiumEnded(ctx context.Context, userID string, reason email_api.PremiumEndReason) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping premium-ended email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping premium-ended email", userID)
			return nil
		}
		return fmt.Errorf("failed to get email data: %w", err)
	}

	switch reason {
	case email_api.PremiumEndReasonExpired:
		return s.emailService.SendSubscriptionExpired(ctx, *emailData)
	case email_api.PremiumEndReasonChargeback, email_api.PremiumEndReasonRefund, email_api.PremiumEndReasonAdmin, email_api.PremiumEndReasonProcessor:
		return s.emailService.SendSubscriptionCancellation(ctx, *emailData, reason)
	case email_api.PremiumEndReasonUserCancel:
		fallthrough
	case email_api.PremiumEndReasonUnknown:
		return s.emailService.SendSubscriptionCancellation(ctx, *emailData, email_api.PremiumEndReasonUserCancel)
	default:
		return s.emailService.SendSubscriptionCancellation(ctx, *emailData, reason)
	}
}

// SendPaymentFailed sends a payment failure email
func (s *SubscriptionEmailService) SendPaymentFailed(ctx context.Context, userID string) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping payment failure email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping payment failure email", userID)
			return nil
		}
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendPaymentFailed(ctx, *emailData)
}

// SendEntitlementExpired sends an entitlement expiration email
func (s *SubscriptionEmailService) SendEntitlementExpired(ctx context.Context, userID string, entitlementName string, expiresAt time.Time) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping entitlement expiration email")
		return nil
	}

	username, email, err := s.getUserEmail(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping entitlement expiration email", userID)
			return nil
		}
		return fmt.Errorf("failed to get user profile: %w", err)
	}

	return s.emailService.SendEntitlementExpiration(ctx, email, username, entitlementName, expiresAt)
}

// getEmailData fetches subscription data for email notifications
func (s *SubscriptionEmailService) getEmailData(ctx context.Context, userID string) (*email_api.SubscriptionEmailData, error) {
	// Get the user's active subscription or last known subscription as fallback
	subscription, err := s.subscriptionService.GetActiveSubscription(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			subscription, err = s.subscriptionService.GetByUserID(ctx, userID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, errUserEmailUnavailable
				}
				return nil, fmt.Errorf("failed to get subscription for user %s: %w", userID, err)
			}
		} else {
			return nil, fmt.Errorf("failed to get active subscription: %w", err)
		}
	}

	var (
		username string
		email    string
	)

	if subscription.UserEmail != nil && strings.TrimSpace(*subscription.UserEmail) != "" {
		email = strings.TrimSpace(*subscription.UserEmail)
	}

	if email == "" {
		var err error
		username, email, err = s.getUserEmail(ctx, userID)
		if err != nil {
			return nil, err
		}
	}

	if email == "" {
		return nil, errUserEmailUnavailable
	}

	// Get the price details
	price, err := s.priceService.GetByID(ctx, subscription.PriceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	// Calculate billing period based on subscription and price interval
	periodStart := time.Now()
	periodEnd := time.Now()
	if subscription.CurrentPeriodStartsAt != nil {
		periodStart = *subscription.CurrentPeriodStartsAt
		if price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
			periodEnd = periodStart.AddDate(0, 0, *price.BillingCycleDays)
		} else {
			periodEnd = periodStart.AddDate(0, 1, 0) // Default to monthly for one-time purchases
		}
		if subscription.CurrentPeriodEndsAt != nil {
			periodEnd = *subscription.CurrentPeriodEndsAt
		}
	}

	paymentMethod := describePaymentMethod(subscription)
	if paymentMethod == "" {
		paymentMethod = processorDisplayName(subscription.Processor)
		if paymentMethod == "" {
			paymentMethod = "Credit Card"
		}
	}

	return &email_api.SubscriptionEmailData{
		UserEmail:      email,
		Username:       username,
		SubscriptionID: subscription.ID,
		Amount:         price.Amount,
		Currency:       price.Currency,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		PaymentMethod:  paymentMethod,
		TransactionID:  "", // Would come from payment processor
	}, nil
}

// getUserProfile gets user profile and validates email exists
func (s *SubscriptionEmailService) getUserEmail(ctx context.Context, userID string) (username string, email string, err error) {
	if s.profiles == nil {
		return "", "", errUserEmailUnavailable
	}
	uid, perr := uuid.Parse(userID)
	if perr != nil {
		return "", "", errUserEmailUnavailable
	}
	uname, mail, verified, active, qerr := s.profiles.GetUserEmail(ctx, uid)
	if qerr != nil || mail == "" || !active {
		return "", "", errUserEmailUnavailable
	}
	_ = verified // reserved for future policy checks
	return uname, mail, nil
}

func describePaymentMethod(subscription *models2.Subscription) string {
	if subscription == nil || subscription.PaymentMethod == nil {
		return ""
	}

	pm := subscription.PaymentMethod
	cardType := ""
	if pm.CardType != nil {
		cardType = strings.TrimSpace(*pm.CardType)
	}
	lastFour := ""
	if pm.LastFour != nil {
		lastFour = strings.TrimSpace(*pm.LastFour)
	}

	parts := make([]string, 0, 2)
	if cardType != "" {
		parts = append(parts, cardType)
	} else {
		friendly := processorDisplayName(pm.Processor)
		if friendly != "" {
			parts = append(parts, friendly)
		}
	}

	if lastFour != "" {
		parts = append(parts, fmt.Sprintf("••••%s", lastFour))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " ")
}

func processorDisplayName(processor models2.Processor) string {
	switch processor {
	case models2.ProcessorNMI, models2.ProcessorCCBill:
		return "Credit Card"
	case models2.ProcessorPayPal:
		return "PayPal"
	case models2.ProcessorSolana:
		return "Solana"
	default:
		clean := strings.TrimSpace(string(processor))
		if clean == "" {
			return ""
		}
		return strings.ToUpper(clean)
	}
}
