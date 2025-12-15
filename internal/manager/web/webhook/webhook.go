package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/dead-letter"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/api/types"
	riverjobs2 "github.com/doujins-org/doujins-billing/internal/manager/data/river"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/app"
	ipverify "github.com/doujins-org/doujins-billing/internal/utils"
)

func Webhook(r *request.Request) {
	processor := r.Param("processor")
	provider := strings.Trim(strings.Trim(r.Param("provider"), "/"), " ")
	if provider == "" {
		provider = strings.TrimSpace(r.Query("provider"))
	}

	// NOTE: Webhook authentication can be bypassed for testing by setting test_mode: true
	// in the respective processor config (nmi.test_mode or ccbill.test_mode)
	// This is useful for integration tests and development environments

	// Create dead letter service
	deadLetterService := &dead_letter.DeadLetterService{
		DB:                       r.State.DB,
		NotificationQueueService: r.State.NotificationQueueService,
	}

	// Capture request headers for dead letter logging
	headers := make(map[string]string)
	for key, values := range r.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	clientIP := r.GetClientIP()

	log.WithFields(log.Fields{
		"processor": processor,
		"client_ip": clientIP,
	}).Debug("Received webhook")

	ccbillTestMode := false
	if r.State != nil && r.State.CCBillRESTClient != nil {
		if cfg := r.State.CCBillRESTClient.Config(); cfg != nil {
			ccbillTestMode = cfg.TestMode
		}
	}

	switch processor {
	case subscription.ProcessorCCBill:
		// Check if CCBill is in test mode - bypass authentication for testing
		if !ccbillTestMode {
			// Verify CCBill webhook comes from authorized IP ranges
			if !ipverify.IsValidCCBillIP(clientIP) {
				log.WithFields(log.Fields{
					"client_ip":  clientIP,
					"processor":  "ccbill",
					"event_type": r.Query("eventType"),
				}).Warn("CCBill webhook rejected - unauthorized IP address")

				// Log the security violation for monitoring
				deadLetterService.LogInvalidPayload(
					context.Background(),
					"ccbill",
					[]byte(fmt.Sprintf("Unauthorized IP: %s", clientIP)),
					fmt.Errorf("webhook from unauthorized IP address: %s", clientIP),
					headers,
					clientIP,
				)

				r.ErrorJSON(http.StatusForbidden, "Unauthorized webhook source")
				return
			}

			log.WithField("client_ip", clientIP).Debug("CCBill webhook authenticated - valid IP range")
		} else {
			log.WithField("client_ip", clientIP).Debug("CCBill webhook authentication bypassed - test mode enabled")
		}

		body, err := readRequestBody(r.Request.Body)
		if err != nil {
			deadLetterService.LogInvalidPayload(context.Background(), "ccbill", nil, err, headers, clientIP)
			r.ErrorJSON(http.StatusInternalServerError, "Failed to read request body")
			return
		}

		body, err = normalizeCCBillPayload(body)
		if err != nil {
			deadLetterService.LogInvalidPayload(context.Background(), "ccbill", body, err, headers, clientIP)
			r.ErrorJSON(http.StatusBadRequest, "Invalid webhook payload")
			return
		}

		eventType := strings.TrimSpace(r.Query("eventType"))
		if eventType == "" {
			r.ErrorJSON(http.StatusBadRequest, "Missing eventType parameter")
			return
		}

		if r.State == nil || r.State.WebhookEventService == nil || r.State.WebhookProcessor == nil {
			log.Error("Webhook components not configured; unable to persist event")
			r.ErrorJSON(http.StatusInternalServerError, "Webhook processing unavailable")
			return
		}

		eventHeaders := copyHeaders(headers)
		if provider != "" {
			eventHeaders["x-internal-provider"] = provider
		}

		ctx := r.Request.Context()
		event, err := r.State.WebhookEventService.Create(ctx, CreateWebhookEventParams{
			Processor: subscription.ProcessorCCBill,
			EventType: eventType,
			Payload:   body,
			Headers:   eventHeaders,
			IPAddress: clientIP,
		})
		if err != nil {
			log.WithError(err).Error("failed to persist CCBill webhook event")
			r.ErrorJSON(http.StatusInternalServerError, "Failed to persist webhook event")
			return
		}

		if err := queueOrProcessWebhook(ctx, r.State, event.ID); err != nil {
			log.WithError(err).Error("failed to enqueue CCBill webhook for processing")
		}

		r.SuccessJSON(map[string]string{"status": "accepted"})
		return
	case subscription.ProcessorNMI:
		handleNMIWebhook(r, provider, headers, clientIP)
		return
	default:
		webhookBody, readErr := readRequestBody(r.Request.Body)
		if readErr != nil {
			log.WithError(readErr).WithField("processor", processor).Warn("Failed to read body for unknown webhook processor")
		}
		// Log unknown processor to dead letter queue
		deadLetterService.LogUnknownEvent(context.Background(), processor, "unknown", json.RawMessage(webhookBody), headers, clientIP)
		r.ErrorJSON(http.StatusBadRequest, "Invalid processor")
		return
	}

}

