package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// WebhookEvent tracks incoming webhook events for deduplication and debugging
type WebhookEvent struct {
	bun.BaseModel `bun:"table:billing.webhook_events,alias:wh"`

	ID uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`

	// Event identification
	Processor string  `bun:",notnull" json:"processor"`
	EventID   *string `bun:"" json:"event_id,omitempty"` // Processor's event ID
	EventType string  `bun:",notnull" json:"event_type"`

	// Processing status
	Status      string     `bun:",notnull,default:'pending'" json:"status"` // pending, processed, failed, duplicate
	ProcessedAt *time.Time `bun:",nullzero" json:"processed_at,omitempty"`

	// Request details
	RawPayload string            `bun:",type:text,notnull" json:"raw_payload"`
	Headers    map[string]string `bun:",type:jsonb" json:"headers,omitempty"`
	IPAddress  string            `bun:",notnull" json:"ip_address"`

	// Signature verification
	SignatureValid *bool   `bun:"" json:"signature_valid,omitempty"`
	Signature      *string `bun:"" json:"signature,omitempty"`

	// Processing results
	ProcessingResult map[string]interface{} `bun:",type:jsonb" json:"processing_result,omitempty"`
	ErrorMessage     *string                `bun:"" json:"error_message,omitempty"`

	// Reference to related entities
	SubscriptionID *uuid.UUID `bun:",type:uuid" json:"subscription_id,omitempty"`
	UserID         *string    `bun:"" json:"user_id,omitempty"`

	// Retry tracking
	ProcessingAttempts int        `bun:",default:0" json:"processing_attempts"`
	LastAttemptAt      *time.Time `bun:",nullzero" json:"last_attempt_at,omitempty"`
	NextAttemptAt      *time.Time `bun:",nullzero" json:"next_attempt_at,omitempty"`

	// Timestamps
	ReceivedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"received_at"`
	CreatedAt  time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// SolanaTransaction tracks Solana payment transactions
type SolanaTransaction struct {
	bun.BaseModel `bun:"table:billing.solana_transactions,alias:stx"`

	ID     uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID *string   `bun:"" json:"user_id,omitempty"`

	// Transaction details
	Signature *string `bun:"" json:"signature,omitempty"` // Solana transaction signature
	Status    string  `bun:",notnull" json:"status"`      // pending, confirmed, failed

	// Payment details
	Amount    float64 `bun:",type:decimal(18,9),notnull" json:"amount"`
	Token     string  `bun:",notnull" json:"token"`      // SOL, USDC, PYUSD
	TokenMint string  `bun:",notnull" json:"token_mint"` // Token mint address

	// Addresses
	FromAddress string     `bun:",notnull" json:"from_address"` // Payer address
	ToAddress   string     `bun:",notnull" json:"to_address"`   // Recipient address
	IntentID    *uuid.UUID `bun:"intent_id" json:"intent_id,omitempty"`

	// Product reference
	ProductID  *uuid.UUID `bun:",type:uuid" json:"product_id,omitempty"`
	PurchaseID *uuid.UUID `bun:",type:uuid" json:"purchase_id,omitempty"`

	// Blockchain details
	BlockTime      *time.Time `bun:",nullzero" json:"block_time,omitempty"`
	Slot           *int64     `bun:"" json:"slot,omitempty"`
	Confirmations  int        `bun:",default:0" json:"confirmations"`
	TransactionFee *float64   `bun:",type:decimal(18,9)" json:"transaction_fee,omitempty"`

	// Processing metadata
	ProcessingResult map[string]interface{} `bun:",type:jsonb" json:"processing_result,omitempty"`
	ErrorMessage     *string                `bun:"" json:"error_message,omitempty"`

	// QR code reference (for payments initiated via QR)
	QRCodeID *string `bun:"" json:"qr_code_id,omitempty"`

	// Expiration for pending transactions
	ExpiresAt *time.Time `bun:",nullzero" json:"expires_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

const (
	TableWebhookEvents      = "webhook_events"
	TableSolanaTransactions = "solana_transactions"
)
