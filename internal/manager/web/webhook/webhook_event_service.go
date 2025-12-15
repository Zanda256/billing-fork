package webhook

import (
	"context"
	"fmt"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	WebhookStatusPending    = "pending"
	WebhookStatusProcessing = "processing"
	WebhookStatusProcessed  = "processed"
	WebhookStatusFailed     = "failed"
	WebhookStatusError      = "error"
)

// WebhookEventService persists webhook events and manages retry metadata.
type WebhookEventService struct {
	db    *db.DB
	retry config.WebhookRetryConfig
}

// NewWebhookEventService constructs a new persistence service.
func NewWebhookEventService(database *db.DB, retryCfg config.WebhookRetryConfig) *WebhookEventService {
	return &WebhookEventService{db: database, retry: retryCfg}
}

// CreateWebhookEventParams holds the metadata for storing a webhook.
type CreateWebhookEventParams struct {
	Processor      string
	EventID        string
	EventType      string
	Payload        []byte
	Headers        map[string]string
	IPAddress      string
	Signature      string
	SignatureValid *bool
}

// Create stores the webhook event and returns the persisted record.
func (s *WebhookEventService) Create(ctx context.Context, params CreateWebhookEventParams) (*models.WebhookEvent, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("webhook event service not initialised")
	}

	processor := strings.TrimSpace(strings.ToLower(params.Processor))
	if processor == "" {
		return nil, fmt.Errorf("processor is required")
	}
	if params.EventType == "" {
		return nil, fmt.Errorf("event type is required")
	}
	if params.IPAddress == "" {
		return nil, fmt.Errorf("ip address is required")
	}

	nextAttempt := time.Now()
	event := &models.WebhookEvent{
		Processor:          processor,
		EventID:            nilIfEmpty(params.EventID),
		EventType:          params.EventType,
		Status:             WebhookStatusPending,
		RawPayload:         string(params.Payload),
		Headers:            params.Headers,
		IPAddress:          params.IPAddress,
		Signature:          nilIfEmpty(params.Signature),
		SignatureValid:     params.SignatureValid,
		ProcessingAttempts: 0,
		NextAttemptAt:      &nextAttempt,
	}

	if _, err := s.db.GetDB().NewInsert().Model(event).Returning("*").Exec(ctx); err != nil {
		return nil, fmt.Errorf("insert webhook event: %w", err)
	}
	return event, nil
}

// Get fetches a webhook event by ID.
func (s *WebhookEventService) Get(ctx context.Context, id uuid.UUID) (*models.WebhookEvent, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("webhook event service not initialised")
	}
	event := new(models.WebhookEvent)
	if err := s.db.GetDB().NewSelect().Model(event).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return event, nil
}

// BeginProcessing increments attempt counters and marks the event as processing.
func (s *WebhookEventService) BeginProcessing(ctx context.Context, id uuid.UUID) (*models.WebhookEvent, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("webhook event service not initialised")
	}
	now := time.Now()
	event := new(models.WebhookEvent)
	err := s.db.GetDB().NewUpdate().Model(event).
		Set("status = ?", WebhookStatusProcessing).
		Set("processing_attempts = processing_attempts + 1").
		Set("last_attempt_at = ?", now).
		Set("updated_at = ?", now).
		Set("next_attempt_at = NULL").
		Where("id = ?", id).
		Where("status IN (?)", bun.In([]string{WebhookStatusPending, WebhookStatusFailed})).
		Returning("*").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return event, nil
}

// MarkProcessed marks the webhook as processed successfully.
func (s *WebhookEventService) MarkProcessed(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("webhook event service not initialised")
	}
	now := time.Now()
	_, err := s.db.GetDB().NewUpdate().Model((*models.WebhookEvent)(nil)).
		Set("status = ?", WebhookStatusProcessed).
		Set("processed_at = ?", now).
		Set("next_attempt_at = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark webhook processed: %w", err)
	}
	return nil
}

// MarkFailure updates the webhook state after a processing failure.
// Returns true if the event should be retried.
func (s *WebhookEventService) MarkFailure(ctx context.Context, event *models.WebhookEvent, failure error) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("webhook event service not initialised")
	}
	if event == nil {
		return false, fmt.Errorf("event is required")
	}
	attempts := event.ProcessingAttempts
	shouldRetry := attempts < s.retry.MaxAttempts
	status := WebhookStatusFailed
	var nextAttempt *time.Time
	now := time.Now()
	if shouldRetry {
		backoff := s.nextBackoff(attempts)
		scheduled := now.Add(backoff)
		nextAttempt = &scheduled
	} else {
		status = WebhookStatusError
	}

	_, err := s.db.GetDB().NewUpdate().Model((*models.WebhookEvent)(nil)).
		Set("status = ?", status).
		Set("error_message = ?", truncateError(failure)).
		Set("next_attempt_at = ?", nextAttempt).
		Set("processing_result = ?", map[string]any{"error": truncateError(failure)}).
		Set("updated_at = ?", now).
		Where("id = ?", event.ID).
		Exec(ctx)
	if err != nil {
		return shouldRetry, fmt.Errorf("mark webhook failure: %w", err)
	}
	return shouldRetry, nil
}

// ListRetryable returns events that are ready to be retried.
func (s *WebhookEventService) ListRetryable(ctx context.Context, limit int) ([]models.WebhookEvent, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("webhook event service not initialised")
	}
	if limit <= 0 {
		limit = s.retry.BatchSize
	}
	events := []models.WebhookEvent{}
	query := s.db.GetDB().NewSelect().Model(&events).
		Where("status IN (?)", bun.In([]string{WebhookStatusPending, WebhookStatusFailed})).
		Where("next_attempt_at IS NULL OR next_attempt_at <= now()").
		OrderExpr("received_at ASC").
		Limit(limit)
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *WebhookEventService) nextBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return s.retry.InitialBackoff
	}
	multiplier := 1 << (attempt - 1)
	backoff := time.Duration(multiplier) * s.retry.InitialBackoff
	if backoff > s.retry.MaxBackoff {
		backoff = s.retry.MaxBackoff
	}
	return backoff
}

func nilIfEmpty(val string) *string {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 2048 {
		return msg[:2048]
	}
	return msg
}
