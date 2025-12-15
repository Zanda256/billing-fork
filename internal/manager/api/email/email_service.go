package email

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
)

type EmailService struct {
	client *sendgrid.Client
	from   *mail.Email
}

// OneOffPurchaseEmailData contains data for one-off purchase receipts
type OneOffPurchaseEmailData struct {
	UserEmail     string
	Amount        float64
	Currency      string
	ProductName   string
	PaymentMethod string
	IsPremium     bool
}

// NewEmailService wires the SendGrid SDK into the billing domain service.
func NewEmailService(cfg *config.SendGridConfig) (*EmailService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("email configuration not provided")
	}

	apiKey := strings.TrimSpace(cfg.APIKey)
	fromEmail := strings.TrimSpace(cfg.FromEmail)
	fromName := strings.TrimSpace(cfg.FromName)
	if apiKey == "" || fromEmail == "" {
		return nil, fmt.Errorf("sendgrid configuration incomplete")
	}

	client := sendgrid.NewSendClient(apiKey)
	from := mail.NewEmail(fromName, fromEmail)

	return &EmailService{client: client, from: from}, nil
}

// IsEnabled returns true when delivery is possible.
func (s *EmailService) IsEnabled() bool {
	return s != nil && s.client != nil && s.from != nil
}

// SendEmail sends a basic email using the configured provider.
func (s *EmailService) SendEmail(ctx context.Context, to, subject, htmlContent, plainContent string) error {
	if !s.IsEnabled() {
		log.WithContext(ctx).WithFields(log.Fields{
			"to":      to,
			"subject": subject,
		}).Debug("email service disabled - skipping send")
		return nil
	}

	toMail := mail.NewEmail("", to)
	msg := mail.NewSingleEmail(s.from, subject, toMail, plainContent, htmlContent)

	return s.send(ctx, msg, to)
}

// SendTemplatedEmail sends a template-based email using the configured provider.
func (s *EmailService) SendTemplatedEmail(ctx context.Context, to, templateID string, templateData map[string]any) error {
	if !s.IsEnabled() {
		log.WithContext(ctx).WithFields(log.Fields{
			"to":          to,
			"template_id": templateID,
		}).Debug("email service disabled - skipping templated send")
		return nil
	}

	toMail := mail.NewEmail("", to)
	msg := mail.NewV3Mail()
	msg.SetFrom(s.from)
	msg.SetTemplateID(templateID)
	personalization := mail.NewPersonalization()
	personalization.AddTos(toMail)
	for key, value := range templateData {
		personalization.SetDynamicTemplateData(key, value)
	}
	msg.AddPersonalizations(personalization)

	return s.send(ctx, msg, to)
}

// SendOneOffPurchaseReceipt sends a receipt for a one-off purchase (e.g., Solana payment).
func (s *EmailService) SendOneOffPurchaseReceipt(ctx context.Context, data OneOffPurchaseEmailData) error {
	if !s.IsEnabled() {
		log.WithContext(ctx).WithField("user_email", data.UserEmail).Debug("email service disabled - skipping one-off receipt send")
		return nil
	}

	productName := data.ProductName
	if productName == "" {
		productName = "Doujins premium content"
	}

	amountLine := fmt.Sprintf("%.2f %s", data.Amount, data.Currency)
	if data.Currency == "USD" {
		amountLine = fmt.Sprintf("$%.2f %s", data.Amount, data.Currency)
	}

	issuedAt := time.Now().Format("Jan 2, 2006 15:04 MST")

	paymentMethod := strings.ToLower(data.PaymentMethod)
	isSolana := paymentMethod == "solana"

	subject := "Thanks for supporting Doujins!"
	if isSolana {
		subject = "Your Solana premium purchase is confirmed"
	}

	messageIntro := "Thanks for completing your purchase!"
	if data.IsPremium {
		messageIntro = "Thanks for unlocking Doujins Premium!"
	}

	if isSolana {
		htmlContent := fmt.Sprintf(`
			<h2>Solana Payment Received</h2>
			<p>Hi there,</p>
			<p>%s This one-time Solana transaction instantly extended your premium access.</p>
			<ul>
				<li><strong>Product:</strong> %s</li>
				<li><strong>Amount:</strong> %s</li>
				<li><strong>Date:</strong> %s</li>
			</ul>
			<p>Enjoy your premium benefits—no rebill will occur automatically.</p>
			<p>The Doujins Team</p>
		`, messageIntro, productName, amountLine, issuedAt)

		plainContent := fmt.Sprintf(`
		Solana Payment Received
		
		%s This one-time Solana transaction instantly extended your premium access.
		Product: %s
		Amount: %s
		Date: %s
		
		Enjoy your premium benefits—there won't be an automatic rebill.
		The Doujins Team
		`, messageIntro, productName, amountLine, issuedAt)

		return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
	}

	htmlContent := fmt.Sprintf(`
		<h2>Payment Received</h2>
		<p>Hi there,</p>
		<p>%s</p>
		<ul>
			<li><strong>Product:</strong> %s</li>
			<li><strong>Amount:</strong> %s</li>
			<li><strong>Date:</strong> %s</li>
		</ul>
		<p>Your access has been updated instantly. Enjoy!</p>
		<p>The Doujins Team</p>
	`, messageIntro, productName, amountLine, issuedAt)

	plainContent := fmt.Sprintf(`
		Payment Received
		
		%s
		Product: %s
		Amount: %s
		Date: %s
		
		Your access has been updated instantly. Enjoy!
		The Doujins Team
	`, messageIntro, productName, amountLine, issuedAt)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

func (s *EmailService) send(ctx context.Context, msg *mail.SGMailV3, to string) error {
	res, err := s.client.SendWithContext(ctx, msg)
	if err != nil {
		return fmt.Errorf("sendgrid email send failed: %w", err)
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("sendgrid api error: status %d, body: %s", res.StatusCode, res.Body)
	}

	log.WithContext(ctx).WithFields(log.Fields{
		"to":     to,
		"status": res.StatusCode,
	}).Debug("email sent successfully via sendgrid")
	return nil
}
