package models

import (
	"github.com/uptrace/bun"
)

// (Removed) GrantSource: use EntitlementSourceType instead (admin, grace, one_off, subscription)

// Processor represents payment processor types
type Processor string

const (
	ProcessorNMI    Processor = "nmi"
	ProcessorCCBill Processor = "ccbill"
	ProcessorSolana Processor = "solana"
	ProcessorPayPal Processor = "paypal"
)

var ModelRegistry = []any{
	(*Product)(nil),
	(*Price)(nil),
	(*Payment)(nil),
	(*Purchase)(nil),
	(*Subscription)(nil),
	(*PaymentMethod)(nil),
	(*SolanaWallet)(nil),
	(*SolanaWalletChallenge)(nil),
	(*SolanaPaymentIntent)(nil),
	(*SolanaTransaction)(nil),
	(*NotificationQueue)(nil),
	(*Entitlement)(nil),
	(*IdempotencyRequest)(nil),
}

func RegisterModels(db *bun.DB) {
	db.RegisterModel(ModelRegistry...)
}
