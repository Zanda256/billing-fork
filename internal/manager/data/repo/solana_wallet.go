package repo

import (
	"context"
	"fmt"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"
)

type SolanaWalletRepo struct {
	db *db.DB
}

func NewSolanaWalletRepo(d *db.DB) *SolanaWalletRepo { return &SolanaWalletRepo{db: d} }

func (r *SolanaWalletRepo) GetByUserAndAddress(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	wallet := new(models.SolanaWallet)
	err := r.db.GetDB().NewSelect().
		Model(wallet).
		Where("sw.user_id = ? AND sw.address = ?", userID, address).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

func (r *SolanaWalletRepo) Insert(ctx context.Context, wallet *models.SolanaWallet) error {
	res, err := r.db.GetDB().NewInsert().Model(wallet).Exec(ctx)
	if err != nil {
		return err
	}
	if rows, err := res.RowsAffected(); err != nil {
		return err
	} else if rows < 1 {
		return ErrNoRowsAffected
	}
	return nil
}

func (r *SolanaWalletRepo) ListByUser(ctx context.Context, userID string) ([]*models.SolanaWallet, error) {
	wallets := []*models.SolanaWallet{}
	if err := r.db.GetDB().NewSelect().Model(&wallets).
		Where("sw.user_id = ?", userID).
		OrderExpr("sw.created_at DESC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return wallets, nil
}

func (r *SolanaWalletRepo) MarkVerified(ctx context.Context, userID, address string, verifiedAt time.Time) (int64, error) {
	res, err := r.db.GetDB().NewUpdate().
		Model((*models.SolanaWallet)(nil)).
		Set("is_verified = ?", true).
		Set("verified_at = ?", &verifiedAt).
		Set("updated_at = ?", verifiedAt).
		Where("sw.user_id = ? AND sw.address = ?", userID, address).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *SolanaWalletRepo) Delete(ctx context.Context, userID, address string) (int64, error) {
	res, err := r.db.GetDB().NewDelete().
		Model((*models.SolanaWallet)(nil)).
		Where("sw.user_id = ? AND sw.address = ?", userID, address).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *SolanaWalletRepo) GetLatest(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	wallet := new(models.SolanaWallet)
	err := r.db.GetDB().NewSelect().
		Model(wallet).
		Where("sw.user_id = ? AND sw.address = ?", userID, address).
		OrderExpr("sw.updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

func (r *SolanaWalletRepo) GetPrimary(ctx context.Context, userID string) (*models.SolanaWallet, error) {
	wallet := new(models.SolanaWallet)
	err := r.db.GetDB().NewSelect().
		Model(wallet).
		Where("sw.user_id = ?", userID).
		OrderExpr("sw.is_verified DESC").
		OrderExpr("sw.updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

var ErrNoRowsAffected = fmt.Errorf("no rows affected")
