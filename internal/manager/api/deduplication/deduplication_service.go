package deduplication

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/idempotency"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// DeduplicationService provides robust webhook deduplication using PostgreSQL
type DeduplicationService struct{ idem *idempotency.IdempotencyService
}

// NewDeduplicationService creates a new webhook deduplication service
func NewDeduplicationService(db *db.DB) *DeduplicationService {
	return &DeduplicationService{idem: idempotency.NewIdempotencyService(db)}
}

// ProcessWebhook handles webhook deduplication and processing coordination.
// Returns (shouldProcess, webhookRecord, error)
func (s *DeduplicationService) ProcessWebhook(ctx context.Context, eventID, eventType string, processor models2.Processor, payload interface{}, processingFunc func(ctx context.Context) error) error {
	var payloadBytes []byte
	if payload != nil {
		if data, err := json.Marshal(payload); err == nil {
			payloadBytes = data
		} else {
			log.WithContext(ctx).WithError(err).Warn("failed to marshal webhook payload for idempotency storage")
		}
	}

	trimmedEventID := strings.TrimSpace(eventID)
	var idemReq *models2.IdempotencyRequest
	if trimmedEventID != "" {
		op := fmt.Sprintf("webhook.%s.%s", processor, eventType)
		req, exists, err := s.idem.Begin(ctx, op, trimmedEventID, nil)
		if err != nil {
			return fmt.Errorf("failed to begin idempotency: %w", err)
		}
		if exists && strings.EqualFold(req.Status, "success") {
			log.WithContext(ctx).WithFields(log.Fields{
				"eventID":   trimmedEventID,
				"eventType": eventType,
				"processor": processor,
			}).Info("Webhook already processed successfully, skipping")
			return nil
		}
		idemReq = req
	}

	processingErr := processingFunc(ctx)
	if processingErr != nil {
		log.WithContext(ctx).WithFields(log.Fields{
			"eventID":   trimmedEventID,
			"eventType": eventType,
			"processor": processor,
			"error":     processingErr.Error(),
		}).Error("Webhook processing failed")

		if idemReq != nil {
			if err := s.idem.Fail(ctx, idemReq.ID, processingErr); err != nil {
				log.WithContext(ctx).WithError(err).Warn("failed to mark webhook idempotency as failed")
			}
		}

		return fmt.Errorf("webhook processing failed: %w", processingErr)
	}

	if idemReq != nil {
		if err := s.idem.Complete(ctx, idemReq.ID, payloadBytes); err != nil {
			log.WithContext(ctx).WithError(err).Warn("failed to mark webhook idempotency as complete")
		}
	}

	log.WithContext(ctx).WithFields(log.Fields{
		"eventID":   trimmedEventID,
		"eventType": eventType,
		"processor": processor,
	}).Info("Webhook processed successfully")

	return nil
}

// GetFailedWebhooks retrieves webhooks that failed and can be retried
func (s *DeduplicationService) GetFailedWebhooks(ctx context.Context, processor models2.Processor, limit int) ([]any, error) {
	// Using generalized idempotency; failed webhook tracking not persisted
	return []any{}, nil
}

// CleanupOldWebhooks removes old completed webhook records to prevent table growth
func (s *DeduplicationService) CleanupOldWebhooks(ctx context.Context, retentionDays int) (int64, error) {
	// Keep completed webhooks for the specified retention period (default 30 days)
	// Failed webhooks are kept indefinitely for debugging
	retentionPeriod := fmt.Sprintf("%dd", retentionDays)
	_, err := parseRetentionPeriod(retentionPeriod)
	if err != nil {
		return 0, fmt.Errorf("invalid retention period: %w", err)
	}

	// No-op with generalized idempotency store
	log.WithContext(ctx).WithFields(log.Fields{
		"retentionPeriod": retentionPeriod,
	}).Info("Webhook cleanup is a no-op with idempotency store")
	return 0, nil
}

// parseRetentionPeriod converts a string like "30d" to a time.Duration
func parseRetentionPeriod(period string) (time.Duration, error) {
	// Simple implementation for days
	if len(period) < 2 {
		return 0, fmt.Errorf("invalid period format")
	}

	unit := period[len(period)-1:]
	valueStr := period[:len(period)-1]

	if unit != "d" {
		return 0, fmt.Errorf("only days (d) unit supported")
	}

	days, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, fmt.Errorf("invalid number of days: %w", err)
	}

	return time.Duration(days) * 24 * time.Hour, nil
}
