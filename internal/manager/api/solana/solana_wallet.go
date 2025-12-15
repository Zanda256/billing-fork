package solana

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	"github.com/doujins-org/doujins-billing/internal/utils/solana"
	"github.com/google/uuid"
)

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletAlreadyExists = errors.New("wallet already exists")
)

// SolanaWalletService provides DB-backed operations for user wallets
type SolanaWalletService struct{ repo *repo.SolanaWalletRepo }

func NewSolanaWalletService(db *db.DB) *SolanaWalletService {
	return &SolanaWalletService{repo: repo.NewSolanaWalletRepo(db)}
}

// Link adds a wallet for a user if not present; returns existing or newly created
func (s *SolanaWalletService) Link(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return nil, fmt.Errorf("address validation failed: %w", err)
	}

	existing, err := s.repo.GetByUserAndAddress(ctx, userID, address)
	if err == nil {
		return existing, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing wallet: %w", err)
	}

	now := time.Now()
	wallet := &models.SolanaWallet{
		ID:         uuid.New(),
		UserID:     userID,
		Address:    address,
		IsVerified: false,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.repo.Insert(ctx, wallet); err != nil {
		if errors.Is(err, repo.ErrNoRowsAffected) {
			return nil, ErrWalletAlreadyExists
		}
		return nil, fmt.Errorf("failed to insert wallet: %w", err)
	}

	return wallet, nil
}

// List returns wallets for a user
func (s *SolanaWalletService) List(ctx context.Context, userID string) ([]*models.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	wallets, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list wallets for user %s: %w", userID, err)
	}
	return wallets, nil
}

// Verify marks a wallet as verified
func (s *SolanaWalletService) Verify(ctx context.Context, userID, address string) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return fmt.Errorf("address validation failed: %w", err)
	}

	now := time.Now()
	rows, err := s.repo.MarkVerified(ctx, userID, address, now)
	if err != nil {
		return fmt.Errorf("failed to verify wallet %s for user %s: %w", address, userID, err)
	}
	if rows < 1 {
		return fmt.Errorf("%w: wallet %s for user %s", ErrWalletNotFound, address, userID)
	}
	return nil
}

// Delete removes a wallet for a user
func (s *SolanaWalletService) Delete(ctx context.Context, userID, address string) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return fmt.Errorf("address validation failed: %w", err)
	}

	rows, err := s.repo.Delete(ctx, userID, address)
	if err != nil {
		return fmt.Errorf("failed to delete wallet %s for user %s: %w", address, userID, err)
	}
	if rows < 1 {
		return fmt.Errorf("%w: wallet %s for user %s", ErrWalletNotFound, address, userID)
	}
	return nil
}

// Get returns the latest record for a wallet belonging to a user.
func (s *SolanaWalletService) Get(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return nil, fmt.Errorf("address validation failed: %w", err)
	}

	wallet, err := s.repo.GetLatest(ctx, userID, address)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: wallet %s for user %s", ErrWalletNotFound, address, userID)
		}
		return nil, fmt.Errorf("failed to get wallet %s for user %s: %w", address, userID, err)
	}
	return wallet, nil
}

// GetPrimary returns the most recently verified wallet for a user.
func (s *SolanaWalletService) GetPrimary(ctx context.Context, userID string) (*models.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}

	wallet, err := s.repo.GetPrimary(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: no wallet for user %s", ErrWalletNotFound, userID)
		}
		return nil, fmt.Errorf("failed to get primary wallet for user %s: %w", userID, err)
	}
	return wallet, nil
}
