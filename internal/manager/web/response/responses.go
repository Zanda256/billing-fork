package response

import (
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/api/user"
	"time"
)

type GetSubscriptionResponse = user.UserSubscriptionResponse

type SubscribeResponse = subscription.SubscribeResponse

type GetProductsResponse = []*subscription.PublicProductResponse

func NewGetProductsResponse(products []*subscription.PublicProductResponse) GetProductsResponse {
	return GetProductsResponse(products)
}

// -------------------------------- Solana / Payments Responses --------------------------------

// GeneratePaymentResponse represents a generated on-chain transaction with helper info
type GeneratePaymentResponse struct {
	Transaction  string  `json:"transaction"`            // base64-encoded tx (placeholder if not available)
	Amount       float64 `json:"amount"`                 // fiat price amount
	Currency     string  `json:"currency"`               // fiat currency (e.g., USD)
	TokenAmount  uint64  `json:"token_amount"`           // smallest unit amount
	TokenSymbol  string  `json:"token_symbol"`           // SOL/USDC/etc.
	ExpiresAt    int64   `json:"expires_at"`             // unix epoch expiry
	Instructions string  `json:"instructions,omitempty"` // human-readable instructions
	IntentID     string  `json:"intent_id"`
}

// SubmitPaymentResponse represents the result of submitting a signed transaction
type SubmitPaymentResponse struct {
	PurchaseID    string    `json:"purchase_id"`
	TransactionID string    `json:"transaction_id"`
	Status        string    `json:"status"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	ProcessedAt   time.Time `json:"processed_at"`
	Message       string    `json:"message"`
	IntentID      string    `json:"intent_id"`
}

// PaymentStatusResponse represents the status of a payment
type PaymentStatusResponse struct {
	PurchaseID    string     `json:"purchase_id"`
	TransactionID string     `json:"transaction_id"`
	Status        string     `json:"status"`
	Amount        float64    `json:"amount"`
	Currency      string     `json:"currency"`
	CreatedAt     time.Time  `json:"created_at"`
	ConfirmedAt   *time.Time `json:"confirmed_at,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// SolanaPayQRResponse contains the Solana Pay URL metadata
type SolanaPayQRResponse struct {
	URL         string  `json:"url"`          // Solana Pay URL for QR code
	Amount      float64 `json:"amount"`       // USD amount
	TokenAmount string  `json:"token_amount"` // human-readable token amount
	TokenSymbol string  `json:"token_symbol"` // SOL, USDC, etc.
	Label       string  `json:"label"`        // Merchant label
	Message     string  `json:"message"`      // Payment message
	ExpiresAt   int64   `json:"expires_at"`   // Unix timestamp when QR expires
	Reference   string  `json:"reference"`
	IntentID    string  `json:"intent_id"`
}

// CheckSolanaPaymentResponse contains the payment check result
type CheckSolanaPaymentResponse struct {
	Status       string `json:"status"`               // pending, confirmed, failed
	PaymentID    string `json:"payment_id,omitempty"` // Payment ID if confirmed
	IntentID     string `json:"intent_id"`
	Transaction  string `json:"transaction,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// SupportedTokensResponse lists available Solana tokens from config
type SupportedTokensResponse struct {
	Tokens []TokenInfo `json:"tokens"`
}

type TokenInfo struct {
	Symbol   string  `json:"symbol"`
	Name     string  `json:"name"`
	Mint     string  `json:"mint"`
	Decimals int     `json:"decimals"`
	Price    float64 `json:"price"`
}

type PublicPriceResponse struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	BillingCycleDays int     `json:"billing_cycle_days"`
}

// (Removed) NMI setup response and nonce: no longer needed because
// the Collect.js tokenization key is injected into the frontend template.

type GetSubscribePageDataResponse = map[string]any

func NewGetSubscribePageDataResponse(data map[string]any) GetSubscribePageDataResponse {
	return GetSubscribePageDataResponse(data)
}

type CancelSubscriptionResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type GenerateFlexFormURLResponse struct {
	IFrameURL  string `json:"iframe_url" binding:"required"`
	Width      string `json:"width" binding:"required"`
	Height     string `json:"height" binding:"required"`
	SuccessURL string `json:"success_url,omitempty"`
	DeclineURL string `json:"decline_url,omitempty"`
}

// -------------------------------- Enhanced Billing History Responses --------------------------------

// SubscriptionHistoryItem represents a subscription record with price details
type SubscriptionHistoryItem struct {
	ID                      string                 `json:"id"`
	Status                  string                 `json:"status"`
	Processor               string                 `json:"processor"`
	ProcessorSubscriptionID string                 `json:"processor_subscription_id"`
	StartedAt               time.Time              `json:"started_at"`
	EndedAt                 *time.Time             `json:"ended_at,omitempty"`
	CurrentPeriodStartsAt   *time.Time             `json:"current_period_starts_at,omitempty"`
	CurrentPeriodEndsAt     *time.Time             `json:"current_period_ends_at,omitempty"`
	CancelledAt             *time.Time             `json:"cancelled_at,omitempty"`
	CancelType              *string                `json:"cancel_type,omitempty"`
	CancelFeedback          *string                `json:"cancel_feedback,omitempty"`
	CreatedAt               time.Time              `json:"created_at"`
	UpdatedAt               time.Time              `json:"updated_at"`
	Price                   *PriceInfo             `json:"price,omitempty"`
	PaymentMethod           *PaymentMethodInfo     `json:"payment_method,omitempty"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
}

// PriceInfo represents price information for billing history
type PriceInfo struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	BillingCycleDays int     `json:"billing_cycle_days"`
}

