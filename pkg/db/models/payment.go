package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Payment represents a payment event (both one-time and subscription payments)
// This is an immutable event log of all payments received
type Payment struct {
	bun.BaseModel `bun:"table:billing.payments,alias:purch"`

	ID      uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	UserID  string    `bun:"user_id,notnull" json:"user_id"`
	PriceID uuid.UUID `bun:"price_id,notnull" json:"price_id"`

	// Optional linkage to the subscription that generated this payment
	SubscriptionID *uuid.UUID `bun:"subscription_id,type:uuid,nullzero" json:"subscription_id,omitempty"`

	// Optional linkage back to the payment that this record refunds
	RefundedPaymentID *uuid.UUID `bun:"refunded_payment_id,type:uuid,nullzero" json:"refunded_payment_id,omitempty"`

	Processor         Processor `bun:"processor,notnull" json:"processor"`
	ProcessorProvider *string   `bun:"processor_provider,nullzero" json:"processor_provider"`
	TransactionID     string    `bun:"transaction_id,notnull" json:"transaction_id"`

	// Payment details
	Amount   float64 `bun:"amount,notnull,type:numeric" json:"amount"`
	Currency string  `bun:"currency,notnull,default:'USD'" json:"currency"`

	PurchasedAt time.Time `bun:"purchased_at,notnull,default:current_timestamp" json:"purchased_at"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`

	// Relationships
	Price        *Price         `bun:"rel:belongs-to,join:price_id=id" json:"price,omitempty"`
	Subscription *Subscription  `bun:"rel:belongs-to,join:subscription_id=id" json:"subscription,omitempty"`
	Entitlements []*Entitlement `bun:"rel:has-many,join:id=source_id" json:"entitlements,omitempty"`
}

// Purchase is an alias for Payment for backward compatibility
type Purchase = Payment
