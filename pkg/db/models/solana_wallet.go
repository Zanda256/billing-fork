package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// SolanaWallet represents a user's Solana wallet address
// UserID uses OIDC subject (string), not UUID
type SolanaWallet struct {
	bun.BaseModel `bun:"table:billing.solana_wallets,alias:sw"`

	ID         uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID     string     `bun:"user_id,notnull" json:"user_id"`
	Address    string     `bun:"address,notnull" json:"address"` // Base58 encoded address
	IsVerified bool       `bun:"is_verified,notnull,default:false" json:"is_verified"`
	VerifiedAt *time.Time `bun:"verified_at,nullzero" json:"verified_at,omitempty"`
	CreatedAt  time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time  `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
