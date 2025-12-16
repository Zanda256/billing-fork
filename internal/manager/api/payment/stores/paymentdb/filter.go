package paymentdb

import (
	"github.com/google/uuid"
	"time"
)

type PaymentFilters struct {
	UserID    string
	PriceID   uuid.UUID
	Processor string
	StartDate *time.Time
	EndDate   *time.Time
	MinAmount *float64
	MaxAmount *float64
}
