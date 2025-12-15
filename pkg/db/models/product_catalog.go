package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Product represents a product offering (e.g., Premium Membership)
// This represents our product catalog concept
type Product struct {
	bun.BaseModel `bun:"table:billing.products,alias:prod"`

	ID          uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	Slug        string    `bun:"slug,notnull,unique" json:"slug"`
	DisplayName string    `bun:"display_name,notnull" json:"display_name"`
	Description string    `bun:"description,nullzero" json:"description"`

	// Entitlements configuration: map entitlement name -> duration days (nil or 0 means indefinite)
	EntitlementsSpec map[string]*int `bun:"entitlements_spec,type:jsonb,nullzero" json:"entitlements_spec,omitempty"`

	IsActive  bool      `bun:"is_active,notnull,default:true" json:"is_active"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Relationships
	Prices []*Price `bun:"rel:has-many,join:id=product_id" json:"prices,omitempty"`
}

// Price represents a specific pricing option for a product
// This represents pricing options similar to Stripe's pricing model
type Price struct {
	bun.BaseModel `bun:"table:billing.prices,alias:price"`

	ID          uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	ProductID   uuid.UUID `bun:"product_id,notnull" json:"product_id"`
	DisplayName string    `bun:"display_name,notnull" json:"display_name"`
	IsActive    bool      `bun:"is_active,notnull,default:true" json:"is_active"`
	Amount      float64   `bun:"amount,notnull,type:decimal" json:"amount"`
	Currency    string    `bun:"currency,notnull" json:"currency"`

	// Billing interval in days (nullable for one-time purchases)
	// 30 = monthly, 365 = yearly, null = one-time purchase
	BillingCycleDays *int `bun:"billing_cycle_days,nullzero" json:"billing_cycle_days"`

	// Payment processor specific IDs
	NMIPlanID     *string `bun:"nmi_plan_id,nullzero" json:"nmi_plan_id"`
	NMIProvider   *string `bun:"nmi_provider,nullzero" json:"nmi_provider"`
	CCBillPriceID *string `bun:"ccbill_price_id,nullzero" json:"ccbill_price_id"`

	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Relationships
	Product       *Product       `bun:"rel:belongs-to,join:product_id=id" json:"-"`
	Subscriptions []Subscription `bun:"rel:has-many,join:id=price_id" json:"-"`
	Payments      []Payment      `bun:"rel:has-many,join:id=price_id" json:"-"`
}