func handleNMIWebhook(r *request.Request, provider string, headers map[string]string, clientIP string) {
	// Read the request body for signature verification
	body, err := readRequestBody(r.Request.Body)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, "Failed to read request body")
		return
	}

	providerKey := strings.TrimSpace(strings.ToLower(provider))
	if providerKey == "" {
		providerKey = "mobius"
	}

	client, ok := r.State.NMIClients[providerKey]
	if !ok || client == nil {
		r.ErrorJSON(http.StatusNotFound, fmt.Sprintf("unknown nmi provider '%s'", providerKey))
		return
	}

	signature := ""
	signatureValidated := false
	if !client.Config().TestMode {
		if client.GetWebhookSecret() != "" {
			signature = r.Request.Header.Get("X-Signature")
			if signature == "" {
				signature = r.Request.Header.Get("X-NMI-Signature")
			}
			if signature == "" {
				signature = r.Request.Header.Get("X-Mobius-Signature")
			}
			if signature == "" {
				log.Error("Missing webhook signature for NMI webhook")
				r.ErrorJSON(http.StatusUnauthorized, "Missing webhook signature")
				return
			}
			if err := client.VerifyWebhookSignature(body, signature); err != nil {
				log.WithError(err).Error("NMI webhook signature verification failed")
				r.ErrorJSON(http.StatusUnauthorized, "Invalid webhook signature")
				return
			}
			signatureValidated = true
		} else {
			log.Warn("NMI webhook secret not configured - skipping signature verification")
		}
	} else {
		log.Debug("NMI webhook authentication bypassed - test mode enabled")
	}

	var data types.NMIWebhookEvent
	if err := json.Unmarshal(body, &data); err != nil {
		log.WithError(err).Error("failed to parse NMI webhook JSON")
		r.ErrorJSON(http.StatusBadRequest, "Invalid JSON data")
		return
	}
	if data.EventID == "" {
		r.ErrorJSON(http.StatusBadRequest, "Missing event_id in payload")
		return
	}

	if r.State == nil || r.State.WebhookEventService == nil || r.State.WebhookProcessor == nil {
		log.Error("Webhook components not configured; unable to persist NMI event")
		r.ErrorJSON(http.StatusInternalServerError, "Webhook processing unavailable")
		return
	}

	ctx := r.Request.Context()
	eventHeaders := copyHeaders(headers)
	eventHeaders["x-internal-provider"] = providerKey

	var signatureValidPtr *bool
	if signatureValidated {
		truth := true
		signatureValidPtr = &truth
	}

	params := CreateWebhookEventParams{
		Processor:      subscription.ProcessorNMI,
		EventID:        data.EventID,
		EventType:      string(data.EventType),
		Payload:        body,
		Headers:        eventHeaders,
		IPAddress:      clientIP,
		Signature:      signature,
		SignatureValid: signatureValidPtr,
	}
	if event, err := r.State.WebhookEventService.Create(ctx, params); err != nil {
		log.WithError(err).Error("failed to persist NMI webhook event")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to persist webhook event")
		return
	} else {
		if err := queueOrProcessWebhook(ctx, r.State, event.ID); err != nil {
			log.WithError(err).Error("failed to enqueue NMI webhook for processing")
		}
	}

	r.SuccessJSON(map[string]string{"status": "accepted"})
}

func readRequestBody(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return []byte{}, nil
	}
	defer body.Close()
	return io.ReadAll(body)
}

func normalizeCCBillPayload(body []byte) ([]byte, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return body, nil
	}
	if body[0] == '{' || body[0] == '[' {
		return body, nil
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	payload := make(map[string]string, len(values))
	for key, val := range values {
		if len(val) > 0 {
			payload[key] = val[0]
		}
	}
	return json.Marshal(payload)
}

func copyHeaders(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func queueOrProcessWebhook(ctx context.Context, runtime *app.Runtime, eventID uuid.UUID) error {
	if runtime == nil || runtime.WebhookProcessor == nil {
		return fmt.Errorf("webhook processor unavailable")
	}
	if runtime.RiverClient != nil {
		if _, err := runtime.RiverClient.Insert(ctx, riverjobs2.WebhookProcessArgs{EventID: eventID}, &river.InsertOpts{Queue: riverjobs2.QueueBilling}); err == nil {
			return nil
		} else {
			log.WithError(err).Warn("Failed to enqueue webhook job; processing inline")
		}
	}
	_, err := runtime.WebhookProcessor.Process(ctx, eventID)
	return err
}
