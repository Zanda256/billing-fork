package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// SolanaPaymentIntent persists metadata for both direct and Solana Pay flows.
type SolanaPaymentIntent struct {
	bun.BaseModel `bun:"table:billing.solana_payment_intents,alias:spi"`

	ID                     uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID                 string     `bun:"user_id,notnull" json:"user_id"`
	PriceID                uuid.UUID  `bun:"price_id,notnull" json:"price_id"`
	FlowType               string     `bun:"flow_type,notnull" json:"flow_type"`
	Token                  string     `bun:"token,notnull" json:"token"`
	TokenMint              string     `bun:"token_mint,notnull" json:"token_mint"`
	Amount                 float64    `bun:"amount,notnull,type:decimal(18,9)" json:"amount"`
	Currency               string     `bun:"currency,notnull" json:"currency"`
	ExpectedAmountLamports uint64     `bun:"expected_amount_lamports,notnull" json:"expected_amount_lamports"`
	PayerWallet            *string    `bun:"payer_wallet" json:"payer_wallet,omitempty"`
	RecipientWallet        string     `bun:"recipient_wallet,notnull" json:"recipient_wallet"`
	Reference              *string    `bun:"reference" json:"reference,omitempty"`
	Memo                   *string    `bun:"memo" json:"memo,omitempty"`
	Status                 string     `bun:"status,notnull" json:"status"`
	Signature              *string    `bun:"signature" json:"signature,omitempty"`
	TransactionSignature   *string    `bun:"transaction_signature" json:"transaction_signature,omitempty"`
	ErrorMessage           *string    `bun:"error_message" json:"error_message,omitempty"`
	ExpiresAt              *time.Time `bun:"expires_at" json:"expires_at,omitempty"`
	ConfirmedAt            *time.Time `bun:"confirmed_at" json:"confirmed_at,omitempty"`
	CreatedAt              time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt              time.Time  `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
