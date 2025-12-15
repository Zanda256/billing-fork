package repo

import (
	"context"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"
)

type SolanaChallengeRepo struct {
	db *db.DB
}

func NewSolanaChallengeRepo(d *db.DB) *SolanaChallengeRepo { return &SolanaChallengeRepo{db: d} }

func (r *SolanaChallengeRepo) Upsert(ctx context.Context, challenge *models.SolanaWalletChallenge) error {
	_, err := r.db.GetDB().NewInsert().Model(challenge).
		Column("user_id", "address", "message", "nonce", "expires_at", "updated_at").
		On("CONFLICT (user_id, address) DO UPDATE").
		Set("message = EXCLUDED.message").
		Set("nonce = EXCLUDED.nonce").
		Set("expires_at = EXCLUDED.expires_at").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *SolanaChallengeRepo) Get(ctx context.Context, userID, address string) (*models.SolanaWalletChallenge, error) {
	challenge := new(models.SolanaWalletChallenge)
	if err := r.db.GetDB().NewSelect().
		Model(challenge).
		Where("swc.user_id = ? AND swc.address = ?", userID, address).
		Scan(ctx); err != nil {
		return nil, err
	}
	return challenge, nil
}

func (r *SolanaChallengeRepo) Delete(ctx context.Context, userID, address string) error {
	_, err := r.db.GetDB().NewDelete().
		Model((*models.SolanaWalletChallenge)(nil)).
		Where("swc.user_id = ? AND swc.address = ?", userID, address).
		Exec(ctx)
	return err
}

func (r *SolanaChallengeRepo) DeleteExpired(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.GetDB().NewDelete().
		Model((*models.SolanaWalletChallenge)(nil)).
		Where("swc.expires_at < ?", cutoff).
		Exec(ctx)
	return err
}
