package repo

import (
	"context"
	"errors"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrIdempotencyConflict   = errors.New("idempotency record already exists")
	ErrIdempotencyNotUpdated = errors.New("idempotency record not updated")
)

type IdempotencyRepo struct {
	db *db.DB
}

func NewIdempotencyRepo(d *db.DB) *IdempotencyRepo { return &IdempotencyRepo{db: d} }

func (r *IdempotencyRepo) Create(ctx context.Context, req *models.IdempotencyRequest) error {
	_, err := r.db.GetDB().NewInsert().Model(req).Exec(ctx)
	if err != nil {
		if isDuplicateKeyErr(err) {
			return ErrIdempotencyConflict
		}
		return err
	}
	return nil
}

func isDuplicateKeyErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

func (r *IdempotencyRepo) GetByOperationAndKey(ctx context.Context, operation, key string) (*models.IdempotencyRequest, error) {
	req := new(models.IdempotencyRequest)
	if err := r.db.GetDB().NewSelect().Model(req).
		Where("idem.operation = ?", operation).
		Where("idem.key = ?", key).
		Scan(ctx); err != nil {
		return nil, err
	}
	return req, nil
}

func (r *IdempotencyRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string, result []byte) error {
	res, err := r.db.GetDB().NewUpdate().Model((*models.IdempotencyRequest)(nil)).
		Set("status = ?", status).
		Set("result_json = ?", result).
		Set("updated_at = NOW()").
		Where("idem.id = ?", id).
		Exec(ctx)
	if err != nil {
		return err
	}
	if rows, err := res.RowsAffected(); err != nil {
		return err
	} else if rows == 0 {
		return ErrIdempotencyNotUpdated
	}
	return nil
}
