package repo

import (
	"context"
	"errors"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrCCBillAliasConflict = errors.New("ccbill alias conflict")

type CCBillUsernameAliasRepo struct {
	db *db.DB
}

func NewCCBillUsernameAliasRepo(d *db.DB) *CCBillUsernameAliasRepo {
	return &CCBillUsernameAliasRepo{db: d}
}

func (r *CCBillUsernameAliasRepo) GetByUserID(ctx context.Context, userID string) (*models.CCBillUsernameAlias, error) {
	alias := new(models.CCBillUsernameAlias)
	if err := r.db.GetDB().NewSelect().Model(alias).Where("ccbill_alias.user_id = ?", userID).Scan(ctx); err != nil {
		return nil, err
	}
	return alias, nil
}

func (r *CCBillUsernameAliasRepo) GetByAlias(ctx context.Context, aliasValue string) (*models.CCBillUsernameAlias, error) {
	alias := new(models.CCBillUsernameAlias)
	if err := r.db.GetDB().NewSelect().Model(alias).Where("ccbill_alias.alias = ?", aliasValue).Scan(ctx); err != nil {
		return nil, err
	}
	return alias, nil
}

func (r *CCBillUsernameAliasRepo) Create(ctx context.Context, alias *models.CCBillUsernameAlias) error {
	now := time.Now().UTC()
	if alias.CreatedAt.IsZero() {
		alias.CreatedAt = now
	}
	alias.UpdatedAt = now

	if _, err := r.db.GetDB().NewInsert().Model(alias).Exec(ctx); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return ErrCCBillAliasConflict
			}
		}
		return err
	}
	return nil
}

func (r *CCBillUsernameAliasRepo) Touch(ctx context.Context, aliasValue string) error {
	_, err := r.db.GetDB().NewUpdate().Model((*models.CCBillUsernameAlias)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("alias = ?", aliasValue).
		Exec(ctx)
	return err
}
