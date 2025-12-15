package solana

import (
	"context"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/utils/solana"
)

type SolanaWalletVerifyRequest struct {
	Wallet    string `json:"wallet" validate:"required"`
	Signature string `json:"signature" validate:"required"`
	Message   string `json:"message" validate:"omitempty"`
}

type SolanaWalletChallengeRequest struct {
	Wallet string `json:"wallet" validate:"required"`
}

// GenerateSolanaWalletChallenge generates a verification challenge for a wallet
func GenerateSolanaWalletChallenge(r *request.Request) {
	req := new(SolanaWalletChallengeRequest)
	if !r.BindJSON(req) {
		return
	}

	if err := solana.ValidateAddress(req.Wallet); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid wallet address")
		return
	}

	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	// Ensure the wallet record exists so verification can complete later.
	if _, err := r.State.SolanaWalletService.Link(ctx, user.ID, req.Wallet); err != nil {
		log.WithError(err).Error("Failed to ensure Solana wallet record exists")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare wallet for verification")
		return
	}

	verificationService := NewSolanaVerificationService(r.State.DB, r.State.SolanaWalletService)
	challenge, err := verificationService.GenerateChallenge(ctx, user.ID, req.Wallet)
	if err != nil {
		log.WithError(err).Error("Failed to generate wallet verification challenge")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate challenge")
		return
	}

	r.SuccessJSON(map[string]any{
		"message":    challenge.Message,
		"expires_at": challenge.ExpiresAt.Unix(),
		"wallet":     req.Wallet,
		"nonce":      challenge.Nonce,
	})
}

// VerifySolanaWallet accepts a signature and marks wallet as verified
func VerifySolanaWallet(r *request.Request) {
	req := new(SolanaWalletVerifyRequest)
	if !r.BindJSON(req) {
		return
	}

	if err := solana.ValidateAddress(req.Wallet); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid wallet address")
		return
	}

	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 10*time.Second)
	defer cancel()

	verificationService := NewSolanaVerificationService(r.State.DB, r.State.SolanaWalletService)
	wallet, err := verificationService.VerifySignature(ctx, user.ID, req.Wallet, req.Signature)
	if err != nil {
		log.WithError(err).Error("Failed to verify wallet signature")
		r.ErrorJSON(http.StatusBadRequest, "Signature verification failed")
		return
	}

	r.SuccessJSON(map[string]any{
		"verified":    true,
		"wallet":      wallet.Address,
		"verified_at": wallet.VerifiedAt,
	})
}

// ListSolanaWallets lists the user's connected wallets from database
func ListSolanaWallets(r *request.Request) {
	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	wallets, err := r.State.SolanaWalletService.List(ctx, user.ID)
	if err != nil {
		log.WithError(err).Error("Failed to list Solana wallets")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to list wallets")
		return
	}

	r.SuccessJSON(map[string]any{"wallets": wallets})
}

// DeleteSolanaWallet removes a wallet from database
func DeleteSolanaWallet(r *request.Request) {
	user := r.GetUser()
	wallet := r.Query("wallet")

	if wallet == "" {
		r.ErrorJSON(http.StatusBadRequest, "wallet parameter required")
		return
	}

	if err := solana.ValidateAddress(wallet); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid wallet address")
		return
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	if err := r.State.SolanaWalletService.Delete(ctx, user.ID, wallet); err != nil {
		log.WithError(err).Error("Failed to delete Solana wallet")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to delete wallet")
		return
	}

	r.SuccessJSON(map[string]any{
		"deleted": true,
		"wallet":  wallet,
	})
}

// GetSolanaWallet returns the most recently linked wallet for the authenticated user.
func GetSolanaWallet(r *request.Request) {
	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	wallet, err := r.State.SolanaWalletService.GetPrimary(ctx, user.ID)
	if err != nil {
		log.WithError(err).Error("Failed to get Solana wallet")
		r.ErrorJSON(http.StatusNotFound, "No Solana wallet linked")
		return
	}

	r.SuccessJSON(map[string]any{
		"wallet": wallet,
	})
}
