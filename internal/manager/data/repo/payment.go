package repo

import (
	"context"
	"database/sql"
	"errors"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"math"
	"time"

	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
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

type PaymentRepo struct {
	db *db.DB
}

func NewPaymentRepo(d *db.DB) *PaymentRepo { return &PaymentRepo{db: d} }

func (r *PaymentRepo) Create(ctx context.Context, payment *models2.Payment) error {
	res, err := r.db.GetDB().NewInsert().Model(payment).Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *PaymentRepo) GetByID(ctx context.Context, id uuid.UUID) (*models2.Payment, error) {
	payment := new(models2.Payment)
	if err := r.db.GetDB().NewSelect().Model(payment).Where("purch.id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *PaymentRepo) GetByUserID(ctx context.Context, userID string) ([]*models2.Payment, error) {
	payments := []*models2.Payment{}
	if err := r.db.GetDB().NewSelect().Model(&payments).Where("purch.user_id = ?", userID).OrderExpr("purch.purchased_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return payments, nil
}

func (r *PaymentRepo) GetByTransactionID(ctx context.Context, processor models2.Processor, transactionID string) (*models2.Payment, error) {
	payment := new(models2.Payment)
	if err := r.db.GetDB().NewSelect().Model(payment).Where("purch.processor = ?", processor).Where("purch.transaction_id = ?", transactionID).Scan(ctx); err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *PaymentRepo) GetByPriceID(ctx context.Context, priceID uuid.UUID) ([]*models2.Payment, error) {
	payments := []*models2.Payment{}
	if err := r.db.GetDB().NewSelect().Model(&payments).Where("purch.price_id = ?", priceID).OrderExpr("purch.purchased_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return payments, nil
}

func (r *PaymentRepo) GetByProcessor(ctx context.Context, processor models2.Processor) ([]*models2.Payment, error) {
	payments := []*models2.Payment{}
	if err := r.db.GetDB().NewSelect().Model(&payments).Where("purch.processor = ?", processor).OrderExpr("purch.purchased_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return payments, nil
}

func (r *PaymentRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models2.Payment)(nil)).Where("purch.id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *PaymentRepo) GetRefundTotalByPaymentID(ctx context.Context, paymentID uuid.UUID) (float64, error) {
	var total sql.NullFloat64
	if err := r.db.GetDB().NewSelect().
		Model((*models2.Payment)(nil)).
		ColumnExpr("COALESCE(SUM(purch.amount), 0)").
		Where("purch.refunded_payment_id = ?", paymentID).
		Scan(ctx, &total); err != nil {
		return 0, err
	}
	return math.Abs(total.Float64), nil
}

func (r *PaymentRepo) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]*models2.Payment, int, error) {
	payments := []*models2.Payment{}
	offset := (page - 1) * pageSize

	count, err := r.db.GetDB().NewSelect().Model((*models2.Payment)(nil)).Where("purch.user_id = ?", userID).Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if err := r.db.GetDB().NewSelect().Model(&payments).Where("purch.user_id = ?", userID).OrderExpr("purch.purchased_at DESC").Limit(pageSize).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}

	return payments, count, nil
}

func (r *PaymentRepo) GetPayments(ctx context.Context, opts query.QueryOptions[PaymentFilters]) ([]*models2.Payment, int64, error) {
	payments := []*models2.Payment{}
	q := r.db.GetDB().NewSelect().Model(&payments)

	q = q.Relation("Price").Relation("Price.Product")

	if opts.Filters.UserID != "" {
		q = q.Where("purch.user_id = ?", opts.Filters.UserID)
	}
	if opts.Filters.PriceID != uuid.Nil {
		q = q.Where("purch.price_id = ?", opts.Filters.PriceID)
	}
	if opts.Filters.Processor != "" {
		q = q.Where("purch.processor = ?", opts.Filters.Processor)
	}
	if opts.Filters.StartDate != nil {
		q = q.Where("purch.purchased_at >= ?", opts.Filters.StartDate)
	}
	if opts.Filters.EndDate != nil {
		q = q.Where("purch.purchased_at <= ?", opts.Filters.EndDate)
	}
	if opts.Filters.MinAmount != nil {
		q = q.Where("purch.amount >= ?", opts.Filters.MinAmount)
	}
	if opts.Filters.MaxAmount != nil {
		q = q.Where("purch.amount <= ?", opts.Filters.MaxAmount)
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	q = q.Limit(opts.GetLimit()).Offset(opts.GetOffset()).OrderExpr("purch.purchased_at DESC")

	if err := q.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return payments, int64(total), nil
}
