package webhook

import (
	"github.com/doujins-org/doujins-billing/internal/manager/api/types"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransactionSubscriptionID_Fallbacks(t *testing.T) {
	tests := []struct {
		name string
		body *types.NMITransactionEventBody
		want string
	}{
		{
			name: "subscription reference",
			body: &types.NMITransactionEventBody{
				Subscription: &types.NMISubscriptionRef{SubscriptionID: " sub-123 "},
			},
			want: "sub-123",
		},
		{
			name: "transaction detail subscription",
			body: &types.NMITransactionEventBody{
				TransactionDetail: &types.NMITransactionDetail{
					Subscription: &types.NMISubscriptionRef{SubscriptionID: " detail-456 "},
				},
			},
			want: "detail-456",
		},
		{
			name: "order id fallback",
			body: &types.NMITransactionEventBody{OrderID: " order-789 "},
			want: "order-789",
		},
		{
			name: "transaction detail order id fallback",
			body: &types.NMITransactionEventBody{
				TransactionDetail: &types.NMITransactionDetail{OrderID: " detail-order "},
			},
			want: "detail-order",
		},
		{
			name: "po number fallback",
			body: &types.NMITransactionEventBody{PONumber: " po-001 "},
			want: "po-001",
		},
		{
			name: "transaction detail po number fallback",
			body: &types.NMITransactionEventBody{
				TransactionDetail: &types.NMITransactionDetail{PONumber: " detail-po "},
			},
			want: "detail-po",
		},
		{
			name: "customer id fallback",
			body: &types.NMITransactionEventBody{CustomerID: " cust-002 "},
			want: "cust-002",
		},
		{
			name: "transaction detail customer id fallback",
			body: &types.NMITransactionEventBody{
				TransactionDetail: &types.NMITransactionDetail{CustomerID: " detail-cust "},
			},
			want: "detail-cust",
		},
		{
			name: "empty payload",
			body: &types.NMITransactionEventBody{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, transactionSubscriptionID(tc.body))
		})
	}
}

func TestTransactionActionSource(t *testing.T) {
	require.Equal(t, "recurring", transactionActionSource(&types.NMITransactionEventBody{
		Action: &types.NMIAction{Source: "Recurring"},
	}))

	require.Equal(t, "retry", transactionActionSource(&types.NMITransactionEventBody{
		TransactionDetail: &types.NMITransactionDetail{
			Action: &types.NMIAction{Source: "Retry"},
		},
	}))

	require.Equal(t, "", transactionActionSource(&types.NMITransactionEventBody{}))
}

func TestIsRecurringSource(t *testing.T) {
	require.True(t, isRecurringSource("recurring"))
	require.True(t, isRecurringSource("RETRY"))
	require.False(t, isRecurringSource("api"))
	require.False(t, isRecurringSource(""))
}
