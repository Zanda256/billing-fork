package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// PaymentMethod represents a stored payment method across multiple processors
// This replaces processor-specific payment method tables
type PaymentMethod struct {
	bun.BaseModel `bun:"table:billing.payment_methods,alias:pm"`

	ID        uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	UserID    string    `bun:"user_id,notnull" json:"user_id"`
	Processor Processor `bun:"processor,notnull" json:"processor"` // "nmi", "ccbill", etc.
	Provider  *string   `bun:"processor_provider,nullzero" json:"processor_provider"`

	// Processor-specific vault/payment method identifiers
	VaultID              string  `bun:"vault_id,notnull" json:"vault_id"`                             // Primary identifier in processor's system
	BillingID            *string `bun:"billing_id,nullzero" json:"billing_id"`                        // Secondary identifier (e.g., subscription ID)
	InitialTransactionID string  `bun:"initial_transaction_id,notnull" json:"initial_transaction_id"` // Transaction that created this vault

	// Payment method status and metadata
	IsActive      bool    `bun:"is_active,notnull,default:true" json:"is_active"` // Can this method be used for rebills?
	LastFour      *string `bun:"last_four,nullzero" json:"last_four"`             // Last 4 digits of card
	CardType      *string `bun:"card_type,nullzero" json:"card_type"`             // "Visa", "MasterCard", etc.
	ExpiryDate    *string `bun:"expiry_date,nullzero" json:"expiry_date"`         // "MM/YY" format
	FailureReason *string `bun:"failure_reason,nullzero" json:"failure_reason"`   // Reason if inactive

	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Relationships
	Subscriptions []*Subscription `bun:"rel:has-many,join:id=payment_method_id" json:"subscriptions,omitempty"`
}
