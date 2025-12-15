package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// IdempotencyRequest stores a record of processed operations to ensure idempotency
type IdempotencyRequest struct {
	bun.BaseModel `bun:"table:billing.idempotency_requests,alias:idem"`

	ID         uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	Operation  string    `bun:"operation,notnull" json:"operation"`
	Key        string    `bun:"key,notnull" json:"key"`
	UserID     *string   `bun:"user_id,nullzero" json:"user_id,omitempty"`
	Status     string    `bun:"status,notnull" json:"status"` // pending|success|failed
	ResultJSON []byte    `bun:"result_json,type:jsonb,nullzero" json:"result_json,omitempty"`
	CreatedAt  time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

func (IdempotencyRequest) TableName() string { return "idempotency_requests" }
