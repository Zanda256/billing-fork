package solana

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
)

const (
	FlowTypeDirect    = "direct"
	FlowTypeSolanaPay = "solanapay"

	IntentStatusPending    = "pending"
	IntentStatusProcessing = "processing"
	IntentStatusConfirmed  = "confirmed"
	IntentStatusFailed     = "failed"
	IntentStatusExpired    = "expired"
)

var (
	ErrIntentNotFound     = errors.New("solana payment intent not found")
	ErrIntentInvalidState = errors.New("solana payment intent has invalid state for this operation")
)

// SolanaPaymentIntentService manages persisted payment intents shared by direct and Solana Pay flows.
type SolanaPaymentIntentService struct {
	repo         *repo.SolanaPaymentIntentRepo
	cfg          *config.Config
	priceService *price.PriceService
}

func NewSolanaPaymentIntentService(db *db.DB, cfg *config.Config, priceService *price.PriceService) *SolanaPaymentIntentService {
	return &SolanaPaymentIntentService{repo: repo.NewSolanaPaymentIntentRepo(db), cfg: cfg, priceService: priceService}
}

// CreateDirectIntent creates an intent for a direct wallet transaction using a verified payer wallet.
func (s *SolanaPaymentIntentService) CreateDirectIntent(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, payerWallet string) (*models.SolanaPaymentIntent, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if s.cfg.Solana == nil {
		return nil, fmt.Errorf("solana configuration not available")
	}

	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load price: %w", err)
	}

	tokenCfg, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		return nil, fmt.Errorf("token %s not supported", tokenSymbol)
	}

	merchantWallet := s.cfg.Solana.RecipientWallet
	if merchantWallet == "" {
		return nil, fmt.Errorf("solana recipient wallet not configured")
	}

	tokenAmountUnits, tokenAmountDecimal, err := calculateTokenQuote(ctx, tokenCfg, price.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate token amount: %w", err)
	}

	expiresAt := time.Now().Add(10 * time.Minute)
	intent := &models.SolanaPaymentIntent{
		ID:                     uuid.New(),
		UserID:                 userID,
		PriceID:                price.ID,
		FlowType:               FlowTypeDirect,
		Token:                  tokenSymbol,
		TokenMint:              tokenCfg.Mint,
		Amount:                 tokenAmountDecimal,
		Currency:               tokenSymbol,
		ExpectedAmountLamports: tokenAmountUnits,
		RecipientWallet:        merchantWallet,
		Status:                 IntentStatusPending,
		ExpiresAt:              &expiresAt,
	}

	if payerWallet != "" {
		intent.PayerWallet = &payerWallet
	}

	if err := s.insertIntent(ctx, intent); err != nil {
		return nil, err
	}

	return intent, nil
}

// CreateSolanaPayIntent creates an intent for a Solana Pay transaction and returns memo/reference metadata.
func (s *SolanaPaymentIntentService) CreateSolanaPayIntent(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol string) (*models.SolanaPaymentIntent, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if s.cfg.Solana == nil {
		return nil, fmt.Errorf("solana configuration not available")
	}

	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load price: %w", err)
	}

	tokenCfg, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		return nil, fmt.Errorf("token %s not supported", tokenSymbol)
	}

	merchantWallet := s.cfg.Solana.RecipientWallet

	if merchantWallet == "" {
		return nil, fmt.Errorf("solana recipient wallet not configured")
	}

	tokenAmountUnits, tokenAmountDecimal, err := calculateTokenQuote(ctx, tokenCfg, price.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate token amount: %w", err)
	}

	reference, err := generateReference()
	if err != nil {
		return nil, fmt.Errorf("failed to generate reference: %w", err)
	}

	memo := fmt.Sprintf("purchase:%s:%s", userID, price.ID.String())
	expiresAt := time.Now().Add(15 * time.Minute)
	intent := &models.SolanaPaymentIntent{
		ID:                     uuid.New(),
		UserID:                 userID,
		PriceID:                price.ID,
		FlowType:               FlowTypeSolanaPay,
		Token:                  tokenSymbol,
		TokenMint:              tokenCfg.Mint,
		Amount:                 tokenAmountDecimal,
		Currency:               tokenSymbol,
		ExpectedAmountLamports: tokenAmountUnits,
		RecipientWallet:        merchantWallet,
		Reference:              &reference,
		Memo:                   &memo,
		Status:                 IntentStatusPending,
		ExpiresAt:              &expiresAt,
	}

	if err := s.insertIntent(ctx, intent); err != nil {
		return nil, err
	}

	return intent, nil
}

// GetByID fetches an intent by ID.
func (s *SolanaPaymentIntentService) GetByID(ctx context.Context, intentID uuid.UUID) (*models.SolanaPaymentIntent, error) {
	intent, err := s.repo.GetByID(ctx, intentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIntentNotFound, err)
	}
	return intent, nil
}

// GetByReference fetches an intent by reference value.
func (s *SolanaPaymentIntentService) GetByReference(ctx context.Context, reference string) (*models.SolanaPaymentIntent, error) {
	intent, err := s.repo.GetByReference(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIntentNotFound, err)
	}
	return intent, nil
}

// MarkProcessing marks an intent as processing to avoid races.
func (s *SolanaPaymentIntentService) MarkProcessing(ctx context.Context, intentID uuid.UUID) error {
	rows, err := s.repo.MarkProcessing(ctx, intentID, IntentStatusProcessing, IntentStatusPending)
	if err != nil {
		return fmt.Errorf("failed to mark intent processing: %w", err)
	}
	if rows == 0 {
		return ErrIntentInvalidState
	}
	return nil
}

// MarkConfirmed marks an intent as confirmed and stores signature metadata.
func (s *SolanaPaymentIntentService) MarkConfirmed(ctx context.Context, intentID uuid.UUID, signature string) error {
	rows, err := s.repo.MarkConfirmed(ctx, intentID, IntentStatusConfirmed, []string{IntentStatusPending, IntentStatusProcessing}, signature)
	if err != nil {
		return fmt.Errorf("failed to confirm intent: %w", err)
	}
	if rows == 0 {
		return ErrIntentInvalidState
	}
	return nil
}

// MarkFailed records an error for the intent.
func (s *SolanaPaymentIntentService) MarkFailed(ctx context.Context, intentID uuid.UUID, message string) error {
	rows, err := s.repo.MarkFailed(ctx, intentID, IntentStatusFailed, message)
	if err != nil {
		return fmt.Errorf("failed to mark intent failed: %w", err)
	}
	if rows == 0 {
		return ErrIntentInvalidState
	}
	return nil
}

func (s *SolanaPaymentIntentService) insertIntent(ctx context.Context, intent *models.SolanaPaymentIntent) error {
	intent.CreatedAt = time.Now()
	intent.UpdatedAt = intent.CreatedAt
	if err := s.repo.Insert(ctx, intent); err != nil {
		return fmt.Errorf("failed to create solana payment intent: %w", err)
	}
	log.WithFields(log.Fields{
		"intent_id": intent.ID,
		"flow":      intent.FlowType,
		"user_id":   intent.UserID,
		"price_id":  intent.PriceID,
	}).Info("Created Solana payment intent")
	return nil
}

func generateReference() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base58.Encode(buf), nil
}
