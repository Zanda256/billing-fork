package solana

import (
	"context"
	"errors"
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	response2 "github.com/doujins-org/doujins-billing/internal/manager/web/response"
	"net/http"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// GeneratePayment handles generating Solana payment transactions (scaffold)
// Note: This implementation validates inputs and returns a structured response,
// but does not create a real Solana transaction to avoid external dependencies.
func GeneratePayment(r *request.Request) {
	req := new(request.GeneratePaymentRequest)
	if !r.BindJSON(req) {
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": req.PriceID, "error": err.Error()}).Error("Invalid price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 15*time.Second)
	defer cancel()

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}
	user := userCtx.User

	wallet, err := r.State.SolanaWalletService.Get(ctx, user.ID, req.UserWallet)
	if err != nil {
		if errors.Is(err, ErrWalletNotFound) {
			log.WithFields(log.Fields{"user_id": user.ID, "wallet": req.UserWallet}).Warn("Wallet not linked to user")
			r.ErrorJSON(http.StatusBadRequest, "Wallet is not linked to this account")
			return
		}
		log.WithError(err).Error("Failed to load Solana wallet for transaction generation")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to load wallet")
		return
	}
	if !wallet.IsVerified {
		log.WithFields(log.Fields{"user_id": user.ID, "wallet": wallet.Address}).Warn("Attempted to generate transaction with unverified wallet")
		r.ErrorJSON(http.StatusBadRequest, "Wallet must be verified before generating a transaction")
		return
	}

	amount, currency, tokenAmount, expiresAt, err := r.State.SolanaPaymentService.Generate(ctx, user.ID, priceID, req.Token, wallet.Address)
	if err != nil {
		log.WithError(err).Error("Failed to prepare Solana payment quote")
		r.ErrorJSON(http.StatusBadRequest, "Unable to generate payment for this price")
		return
	}

	intent, err := r.State.SolanaPaymentIntentService.CreateDirectIntent(ctx, user.ID, priceID, req.Token, wallet.Address)
	if err != nil {
		log.WithError(err).Error("Failed to create Solana payment intent")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet"
	}

	rpcService := NewSolanaRPCService(rpcEndpoint, network)
	txService := NewSolanaTransactionService(r.State.DB, rpcService, r.State.Config, r.State.PriceService, r.State.PaymentService)

	txResp, err := txService.BuildPaymentTransaction(ctx, user.ID, priceID, req.Token, wallet.Address)
	if err != nil {
		log.WithError(err).Error("Failed to build solana payment transaction")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to build payment transaction")
		return
	}

	// if err := txService.SimulateTransaction(ctx, txResp.Transaction); err != nil {
	// 	log.WithError(err).Error("Transaction simulation failed")
	// 	r.ErrorJSON(http.StatusBadRequest, "Transaction simulation failed - check wallet balance and token accounts")
	// 	return
	// }

	response := response2.GeneratePaymentResponse{
		Transaction:  txResp.TransactionBase64,
		Amount:       amount,
		Currency:     currency,
		TokenAmount:  tokenAmount,
		TokenSymbol:  txResp.TokenSymbol,
		ExpiresAt:    expiresAt.Unix(),
		Instructions: txResp.Instructions,
		IntentID:     intent.ID.String(),
	}

	r.SuccessJSON(response)
}