// PaymentMethodInfo represents payment method information for billing history
type PaymentMethodInfo struct {
	ID        string `json:"id"`
	Processor string `json:"processor"`
	IsActive  bool   `json:"is_active"`
}

// PaymentItem represents a canonical payment record from Postgres
type PaymentItem struct {
	ID             string     `json:"id"`
	SubscriptionID *string    `json:"subscription_id,omitempty"`
	Processor      string     `json:"processor"`
	TransactionID  string     `json:"transaction_id"`
	Amount         float64    `json:"amount"`
	Currency       string     `json:"currency"`
	Price          *PriceInfo `json:"price,omitempty"`
	PurchasedAt    time.Time  `json:"purchased_at"`
}

// PaymentEventItem represents a payment transaction event
type PaymentEventItem struct {
	EventID                string                 `json:"event_id"`
	SubscriptionID         *string                `json:"subscription_id,omitempty"`
	EventType              string                 `json:"event_type"`
	Processor              string                 `json:"processor"`
	ProcessorTransactionID *string                `json:"processor_transaction_id,omitempty"`
	Amount                 *float64               `json:"amount,omitempty"`
	Currency               string                 `json:"currency"`
	BillingInfo            map[string]interface{} `json:"billing_info,omitempty"`
	WebhookSource          *string                `json:"webhook_source,omitempty"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	Timestamp              time.Time              `json:"timestamp"`
	CreatedAt              time.Time              `json:"created_at"`
}

// SubscriptionEventItem represents a subscription lifecycle event
type SubscriptionEventItem struct {
	EventID                 string                 `json:"event_id"`
	SubscriptionID          string                 `json:"subscription_id"`
	EventType               string                 `json:"event_type"`
	Processor               string                 `json:"processor"`
	ProcessorSubscriptionID *string                `json:"processor_subscription_id,omitempty"`
	ProcessorTransactionID  *string                `json:"processor_transaction_id,omitempty"`
	Amount                  *float64               `json:"amount,omitempty"`
	Currency                string                 `json:"currency"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
	Timestamp               time.Time              `json:"timestamp"`
	CreatedAt               time.Time              `json:"created_at"`
}

// BillingHistoryStats represents aggregated billing statistics
type BillingHistoryStats struct {
	TotalCharged       float64                       `json:"total_charged"`
	TotalCharges       int                           `json:"total_charges"`
	SuccessfulCharges  int                           `json:"successful_charges"`
	FailedCharges      int                           `json:"failed_charges"`
	Refunds            int                           `json:"refunds"`
	TotalRefunded      float64                       `json:"total_refunded"`
	FirstChargeDate    *time.Time                    `json:"first_charge_date,omitempty"`
	LastChargeDate     *time.Time                    `json:"last_charge_date,omitempty"`
	ProcessorBreakdown map[string]ProcessorStatsInfo `json:"processor_breakdown"`
}

// ProcessorStatsInfo represents statistics for a specific payment processor
type ProcessorStatsInfo struct {
	TotalCharged      float64 `json:"total_charged"`
	TotalCharges      int     `json:"total_charges"`
	SuccessfulCharges int     `json:"successful_charges"`
	FailedCharges     int     `json:"failed_charges"`
}

// GetBillingHistoryResponse represents the enhanced billing history response
type GetBillingHistoryResponse struct {
	Subscriptions      []SubscriptionHistoryItem `json:"subscriptions"`
	Payments           []PaymentItem             `json:"payments"`
	PaymentEvents      []PaymentEventItem        `json:"payment_events,omitempty"`
	SubscriptionEvents []SubscriptionEventItem   `json:"subscription_events,omitempty"`
	Stats              *BillingHistoryStats      `json:"stats,omitempty"`
	Page               int                       `json:"page"`
	PageSize           int                       `json:"page_size"`
	TotalItems         int64                     `json:"total_items"`
	TotalPages         int                       `json:"total_pages"`
	HasMore            bool                      `json:"has_more"`
}

