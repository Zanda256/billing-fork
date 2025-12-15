package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// SolanaWalletChallenge stores the latest verification challenge for a wallet
// Challenges are per user+address and expire after a short TTL.
type SolanaWalletChallenge struct {
	bun.BaseModel `bun:"table:billing.solana_wallet_challenges,alias:swc"`

	ID        uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID    string    `bun:"user_id,notnull" json:"user_id"`
	Address   string    `bun:"address,notnull" json:"address"`
	Message   string    `bun:"message,notnull" json:"message"`
	Nonce     string    `bun:"nonce,notnull" json:"nonce"`
	ExpiresAt time.Time `bun:"expires_at,notnull" json:"expires_at"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
