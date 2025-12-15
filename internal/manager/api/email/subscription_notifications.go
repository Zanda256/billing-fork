package email

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SubscriptionEmailData contains data for subscription-related emails
type PremiumEndReason string

const (
	PremiumEndReasonUserCancel PremiumEndReason = "user_cancel"
	PremiumEndReasonExpired    PremiumEndReason = "expired"
	PremiumEndReasonChargeback PremiumEndReason = "chargeback"
	PremiumEndReasonRefund     PremiumEndReason = "refund"
	PremiumEndReasonAdmin      PremiumEndReason = "admin"
	PremiumEndReasonProcessor  PremiumEndReason = "processor_cancel"
	PremiumEndReasonUnknown    PremiumEndReason = "unknown"
)

func ParsePremiumEndReason(value string) PremiumEndReason {
	switch strings.ToLower(value) {
	case string(PremiumEndReasonUserCancel):
		return PremiumEndReasonUserCancel
	case string(PremiumEndReasonExpired):
		return PremiumEndReasonExpired
	case string(PremiumEndReasonChargeback):
		return PremiumEndReasonChargeback
	case string(PremiumEndReasonRefund):
		return PremiumEndReasonRefund
	case string(PremiumEndReasonAdmin):
		return PremiumEndReasonAdmin
	case string(PremiumEndReasonProcessor):
		return PremiumEndReasonProcessor
	default:
		return PremiumEndReasonUnknown
	}
}

