package payment

import (
	"context"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment/stores/paymentdb"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
)

type Storer interface {
	Create(ctx context.Context, payment *models2.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*models2.Payment, error)
	GetByUserID(ctx context.Context, userID string) ([]*models2.Payment, error)
	GetByTransactionID(ctx context.Context, processor models2.Processor, transactionID string) (*models2.Payment, error)
	GetByPriceID(ctx context.Context, priceID uuid.UUID) ([]*models2.Payment, error)
	GetByProcessor(ctx context.Context, processor models2.Processor) ([]*models2.Payment, error)
	GetRefundTotalByPaymentID(ctx context.Context, paymentID uuid.UUID) (float64, error)
	GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]*models2.Payment, int, error)
	GetPayments(ctx context.Context, opts query.QueryOptions[paymentdb.PaymentFilters]) ([]*models2.Payment, int64, error)
}

type PaymentService struct {
	repo Storer
}

const refundEpsilon = 0.0001

type GetPaymentsFilters = paymentdb.PaymentFilters

func NewPaymentService(repo Storer) *PaymentService {
	return &PaymentService{repo: repo}
}

func (s *PaymentService) Create(ctx context.Context, payment *models2.Payment) error {
	return s.repo.Create(ctx, payment)
}

func (s *PaymentService) GetByID(ctx context.Context, id uuid.UUID) (*models2.Payment, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *PaymentService) GetByUserID(ctx context.Context, userID string) ([]*models2.Payment, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *PaymentService) GetByTransactionID(ctx context.Context, processor models2.Processor, transactionID string) (*models2.Payment, error) {
	return s.repo.GetByTransactionID(ctx, processor, transactionID)
}

func (s *PaymentService) GetByPriceID(ctx context.Context, priceID uuid.UUID) ([]*models2.Payment, error) {
	return s.repo.GetByPriceID(ctx, priceID)
}

func (s *PaymentService) GetByProcessor(ctx context.Context, processor models2.Processor) ([]*models2.Payment, error) {
	return s.repo.GetByProcessor(ctx, processor)
}

func (s *PaymentService) Update(ctx context.Context, payment *models2.Payment) error {
	return errors.New("payments are immutable; updates are not supported")
}

func (s *PaymentService) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.New("payments cannot be deleted")
}

// Refund records a refund as a negative payment entry linked by transaction ID
// Note: Processors should handle the actual money movement; this persists the event.
func (s *PaymentService) Refund(ctx context.Context, originalPaymentID uuid.UUID, refundTransactionID string, amount float64) (*models2.Payment, error) {
	orig, err := s.GetByID(ctx, originalPaymentID)
	if err != nil {
		return nil, err
	}
	if amount <= 0 {
		return nil, errors.New("refund amount must be > 0")
	}
	if strings.TrimSpace(refundTransactionID) == "" {
		return nil, errors.New("refund transaction id is required")
	}

	refundedTotal, err := s.repo.GetRefundTotalByPaymentID(ctx, originalPaymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate refunded total: %w", err)
	}
	if amount > orig.Amount {
		return nil, errors.New("refund amount cannot exceed original payment amount")
	}
	if refundedTotal > 0 {
		if amount+refundedTotal-orig.Amount > refundEpsilon {
			return nil, fmt.Errorf("refund total would exceed original payment (refunded %.2f of %.2f)", refundedTotal, orig.Amount)
		}
	}

	refund := &models2.Payment{
		ID:             uuid.New(),
		UserID:         orig.UserID,
		PriceID:        orig.PriceID,
		SubscriptionID: orig.SubscriptionID,
		RefundedPaymentID: func() *uuid.UUID {
			id := orig.ID
			return &id
		}(),
		Processor:     orig.Processor,
		TransactionID: refundTransactionID,
		Amount:        -amount,
		Currency:      orig.Currency,
		PurchasedAt:   time.Now(),
		CreatedAt:     time.Now(),
	}
	if err := s.Create(ctx, refund); err != nil {
		return nil, err
	}
	return refund, nil
}

func (s *PaymentService) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]*models2.Payment, int, error) {
	return s.repo.GetPaginatedByUserID(ctx, userID, page, pageSize)
}

func (s *PaymentService) GetPayments(ctx context.Context, queryOpts query.QueryOptions[GetPaymentsFilters]) ([]*models2.Payment, int64, error) {
	repoOpts := query.QueryOptions[paymentdb.PaymentFilters]{
		Filters: paymentdb.PaymentFilters{
			UserID:    queryOpts.Filters.UserID,
			PriceID:   queryOpts.Filters.PriceID,
			Processor: queryOpts.Filters.Processor,
			StartDate: queryOpts.Filters.StartDate,
			EndDate:   queryOpts.Filters.EndDate,
			MinAmount: queryOpts.Filters.MinAmount,
			MaxAmount: queryOpts.Filters.MaxAmount,
		},
		Limit:    queryOpts.Limit,
		Offset:   queryOpts.Offset,
		Page:     queryOpts.Page,
		PageSize: queryOpts.PageSize,
		All:      queryOpts.All,
	}

	return s.repo.GetPayments(ctx, repoOpts)
}
