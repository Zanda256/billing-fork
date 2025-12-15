package dead_letter

import (
	"context"
	"encoding/json"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// DeadLetterService handles logging of failed or unexpected webhook events
type DeadLetterService struct {
	DB                       *db.DB
	NotificationQueueService *notification.NotificationQueueService
}

// WebhookDeadLetter represents a failed or unexpected webhook event
type WebhookDeadLetter struct {
	ID               uuid.UUID         `json:"id"`
	Processor        string            `json:"processor"`        // "ccbill" or "nmi"
	EventType        string            `json:"event_type"`       // The event type that failed
	RawPayload       json.RawMessage   `json:"raw_payload"`      // Original webhook payload
	FailureReason    string            `json:"failure_reason"`   // Why it failed
	ProcessingError  string            `json:"processing_error"` // Error message
	Headers          map[string]string `json:"headers"`          // Request headers
	ClientIP         string            `json:"client_ip"`        // Client IP address
	CreatedAt        time.Time         `json:"created_at"`
	RetryAttempts    int               `json:"retry_attempts"`
	LastRetryAt      *time.Time        `json:"last_retry_at"`
	ProcessedAt      *time.Time        `json:"processed_at"`
	ProcessingStatus string            `json:"processing_status"` // "failed", "unknown_event", "retry", "processed"
}

// LogDeadLetter logs a failed or unexpected webhook event
func (s *DeadLetterService) LogDeadLetter(ctx context.Context, params LogDeadLetterParams) error {
	deadLetter := WebhookDeadLetter{
		ID:               uuid.New(),
		Processor:        params.Processor,
		EventType:        params.EventType,
		RawPayload:       params.RawPayload,
		FailureReason:    params.FailureReason,
		ProcessingError:  params.ProcessingError,
		Headers:          params.Headers,
		ClientIP:         params.ClientIP,
		CreatedAt:        time.Now(),
		RetryAttempts:    0,
		ProcessingStatus: params.ProcessingStatus,
	}

	// Note: Dead letter events are logged for admin monitoring via structured logs
	// We don't create notification queue entries since those are for user notifications
	// and dead letters are system-level admin alerts that should be monitored via logs/alerting
	if err := s.persist(ctx, params); err != nil {
		log.WithContext(ctx).WithError(err).Warn("failed to persist webhook dead letter")
	}

	// Log structured event for monitoring and alerting
	logFields := log.Fields{
		"processor":         params.Processor,
		"event_type":        params.EventType,
		"failure_reason":    params.FailureReason,
		"processing_error":  params.ProcessingError,
		"client_ip":         params.ClientIP,
		"processing_status": params.ProcessingStatus,
		"dead_letter_id":    deadLetter.ID,
	}

	switch params.ProcessingStatus {
	case "unknown_event":
		log.WithContext(ctx).WithFields(logFields).Warn("Unknown webhook event type - stored in dead letter queue")
	case "failed":
		log.WithContext(ctx).WithFields(logFields).Error("Webhook processing failed - stored in dead letter queue")
	case "invalid_payload":
		log.WithContext(ctx).WithFields(logFields).Error("Invalid webhook payload - stored in dead letter queue")
	case "authentication_failed":
		log.WithContext(ctx).WithFields(logFields).Error("Webhook authentication failed - stored in dead letter queue")
	default:
		log.WithContext(ctx).WithFields(logFields).Error("Webhook dead letter logged")
	}

	return nil
}

func (s *DeadLetterService) persist(ctx context.Context, params LogDeadLetterParams) error {
	if s.DB == nil {
		return nil
	}
	payload := params.RawPayload
	if len(payload) == 0 {
		payload = json.RawMessage("null")
	}
	wh := &models.WebhookEvent{
		Processor:        params.Processor,
		EventType:        params.EventType,
		Status:           params.ProcessingStatus,
		RawPayload:       string(payload),
		Headers:          params.Headers,
		IPAddress:        params.ClientIP,
		ProcessingResult: map[string]interface{}{"failure_reason": params.FailureReason},
	}
	if params.ProcessingError != "" {
		msg := params.ProcessingError
		wh.ErrorMessage = &msg
	}
	_, err := s.DB.GetDB().NewInsert().Model(wh).Exec(ctx)
	return err
}

// LogUnknownEvent specifically logs webhook events with unknown/unsupported event types
func (s *DeadLetterService) LogUnknownEvent(ctx context.Context, processor, eventType string, rawPayload json.RawMessage, headers map[string]string, clientIP string) error {
	return s.LogDeadLetter(ctx, LogDeadLetterParams{
		Processor:        processor,
		EventType:        eventType,
		RawPayload:       rawPayload,
		FailureReason:    "Unknown or unsupported event type",
		ProcessingError:  "Event type not implemented in webhook handler",
		Headers:          headers,
		ClientIP:         clientIP,
		ProcessingStatus: "unknown_event",
	})
}

// LogProcessingFailure logs webhook events that failed during processing
func (s *DeadLetterService) LogProcessingFailure(ctx context.Context, processor, eventType string, rawPayload json.RawMessage, processingError error, headers map[string]string, clientIP string) error {
	return s.LogDeadLetter(ctx, LogDeadLetterParams{
		Processor:        processor,
		EventType:        eventType,
		RawPayload:       rawPayload,
		FailureReason:    "Processing error occurred",
		ProcessingError:  processingError.Error(),
		Headers:          headers,
		ClientIP:         clientIP,
		ProcessingStatus: "failed",
	})
}

// LogInvalidPayload logs webhook events with invalid or malformed payloads
func (s *DeadLetterService) LogInvalidPayload(ctx context.Context, processor string, rawPayload json.RawMessage, validationError error, headers map[string]string, clientIP string) error {
	return s.LogDeadLetter(ctx, LogDeadLetterParams{
		Processor:        processor,
		EventType:        "unknown",
		RawPayload:       rawPayload,
		FailureReason:    "Invalid or malformed payload",
		ProcessingError:  validationError.Error(),
		Headers:          headers,
		ClientIP:         clientIP,
		ProcessingStatus: "invalid_payload",
	})
}

// LogAuthenticationFailure logs webhook events that failed authentication
func (s *DeadLetterService) LogAuthenticationFailure(ctx context.Context, processor string, rawPayload json.RawMessage, authError error, headers map[string]string, clientIP string) error {
	return s.LogDeadLetter(ctx, LogDeadLetterParams{
		Processor:        processor,
		EventType:        "unknown",
		RawPayload:       rawPayload,
		FailureReason:    "Authentication failed",
		ProcessingError:  authError.Error(),
		Headers:          headers,
		ClientIP:         clientIP,
		ProcessingStatus: "authentication_failed",
	})
}

// LogDeadLetterParams holds parameters for logging dead letters
type LogDeadLetterParams struct {
	Processor        string            `json:"processor"`
	EventType        string            `json:"event_type"`
	RawPayload       json.RawMessage   `json:"raw_payload"`
	FailureReason    string            `json:"failure_reason"`
	ProcessingError  string            `json:"processing_error"`
	Headers          map[string]string `json:"headers"`
	ClientIP         string            `json:"client_ip"`
	ProcessingStatus string            `json:"processing_status"`
}