// SendSubscriptionConfirmation sends a confirmation email when subscription is created
func (s *EmailService) SendSubscriptionConfirmation(ctx context.Context, data SubscriptionEmailData) error {
	subject := "Welcome to Doujins Premium! Your subscription is confirmed"

	htmlContent := fmt.Sprintf(`
		<h2>Welcome to Doujins Premium!</h2>
		<p>Hi %s,</p>
		<p>Your premium subscription has been successfully activated. Thank you for supporting Doujins!</p>
		
		<h3>Subscription Details:</h3>
		<ul>
			<li><strong>Subscription ID:</strong> %s</li>
			<li><strong>Amount:</strong> $%.2f %s</li>
			<li><strong>Current Period:</strong> %s to %s</li>
			<li><strong>Payment Method:</strong> %s</li>
		</ul>
		
		<p>You now have access to:</p>
		<ul>
			<li>Exclusive premium content</li>
			<li>High-definition streaming</li>
			<li>Priority support</li>
			<li>Ad-free browsing experience</li>
		</ul>
		
		<p>Enjoy your premium experience!</p>
		<p>The Doujins Team</p>
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency,
		data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"), data.PaymentMethod)

	plainContent := fmt.Sprintf(`
		Welcome to Doujins Premium!
		
		Hi %s,
		
		Your premium subscription has been successfully activated. Thank you for supporting Doujins!
		
		Subscription Details:
		- Subscription ID: %s
		- Amount: $%.2f %s
		- Current Period: %s to %s
		- Payment Method: %s
		
		You now have access to exclusive premium content, HD streaming, priority support, and ad-free browsing.
		
		Enjoy your premium experience!
		The Doujins Team
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency,
		data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"), data.PaymentMethod)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

// SendSubscriptionRenewal sends notification when subscription is renewed
func (s *EmailService) SendSubscriptionRenewal(ctx context.Context, data SubscriptionEmailData) error {
	subject := "Your Doujins Premium subscription has been renewed"

	htmlContent := fmt.Sprintf(`
		<h2>Subscription Renewed Successfully</h2>
		<p>Hi %s,</p>
		<p>Your Doujins Premium subscription has been automatically renewed. Thank you for your continued support!</p>
		
		<h3>Renewal Details:</h3>
		<ul>
			<li><strong>Subscription ID:</strong> %s</li>
			<li><strong>Amount Charged:</strong> $%.2f %s</li>
			<li><strong>New Period:</strong> %s to %s</li>
			<li><strong>Transaction ID:</strong> %s</li>
		</ul>
		
		<p>Your premium access continues uninterrupted. Enjoy exclusive content and features!</p>
		<p>The Doujins Team</p>
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency,
		data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"), data.TransactionID)

	plainContent := fmt.Sprintf(`
		Subscription Renewed Successfully
		
		Hi %s,
		
		Your Doujins Premium subscription has been automatically renewed. Thank you for continued support!
		
		Renewal Details:
		- Subscription ID: %s
		- Amount Charged: $%.2f %s
		- New Period: %s to %s
		- Transaction ID: %s
		
		Your premium access continues uninterrupted.
		
		The Doujins Team
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency,
		data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"), data.TransactionID)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

// SendSubscriptionCancellation sends notification when subscription is cancelled
func (s *EmailService) SendSubscriptionCancellation(ctx context.Context, data SubscriptionEmailData, reason PremiumEndReason) error {
	periodEnd := data.PeriodEnd.Format("Jan 2, 2006")
	subject := "Your Doujins Premium subscription has been cancelled"
	reasonBlurb := "We've cancelled your premium membership as requested."
	footer := "You can resubscribe at any time to regain premium access. We'd love to see you back!"

	switch reason {
	case PremiumEndReasonChargeback:
		subject = "Your Doujins Premium subscription has been terminated"
		reasonBlurb = "We received a dispute on your most recent payment, so the membership has been closed for now."
		footer = "If this was unexpected, please reach out to support so we can help restore your access."
	case PremiumEndReasonRefund:
		subject = "Your Doujins Premium subscription was refunded"
		reasonBlurb = "We've processed your refund and closed the associated premium membership."
	case PremiumEndReasonAdmin:
		subject = "Your Doujins Premium subscription was cancelled"
		reasonBlurb = "Our support team closed this subscription."
	case PremiumEndReasonProcessor:
		subject = "Your Doujins Premium subscription was cancelled"
		reasonBlurb = "Your payment provider confirmed this cancellation, so we’ve closed the membership."
	}

	htmlContent := fmt.Sprintf(`
		<h2>%s</h2>
		<p>Hi %s,</p>
		<p>%s</p>
		<ul>
			<li><strong>Subscription ID:</strong> %s</li>
			<li><strong>Premium access available until:</strong> %s</li>
		</ul>
		<p>You'll continue to enjoy premium access until %s.</p>
		<p>%s</p>
		<p>The Doujins Team</p>
	`, subject, data.Username, reasonBlurb, data.SubscriptionID, periodEnd, periodEnd, footer)

	plainContent := fmt.Sprintf(`
		%s
		
		Hi %s,
		
		%s
		
		Subscription ID: %s
		Premium access available until: %s
		
		You'll continue to enjoy premium access until %s.
		
		%s
		The Doujins Team
	`, subject, data.Username, reasonBlurb, data.SubscriptionID, periodEnd, periodEnd, footer)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

func (s *EmailService) SendSubscriptionExpired(ctx context.Context, data SubscriptionEmailData) error {
	periodEnd := data.PeriodEnd.Format("Jan 2, 2006")
	subject := "Your Doujins Premium access has expired"

	htmlContent := fmt.Sprintf(`
		<h2>Your Premium Access Has Expired</h2>
		<p>Hi %s,</p>
		<p>We tried to renew your Doujins Premium subscription several times but couldn’t complete the payment. Your premium access ended on <strong>%s</strong>.</p>
		<p>If you’d like to jump back in, update your payment method and restart your membership any time.</p>
		<p><a href="https://doujins.com/account/billing" style="display:inline-block;padding:10px 18px;background:#6c4ad0;color:#ffffff;text-decoration:none;border-radius:4px;">Manage billing settings</a></p>
		<p>The Doujins Team</p>
	`, data.Username, periodEnd)

	plainContent := fmt.Sprintf(`
		Your Premium Access Has Expired
		
		Hi %s,
		
		We tried to renew your Doujins Premium subscription several times but couldn’t complete the payment. Your premium access ended on %s.
		
		Update your payment method anytime to restart your membership: https://doujins.com/account/billing
		
		The Doujins Team
	`, data.Username, periodEnd)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

// SendPaymentFailed sends notification when subscription payment fails
func (s *EmailService) SendPaymentFailed(ctx context.Context, data SubscriptionEmailData) error {
	amountLine := fmt.Sprintf("$%.2f %s", data.Amount, data.Currency)
	subject := "We couldn’t renew your Doujins Premium subscription"

	htmlContent := fmt.Sprintf(`
		<h2>Payment Attempt Unsuccessful</h2>
		<p>Hi %s,</p>
		<p>We just tried to renew your Doujins Premium subscription but the payment didn’t go through.</p>
		<ul>
			<li><strong>Subscription ID:</strong> %s</li>
			<li><strong>Amount:</strong> %s</li>
			<li><strong>Payment method:</strong> %s</li>
		</ul>
		<p>Your premium access stays active while we retry automatically. To be safe, please take a moment to update your payment details.</p>
		<p><a href="https://doujins.com/account/billing" style="display:inline-block;padding:10px 18px;background:#6c4ad0;color:#ffffff;text-decoration:none;border-radius:4px;">Update payment method</a></p>
		<p>If payment continues to fail, your membership will expire.</p>
		<p>The Doujins Team</p>
	`, data.Username, data.SubscriptionID, amountLine, data.PaymentMethod)

	plainContent := fmt.Sprintf(`
		We couldn’t renew your Doujins Premium subscription
		
		Hi %s,
		
		We just tried to renew your Doujins Premium subscription but the payment didn’t go through.
		Subscription ID: %s
		Amount: %s
		Payment method: %s
		
		Your premium stays active while we retry automatically. Update your payment details here to avoid losing access: https://doujins.com/account/billing
		
		If payment continues to fail, your membership will expire.
		
		The Doujins Team
	`, data.Username, data.SubscriptionID, amountLine, data.PaymentMethod)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

// SendEntitlementExpiration sends notification when an entitlement is expiring soon
func (s *EmailService) SendEntitlementExpiration(ctx context.Context, userEmail, username, entitlementName string, expiresAt time.Time) error {
	subject := fmt.Sprintf("Your %s access expires soon", entitlementName)

	daysUntilExpiry := int(time.Until(expiresAt).Hours() / 24)

	htmlContent := fmt.Sprintf(`
		<h2>Access Expiring Soon</h2>
		<p>Hi %s,</p>
		<p>This is a reminder that your <strong>%s</strong> access will expire in %d days on <strong>%s</strong>.</p>
		
		<p>To continue enjoying premium features, please renew your subscription before the expiration date.</p>
		<p>Thank you for being a valued member of Doujins!</p>
		<p>The Doujins Team</p>
    `, username, entitlementName, daysUntilExpiry, expiresAt.Format("January 2, 2006"))

	plainContent := fmt.Sprintf(`
		Access Expiring Soon
		
		Hi %s,
		
		Your %s access will expire in %d days on %s.
		
		To continue enjoying premium features, please renew your subscription before the expiration date.
		
		Thank you for being a valued member of Doujins!
		The Doujins Team
    `, username, entitlementName, daysUntilExpiry, expiresAt.Format("January 2, 2006"))

	return s.SendEmail(ctx, userEmail, subject, htmlContent, plainContent)
}
