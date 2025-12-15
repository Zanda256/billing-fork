package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/billing"
	"github.com/doujins-org/doujins-billing/internal/manager/api/cc-bill-alias"
	"github.com/doujins-org/doujins-billing/internal/manager/api/dead-letter"
	"github.com/doujins-org/doujins-billing/internal/manager/api/deduplication"
	"github.com/doujins-org/doujins-billing/internal/manager/api/lifecycle"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/api/types"
	"github.com/doujins-org/doujins-billing/pkg/ccbill"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"github.com/doujins-org/doujins-billing/pkg/nmi"
	"strings"
)

// WebhookDispatcher routes persisted webhook events to processor-specific handlers.
type WebhookDispatcher struct {
	DB                           *db.DB
	PriceService                 *price.PriceService
	ProductService               *product.ProductService
	NotificationQueueService     *notification.NotificationQueueService
	NotificationService          *notification.NotificationService
	SubscriptionService          *subscription.SubscriptionService
	PaymentService               *payment.PaymentService
	BillingEventService          *billing.BillingEventService
	SubscriptionLifecycleService *lifecycle.SubscriptionLifecycleService
	CCBillAliasService           *cc_bill_alias.CCBillAliasService
	DeduplicationService         *deduplication.DeduplicationService
	CCBillRESTClient             *ccbill.RESTClient
	NMIClients                   map[string]*nmi.NMIClient
}

// Process executes the processor-specific webhook flow.
func (d *WebhookDispatcher) Process(ctx context.Context, event *models.WebhookEvent) error {
	if event == nil {
		return fmt.Errorf("webhook event is required")
	}
	processor := strings.ToLower(strings.TrimSpace(event.Processor))
	switch processor {
	case "ccbill":
		return d.processCCBill(ctx, event)
	case "nmi":
		return d.processNMI(ctx, event)
	default:
		return fmt.Errorf("unsupported webhook processor: %s", processor)
	}
}

func (d *WebhookDispatcher) processCCBill(ctx context.Context, event *models.WebhookEvent) error {
	if d.CCBillRESTClient == nil {
		return fmt.Errorf("ccbill rest client not configured")
	}
	data := types.CCBillWebhookEvent{
		EventType: CCBillWebhookEventType(event.EventType),
		EventBody: json.RawMessage(event.RawPayload),
	}
	service := CCBillWebhookService{
		Data:                         data,
		DB:                           d.DB,
		CCBillClient:                 d.CCBillRESTClient,
		ProductService:               d.ProductService,
		PriceService:                 d.PriceService,
		NotificationQueueService:     d.NotificationQueueService,
		NotificationService:          d.NotificationService,
		DeadLetterService:            &dead_letter.DeadLetterService{DB: d.DB, NotificationQueueService: d.NotificationQueueService},
		BillingEventService:          d.BillingEventService,
		SubscriptionService:          d.SubscriptionService,
		SubscriptionLifecycleService: d.SubscriptionLifecycleService,
		CCBillAliasService:           d.CCBillAliasService,
	}
	return service.HandleCCBillWebhook(ctx)
}

func (d *WebhookDispatcher) processNMI(ctx context.Context, event *models.WebhookEvent) error {
	var payload types.NMIWebhookEvent
	if err := json.Unmarshal([]byte(event.RawPayload), &payload); err != nil {
		return fmt.Errorf("parse nmi webhook payload: %w", err)
	}
	provider := strings.ToLower(strings.TrimSpace(extractProvider(event.Headers)))
	if provider == "" {
		provider = "mobius"
	}
	client := d.NMIClients[provider]
	if client == nil {
		return fmt.Errorf("nmi client '%s' not configured", provider)
	}

	service := NMIWebhookService{
		DB:                           d.DB,
		PriceService:                 d.PriceService,
		ProductService:               d.ProductService,
		Data:                         payload,
		Provider:                     provider,
		DeadLetterService:            &dead_letter.DeadLetterService{DB: d.DB, NotificationQueueService: d.NotificationQueueService},
		NMIClient:                    client,
		BillingEventService:          d.BillingEventService,
		SubscriptionService:          d.SubscriptionService,
		PaymentService:               d.PaymentService,
		DeduplicationService:         d.DeduplicationService,
		NotificationQueueService:     d.NotificationQueueService,
		SubscriptionLifecycleService: d.SubscriptionLifecycleService,
	}
	return service.HandleNMIWebhook(ctx)
}

func extractProvider(headers map[string]string) string {
	if headers == nil {
		return ""
	}
	if provider, ok := headers["x-internal-provider"]; ok {
		return provider
	}
	if provider, ok := headers["provider"]; ok {
		return provider
	}
	return ""
}
