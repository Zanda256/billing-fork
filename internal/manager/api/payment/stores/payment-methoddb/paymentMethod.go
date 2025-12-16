package payment_methoddb

import (
	"database/sql"
	"fmt"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"github.com/google/uuid"
	"strings"

	"context"
	"errors"
)

type PaymentMethodStore struct {
	db *db.DB
}

func NewPaymentMethodRepo(d *db.DB) *PaymentMethodStore { return &PaymentMethodStore{db: d} }

var (
	ErrPaymentMethodNotFound = errors.New("payment method not found")
)

func (r *PaymentMethodStore) Create(ctx context.Context, m *models2.PaymentMethod) error {
	res, err := r.db.GetDB().NewInsert().Model(m).Exec(ctx)
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

func (r *PaymentMethodStore) GetByID(ctx context.Context, id uuid.UUID) (*models2.PaymentMethod, error) {
	pm := new(models2.PaymentMethod)
	err := r.db.GetDB().NewSelect().Model(pm).Where("pm.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("payment method %s: %w", id, ErrPaymentMethodNotFound)
		}
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodStore) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models2.PaymentMethod)(nil)).Where("pm.id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return ErrPaymentMethodNotFound
	}

	return nil
}

func (r *PaymentMethodStore) GetByUserID(ctx context.Context, userID string) ([]*models2.PaymentMethod, error) {
	methods := []*models2.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.user_id = ?", userID).
		OrderExpr("pm.created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodStore) GetActiveByUserID(ctx context.Context, userID string) ([]*models2.PaymentMethod, error) {
	methods := []*models2.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.user_id = ?", userID).
		Where("pm.is_active = ?", true).
		OrderExpr("pm.created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodStore) ListByUserID(ctx context.Context, userID string, includeInactive bool, limit, offset int) ([]*models2.PaymentMethod, int64, error) {
	countQuery := r.db.GetDB().NewSelect().Model((*models2.PaymentMethod)(nil)).
		Where("pm.user_id = ?", userID)
	if !includeInactive {
		countQuery.Where("pm.is_active = ?", true)
	}

	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	methods := []*models2.PaymentMethod{}
	dataQuery := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.user_id = ?", userID).
		OrderExpr("pm.created_at DESC")
	if !includeInactive {
		dataQuery.Where("pm.is_active = ?", true)
	}
	if limit > 0 {
		dataQuery.Limit(limit)
	}
	if offset > 0 {
		dataQuery.Offset(offset)
	}

	if err := dataQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return methods, int64(total), nil
}

func (r *PaymentMethodStore) GetByVaultID(ctx context.Context, provider, vaultID string) (*models2.PaymentMethod, error) {
	pm := new(models2.PaymentMethod)
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = "mobius"
	}

	query := r.db.GetDB().NewSelect().Model(pm).
		Where("pm.processor = ?", models2.ProcessorNMI).
		Where("pm.vault_id = ?", vaultID)

	if provider == "mobius" {
		query = query.Where("(pm.processor_provider = ? OR pm.processor_provider IS NULL OR pm.processor_provider = '')", provider)
	} else {
		query = query.Where("pm.processor_provider = ?", provider)
	}

	err := query.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodStore) GetByBillingID(ctx context.Context, provider, billingID string) (*models2.PaymentMethod, error) {
	pm := new(models2.PaymentMethod)
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = "mobius"
	}

	query := r.db.GetDB().NewSelect().Model(pm).
		Where("pm.processor = ?", models2.ProcessorNMI).
		Where("pm.billing_id = ?", billingID)

	if provider == "mobius" {
		query = query.Where("(pm.processor_provider = ? OR pm.processor_provider IS NULL OR pm.processor_provider = '')", provider)
	} else {
		query = query.Where("pm.processor_provider = ?", provider)
	}

	err := query.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodStore) GetByInitialTransactionID(ctx context.Context, provider, initialTransactionID string) (*models2.PaymentMethod, error) {
	pm := new(models2.PaymentMethod)
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = "mobius"
	}

	query := r.db.GetDB().NewSelect().Model(pm).
		Where("pm.processor = ?", models2.ProcessorNMI).
		Where("pm.initial_transaction_id = ?", initialTransactionID)

	if provider == "mobius" {
		query = query.Where("(pm.processor_provider = ? OR pm.processor_provider IS NULL OR pm.processor_provider = '')", provider)
	} else {
		query = query.Where("pm.processor_provider = ?", provider)
	}

	err := query.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodStore) Update(ctx context.Context, method *models2.PaymentMethod) error {
	res, err := r.db.GetDB().NewUpdate().Model(method).WherePK().Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return ErrPaymentMethodNotFound
	}

	return nil
}

func (r *PaymentMethodStore) DeactivateByUserID(ctx context.Context, userID string) error {
	res, err := r.db.GetDB().NewUpdate().
		Model((*models2.PaymentMethod)(nil)).
		Set("is_active = ?", false).
		Where("pm.user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = res.RowsAffected()
	if err != nil {
		return err
	}

	return nil
}

func (r *PaymentMethodStore) ActivateByID(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewUpdate().
		Model((*models2.PaymentMethod)(nil)).
		Set("is_active = ?", true).
		Where("pm.id = ?", id).
		Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return ErrPaymentMethodNotFound
	}

	return nil
}

func (r *PaymentMethodStore) GetAllNMI(ctx context.Context) ([]*models2.PaymentMethod, error) {
	methods := []*models2.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.processor = ?", models2.ProcessorNMI).
		OrderExpr("pm.created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodStore) GetActiveNMI(ctx context.Context) ([]*models2.PaymentMethod, error) {
	methods := []*models2.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.processor = ?", models2.ProcessorNMI).
		Where("pm.is_active = ?", true).
		OrderExpr("pm.created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodStore) GetNMIByUserID(ctx context.Context, userID string) ([]*models2.PaymentMethod, error) {
	methods := []*models2.PaymentMethod{}
	if err := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.user_id = ?", userID).
		Where("pm.processor = ?", models2.ProcessorNMI).
		OrderExpr("pm.created_at DESC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodStore) GetActiveNMIByUserID(ctx context.Context, userID string) ([]*models2.PaymentMethod, error) {
	methods := []*models2.PaymentMethod{}
	if err := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.user_id = ?", userID).
		Where("pm.processor = ?", models2.ProcessorNMI).
		Where("pm.is_active = ?", true).
		OrderExpr("pm.created_at DESC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodStore) ExistsForUser(ctx context.Context, id uuid.UUID, userID string) (bool, error) {
	count, err := r.db.GetDB().NewSelect().
		Model((*models2.PaymentMethod)(nil)).
		Where("pm.id = ?", id).
		Where("pm.user_id = ?", userID).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PaymentMethodStore) WithTx(txdb *db.DB) *PaymentMethodStore {
	return NewPaymentMethodRepo(txdb)
}

func (r *PaymentMethodStore) GetByProcessor(ctx context.Context, processor models2.Processor) ([]*models2.PaymentMethod, error) {
	methods := []*models2.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.processor = ?", processor).
		OrderExpr("pm.created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodStore) GetActiveByProcessor(ctx context.Context, processor models2.Processor) ([]*models2.PaymentMethod, error) {
	methods := []*models2.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("pm.processor = ?", processor).
		Where("pm.is_active = ?", true).
		OrderExpr("pm.created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodStore) RequireByID(ctx context.Context, id uuid.UUID) (*models2.PaymentMethod, error) {
	pm, err := r.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrPaymentMethodNotFound) {
			return nil, err
		}
		return nil, err
	}
	return pm, nil
}
