package email

import (
	"github.com/google/uuid"
	"time"
)

type SubscriptionEmailData struct {
	UserEmail      string
	Username       string
	SubscriptionID uuid.UUID
	Amount         float64
	Currency       string
	PeriodStart    time.Time
	PeriodEnd      time.Time
	PaymentMethod  string
	TransactionID  string
}
