package repo

import (
	"context"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
)

type SolanaTransactionRepo struct {
	db *db.DB
}

func NewSolanaTransactionRepo(d *db.DB) *SolanaTransactionRepo { return &SolanaTransactionRepo{db: d} }

func (r *SolanaTransactionRepo) MarkConfirmedByUserAndAmount(ctx context.Context, userID string, amount float64, signature string) error {
	_, err := r.db.GetDB().NewUpdate().
		Model((*models.SolanaTransaction)(nil)).
		Set("status = ?", "confirmed").
		Set("signature = ?", signature).
		Where("stx.user_id = ?", userID).
		Where("stx.amount = ?", amount).
		Exec(ctx)

	return err
}