// NewGetBillingHistoryResponse creates a new billing history response
// func NewGetBillingHistoryResponse(
// 	subscriptions []models.Subscription,
// 	payments []*models.Payment,
// 	paymentEvents []services.BillingEvent,
// 	subscriptionEvents []services.SubscriptionEvent,
// 	stats *services.BillingHistoryStats,
// 	page, pageSize int,
// 	totalItems int64,
// ) *GetBillingHistoryResponse {
// 	// Calculate pagination
// 	totalPages, hasMore := common.CalculatePagination(page, pageSize, totalItems)

// 	// Convert subscriptions
// 	subscriptionItems := make([]SubscriptionHistoryItem, len(subscriptions))
// 	for i, sub := range subscriptions {
// 		item := SubscriptionHistoryItem{
// 			ID:                      sub.ID.String(),
// 			Status:                  string(sub.Status),
// 			Processor:               string(sub.Processor),
// 			ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
// 			StartedAt:               sub.StartedAt,
// 			EndedAt:                 sub.EndedAt,
// 			CurrentPeriodStartsAt:   sub.CurrentPeriodStartsAt,
// 			CurrentPeriodEndsAt:     sub.CurrentPeriodEndsAt,
// 			CancelledAt:             sub.CancelledAt,
// 			CreatedAt:               sub.CreatedAt,
// 			UpdatedAt:               sub.UpdatedAt,
// 		}

// 		if sub.CancelType != nil {
// 			cancelType := string(*sub.CancelType)
// 			item.CancelType = &cancelType
// 		}
// 		if sub.CancelFeedback != nil {
// 			item.CancelFeedback = sub.CancelFeedback
// 		}

// 		// Include price information if available
// 		if sub.Price != nil {
// 			billingCycleDays := 0
// 			if sub.Price.BillingCycleDays != nil {
// 				billingCycleDays = *sub.Price.BillingCycleDays
// 			}
// 			item.Price = &PriceInfo{
// 				ID:               sub.Price.ID.String(),
// 				Name:             sub.Price.DisplayName,
// 				Amount:           sub.Price.Amount,
// 				Currency:         sub.Price.Currency,
// 				BillingCycleDays: billingCycleDays,
// 			}
// 		}

// 		// Include payment method information if available
// 		if sub.PaymentMethod != nil {
// 			item.PaymentMethod = &PaymentMethodInfo{
// 				ID:        sub.PaymentMethod.ID.String(),
// 				Processor: string(sub.PaymentMethod.Processor),
// 				IsActive:  sub.PaymentMethod.IsActive,
// 			}
// 		}

// 		// Parse metadata if available
// 		if len(sub.Metadata) > 0 {
// 			// Metadata is already a json.RawMessage, try to parse it
// 			var metadata map[string]interface{}
// 			if err := json.Unmarshal(sub.Metadata, &metadata); err == nil {
// 				item.Metadata = metadata
// 			}
// 		}

// 		subscriptionItems[i] = item
// 	}

// 	// Convert payments (Postgres canonical)
// 	paymentItems := make([]PaymentItem, 0, len(payments))
// 	for _, p := range payments {
// 		if p == nil {
// 			continue
// 		}
// 		item := PaymentItem{
// 			ID:             p.ID.String(),
// 			SubscriptionID: nil,
// 			Processor:      string(p.Processor),
// 			TransactionID:  p.TransactionID,
// 			Amount:         p.Amount,
// 			Currency:       p.Currency,
// 			PurchasedAt:    p.PurchasedAt,
// 		}
// 		if p.SubscriptionID != nil {
// 			sid := p.SubscriptionID.String()
// 			item.SubscriptionID = &sid
// 		}
// 		if p.ExtensionDays != nil {
// 			item.ExtensionDays = p.ExtensionDays
// 		}
// 		if p.UserRoleGrantID != nil {
// 			uid := p.UserRoleGrantID.String()
// 			item.UserRoleGrantID = &uid
// 		}
// 		if p.Price != nil {
// 			billingCycleDays := 0
// 			if p.Price.BillingCycleDays != nil {
// 				billingCycleDays = *p.Price.BillingCycleDays
// 			}
// 			item.Price = &PriceInfo{
// 				ID:               p.Price.ID.String(),
// 				Name:             p.Price.DisplayName,
// 				Amount:           p.Price.Amount,
// 				Currency:         p.Price.Currency,
// 				BillingCycleDays: billingCycleDays,
// 			}
// 		}
// 		paymentItems = append(paymentItems, item)
// 	}

