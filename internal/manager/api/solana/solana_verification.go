package solana

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	solanago "github.com/doujins-org/solana-go"
	"github.com/mr-tron/base58"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/utils/solana"
)

const defaultChallengeTTL = 10 * time.Minute

// SolanaVerificationService persists wallet verification challenges and validates signatures.
type SolanaVerificationService struct {
	challenges   *repo.SolanaChallengeRepo
	wallets      *SolanaWalletService
	challengeTTL time.Duration
}

// VerificationChallenge represents a generated challenge message
// returned to the client for signing.
type VerificationChallenge struct {
	UserID    string
	Address   string
	Message   string
	ExpiresAt time.Time
	Nonce     string
}

// NewSolanaVerificationService creates a verification service backed by the database.
func NewSolanaVerificationService(db *db.DB, wallets *SolanaWalletService) *SolanaVerificationService {
	ttl := defaultChallengeTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	return &SolanaVerificationService{
		challenges:   repo.NewSolanaChallengeRepo(db),
		wallets:      wallets,
		challengeTTL: ttl,
	}
}

// GenerateChallenge persists a new verification challenge for the given wallet.
func (s *SolanaVerificationService) GenerateChallenge(ctx context.Context, userID, address string) (*VerificationChallenge, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return nil, fmt.Errorf("address validation failed: %w", err)
	}

	pubKey, err := solanago.PublicKeyFromBase58(address)
	if err != nil {
		return nil, fmt.Errorf("invalid solana address: %w", err)
	}

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	now := time.Now().UTC()
	expiresAt := now.Add(s.challengeTTL)

	message := fmt.Sprintf(
		"Verify wallet ownership for Doujins billing system.\nWallet: %s\nUser: %s\nNonce: %s\nTimestamp: %d",
		pubKey.String(),
		userID,
		nonce,
		now.Unix(),
	)

	challenge := &models2.SolanaWalletChallenge{
		UserID:    userID,
		Address:   pubKey.String(),
		Message:   message,
		Nonce:     nonce,
		ExpiresAt: expiresAt,
		UpdatedAt: now,
	}

	if err := s.challenges.Upsert(ctx, challenge); err != nil {
		return nil, fmt.Errorf("failed to persist verification challenge: %w", err)
	}

	log.WithFields(log.Fields{
		"user_id":    userID,
		"address":    pubKey.String(),
		"expires_at": expiresAt,
	}).Info("Generated Solana wallet verification challenge")

	return &VerificationChallenge{
		UserID:    userID,
		Address:   pubKey.String(),
		Message:   message,
		ExpiresAt: expiresAt,
		Nonce:     nonce,
	}, nil
}

// VerifySignature validates the signature against the stored challenge and marks the wallet as verified.
func (s *SolanaVerificationService) VerifySignature(ctx context.Context, userID, address, signature string) (*models2.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return nil, fmt.Errorf("address validation failed: %w", err)
	}
	if err := solana.ValidateSignature(signature); err != nil {
		return nil, fmt.Errorf("signature validation failed: %w", err)
	}

	challenge, err := s.challenges.Get(ctx, userID, address)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("no active challenge found for wallet %s: %w", address, err)
	}

	if time.Now().After(challenge.ExpiresAt) {
		_ = s.challenges.Delete(ctx, challenge.UserID, challenge.Address)
		return nil, fmt.Errorf("challenge expired")
	}

	pubKey, err := solanago.PublicKeyFromBase58(challenge.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid challenge address: %w", err)
	}

	sigBytes, err := base58.Decode(signature)
	if err != nil {
		return nil, fmt.Errorf("failed to decode signature: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid signature length: expected %d bytes got %d", ed25519.SignatureSize, len(sigBytes))
	}

	if !ed25519.Verify(pubKey[:], []byte(challenge.Message), sigBytes) {
		return nil, fmt.Errorf("signature verification failed")
	}

	if _, err := s.wallets.Link(ctx, userID, challenge.Address); err != nil {
		return nil, fmt.Errorf("failed to link wallet prior to verification: %w", err)
	}

	if err := s.wallets.Verify(ctx, userID, challenge.Address); err != nil {
		return nil, fmt.Errorf("failed to mark wallet verified: %w", err)
	}

	if err := s.challenges.Delete(ctx, challenge.UserID, challenge.Address); err != nil {
		log.WithError(err).Warn("Failed to delete Solana wallet challenge after verification")
	}

	wallet, err := s.wallets.Get(ctx, userID, challenge.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to load verified wallet: %w", err)
	}

	log.WithFields(log.Fields{
		"user_id": userID,
		"address": wallet.Address,
	}).Info("Successfully verified Solana wallet signature")

	return wallet, nil
}
