package webhook

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// WebhookProcessor orchestrates loading, dispatching, and updating webhook events.
type WebhookProcessor struct {
	Events     *WebhookEventService
	Dispatcher *WebhookDispatcher
}

// Process executes the webhook event identified by id.
// It returns true when a retry has been scheduled.
func (p *WebhookProcessor) Process(ctx context.Context, id uuid.UUID) (bool, error) {
	if p == nil || p.Events == nil || p.Dispatcher == nil {
		return false, fmt.Errorf("webhook processor not initialised")
	}

	event, err := p.Events.BeginProcessing(ctx, id)
	if err != nil {
		return false, fmt.Errorf("begin webhook processing: %w", err)
	}

	if err := p.Dispatcher.Process(ctx, event); err != nil {
		retry, markErr := p.Events.MarkFailure(ctx, event, err)
		if markErr != nil {
			return retry, fmt.Errorf("mark webhook failure: %w", markErr)
		}
		return retry, err
	}

	if err := p.Events.MarkProcessed(ctx, id); err != nil {
		return false, fmt.Errorf("mark webhook success: %w", err)
	}
	return false, nil
}
