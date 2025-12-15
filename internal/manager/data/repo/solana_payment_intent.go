package repo

import (
	"context"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type SolanaPaymentIntentRepo struct {
	db *db.DB
}

func NewSolanaPaymentIntentRepo(d *db.DB) *SolanaPaymentIntentRepo {
	return &SolanaPaymentIntentRepo{db: d}
}

func (r *SolanaPaymentIntentRepo) Insert(ctx context.Context, intent *models.SolanaPaymentIntent) error {
	_, err := r.db.GetDB().NewInsert().Model(intent).Exec(ctx)
	return err
}

func (r *SolanaPaymentIntentRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.SolanaPaymentIntent, error) {
	intent := new(models.SolanaPaymentIntent)
	if err := r.db.GetDB().NewSelect().Model(intent).
		Where("spi.id = ?", id).
		Scan(ctx); err != nil {
		return nil, err
	}
	return intent, nil
}

func (r *SolanaPaymentIntentRepo) GetByReference(ctx context.Context, reference string) (*models.SolanaPaymentIntent, error) {
	intent := new(models.SolanaPaymentIntent)
	if err := r.db.GetDB().NewSelect().Model(intent).
		Where("spi.reference = ?", reference).
		Scan(ctx); err != nil {
		return nil, err
	}
	return intent, nil
}

func (r *SolanaPaymentIntentRepo) update(ctx context.Context, intentID uuid.UUID, set map[string]any, conditions ...func(*bun.UpdateQuery) *bun.UpdateQuery) (int64, error) {
	query := r.db.GetDB().NewUpdate().Model((*models.SolanaPaymentIntent)(nil)).
		Where("spi.id = ?", intentID)

	for column, value := range set {
		query = query.Set(column+" = ?", value)
	}

	for _, apply := range conditions {
		if apply != nil {
			query = apply(query)
		}
	}

	res, err := query.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *SolanaPaymentIntentRepo) MarkProcessing(ctx context.Context, intentID uuid.UUID, newStatus, expectedStatus string) (int64, error) {
	return r.update(ctx, intentID, map[string]any{
		"status":     newStatus,
		"updated_at": time.Now(),
	}, func(q *bun.UpdateQuery) *bun.UpdateQuery {
		return q.Where("status = ?", expectedStatus)
	})
}

func (r *SolanaPaymentIntentRepo) MarkConfirmed(ctx context.Context, intentID uuid.UUID, newStatus string, allowedStatuses []string, signature string) (int64, error) {
	now := time.Now()
	return r.update(ctx, intentID, map[string]any{
		"status":                newStatus,
		"transaction_signature": signature,
		"signature":             signature,
		"confirmed_at":          &now,
		"updated_at":            now,
	}, func(q *bun.UpdateQuery) *bun.UpdateQuery {
		return q.Where("status IN (?)", bun.In(allowedStatuses))
	})
}

func (r *SolanaPaymentIntentRepo) MarkFailed(ctx context.Context, intentID uuid.UUID, failedStatus, message string) (int64, error) {
	return r.update(ctx, intentID, map[string]any{
		"status":        failedStatus,
		"error_message": message,
		"updated_at":    time.Now(),
	})
}