// 	// Convert payment events
// 	var paymentEventItems []PaymentEventItem
// 	if paymentEvents != nil {
// 		paymentEventItems = make([]PaymentEventItem, len(paymentEvents))
// 		for i, event := range paymentEvents {
// 			paymentEventItems[i] = PaymentEventItem{
// 				EventID:                event.EventID,
// 				SubscriptionID:         event.SubscriptionID,
// 				EventType:              event.EventType,
// 				Processor:              event.Processor,
// 				ProcessorTransactionID: event.ProcessorTransactionID,
// 				Amount:                 event.Amount,
// 				Currency:               event.Currency,
// 				BillingInfo:            event.BillingInfo,
// 				WebhookSource:          event.WebhookSource,
// 				Metadata:               event.Metadata,
// 				Timestamp:              event.Timestamp,
// 				CreatedAt:              event.CreatedAt,
// 			}
// 		}
// 	}

// 	// Convert subscription events
// 	var subscriptionEventItems []SubscriptionEventItem
// 	if subscriptionEvents != nil {
// 		subscriptionEventItems = make([]SubscriptionEventItem, len(subscriptionEvents))
// 		for i, event := range subscriptionEvents {
// 			subscriptionEventItems[i] = SubscriptionEventItem{
// 				EventID:                 event.EventID,
// 				SubscriptionID:          event.SubscriptionID,
// 				EventType:               event.EventType,
// 				Processor:               event.Processor,
// 				ProcessorSubscriptionID: event.ProcessorSubscriptionID,
// 				ProcessorTransactionID:  event.ProcessorTransactionID,
// 				Amount:                  event.Amount,
// 				Currency:                event.Currency,
// 				Metadata:                event.Metadata,
// 				Timestamp:               event.Timestamp,
// 				CreatedAt:               event.CreatedAt,
// 			}
// 		}
// 	}

// 	// Convert stats
// 	var statsResponse *BillingHistoryStats
// 	if stats != nil {
// 		processorBreakdown := make(map[string]ProcessorStatsInfo)
// 		for processor, processorStats := range stats.ProcessorBreakdown {
// 			processorBreakdown[processor] = ProcessorStatsInfo{
// 				TotalCharged:      processorStats.TotalCharged,
// 				TotalCharges:      processorStats.TotalCharges,
// 				SuccessfulCharges: processorStats.SuccessfulCharges,
// 				FailedCharges:     processorStats.FailedCharges,
// 			}
// 		}

// 		statsResponse = &BillingHistoryStats{
// 			TotalCharged:       stats.TotalCharged,
// 			TotalCharges:       stats.TotalCharges,
// 			SuccessfulCharges:  stats.SuccessfulCharges,
// 			FailedCharges:      stats.FailedCharges,
// 			Refunds:            stats.Refunds,
// 			TotalRefunded:      stats.TotalRefunded,
// 			FirstChargeDate:    stats.FirstChargeDate,
// 			LastChargeDate:     stats.LastChargeDate,
// 			ProcessorBreakdown: processorBreakdown,
// 		}
// 	}

// 	return &GetBillingHistoryResponse{
// 		Subscriptions:      subscriptionItems,
// 		Payments:           paymentItems,
// 		PaymentEvents:      paymentEventItems,
// 		SubscriptionEvents: subscriptionEventItems,
// 		Stats:              statsResponse,
// 		Page:               page,
// 		PageSize:           pageSize,
// 		TotalItems:         totalItems,
// 		TotalPages:         totalPages,
// 		HasMore:            hasMore,
// 	}
// }

// Type alias for admin endpoint (same structure)
type GetUserBillingHistoryResponse = GetBillingHistoryResponse

// -------------------------------- Payment Method Responses --------------------------------

// PaymentMethodResponse represents a NMI payment method response
// Payment methods only support NMI card vaults
type PaymentMethodResponse struct {
	ID        string `json:"id"`
	Type      string `json:"type"`      // Always "card" for NMI
	Processor string `json:"processor"` // Always "nmi"
	IsActive  bool   `json:"is_active"`

	// Card-specific fields (NMI only)
	LastFour   *string `json:"last_four,omitempty"`
	CardType   *string `json:"card_type,omitempty"`
	ExpiryDate *string `json:"expiry_date,omitempty"`

	// Common fields
	DisplayName   string    `json:"display_name"`
	FailureReason *string   `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}
