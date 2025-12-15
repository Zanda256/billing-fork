package repo

import (
	"context"
	"errors"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	"github.com/google/uuid"
)

type EntitlementRepo struct {
	db *db.DB
}

func NewEntitlementRepo(d *db.DB) *EntitlementRepo { return &EntitlementRepo{db: d} }

func (r *EntitlementRepo) IsEntitled(ctx context.Context, userID, entitlement string, at time.Time) (bool, error) {
	q := r.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		Where("ent.user_id = ?", userID).
		Where("ent.entitlement = ?", entitlement).
		Where("ent.start_at <= ?", at).
		Where("(ent.end_at IS NULL OR ent.end_at > ?)", at).
		Where("ent.revoked_at IS NULL")
	return q.Exists(ctx)
}

func (r *EntitlementRepo) HasActiveIndefinite(ctx context.Context, userID, entitlement string, at time.Time) (bool, error) {
	q := r.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		Where("ent.user_id = ?", userID).
		Where("ent.entitlement = ?", entitlement).
		Where("ent.revoked_at IS NULL AND ent.end_at IS NULL").
		Where("ent.start_at <= ?", at)
	return q.Exists(ctx)
}

func (r *EntitlementRepo) GetLatestActive(ctx context.Context, userID, entitlement string) (*models.Entitlement, error) {
	var ent models.Entitlement
	err := r.db.GetDB().NewSelect().
		Model(&ent).
		Where("ent.user_id = ? AND ent.entitlement = ?", userID, entitlement).
		Where("ent.revoked_at IS NULL").
		OrderExpr("ent.start_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &ent, nil
}

func (r *EntitlementRepo) GetLatestFiniteActive(ctx context.Context, userID, entitlement string, at time.Time) (*models.Entitlement, error) {
	var ent models.Entitlement
	err := r.db.GetDB().NewSelect().
		Model(&ent).
		Where("ent.user_id = ? AND ent.entitlement = ?", userID, entitlement).
		Where("ent.revoked_at IS NULL").
		Where("ent.end_at IS NOT NULL").
		Where("ent.start_at <= ?", at).
		Where("ent.end_at > ?", at).
		OrderExpr("ent.end_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &ent, nil
}

func (r *EntitlementRepo) Insert(ctx context.Context, entitlement *models.Entitlement) error {
	res, err := r.db.GetDB().NewInsert().Model(entitlement).Exec(ctx)
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

func (r *EntitlementRepo) ListActiveEntitlements(ctx context.Context, userID string, at time.Time) ([]string, error) {
	var out []string
	if err := r.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		ColumnExpr("DISTINCT ent.entitlement").
		Where("ent.user_id = ?", userID).
		Where("ent.start_at <= ?", at).
		Where("(ent.end_at IS NULL OR ent.end_at > ?)", at).
		Where("ent.revoked_at IS NULL").
		Scan(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *EntitlementRepo) ListActiveRecords(ctx context.Context, userID string, at time.Time) ([]models.Entitlement, error) {
	ents := []models.Entitlement{}
	if err := r.db.GetDB().NewSelect().
		Model(&ents).
		Where("ent.user_id = ?", userID).
		Where("ent.revoked_at IS NULL").
		Where("ent.start_at <= ?", at).
		Where("(ent.end_at IS NULL OR ent.end_at > ?)", at).
		OrderExpr("ent.start_at ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return ents, nil
}

func (r *EntitlementRepo) EndActiveBySubscription(ctx context.Context, subscriptionID uuid.UUID, endAt time.Time, reason *models.EntitlementRevokeReason) error {
	now := time.Now()
	_, err := r.db.GetDB().NewUpdate().
		Model((*models.Entitlement)(nil)).
		Set("end_at = ?", endAt).
		Set("revoked_at = ?", now).
		Set("revoke_reason = ?", reason).
		Set("updated_at = ?", now).
		Where("ent.source_type = ?", models.EntitlementSourceSubscription).
		Where("ent.source_id = ?", subscriptionID).
		Where("ent.end_at IS NULL").
		Exec(ctx)
	return err
}

func (r *EntitlementRepo) EndActiveByPayment(ctx context.Context, paymentID uuid.UUID, endAt time.Time, reason *models.EntitlementRevokeReason) error {
	now := time.Now()
	_, err := r.db.GetDB().NewUpdate().
		Model((*models.Entitlement)(nil)).
		Set("end_at = ?", endAt).
		Set("revoked_at = ?", now).
		Set("revoke_reason = ?", reason).
		Set("updated_at = ?", now).
		Where("ent.source_type = ?", models.EntitlementSourceOneOff).
		Where("ent.source_id = ?", paymentID).
		Where("ent.end_at IS NULL").
		Exec(ctx)
	return err
}

func (r *EntitlementRepo) ExistsBySource(ctx context.Context, sourceType models.EntitlementSourceType, sourceID uuid.UUID, entitlement string) (bool, error) {
	return r.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		Where("ent.source_type = ?", sourceType).
		Where("ent.source_id = ?", sourceID).
		Where("ent.entitlement = ?", entitlement).
		Where("ent.revoked_at IS NULL").
		Exists(ctx)
}

func (r *EntitlementRepo) ListByUser(ctx context.Context, userID string) ([]models.Entitlement, error) {
	ents := []models.Entitlement{}
	if err := r.db.GetDB().NewSelect().
		Model(&ents).
		Where("ent.user_id = ?", userID).
		OrderExpr("ent.start_at DESC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return ents, nil
}
