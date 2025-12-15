package models

import (
	"time"

	"github.com/uptrace/bun"
)

type CCBillUsernameAlias struct {
	bun.BaseModel `bun:"table:billing.ccbill_username_aliases,alias:ccbill_alias"`

	Alias     string    `bun:"alias,pk" json:"alias"`
	UserID    string    `bun:"user_id,unique,notnull" json:"user_id"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
}
