package solana

import (
	"context"
	"errors"
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"github.com/doujins-org/doujins-billing/internal/manager/web/response"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"net/http"
	"time"

	"github.com/doujins-org/solana-go"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// CheckSolanaPayment checks if a Solana Pay transfer has been completed on-chain
func CheckSolanaPayment(r *request.Request) {
	req := new(request.CheckSolanaPaymentRequest)
	if !r.BindQuery(req.Query()) {
		return
	}

	if req.Reference == "" {
		r.ErrorJSON(http.StatusBadRequest, "Reference is required")
		return
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}
	user := userCtx.User

	ctx, cancel := context.WithTimeout(r.Request.Context(), 30*time.Second)
	defer cancel()

	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet"
	}
	rpcService := NewSolanaRPCService(rpcEndpoint, network)
	txService := NewSolanaTransactionService(r.State.DB, rpcService, r.State.Config, r.State.PriceService, r.State.PaymentService)

	intent, err := r.State.SolanaPaymentIntentService.GetByReference(ctx, req.Reference)
	if err != nil {
		log.WithError(err).Warn("No payment intent found for reference")
		r.SuccessJSON(response.CheckSolanaPaymentResponse{Status: "pending", IntentID: ""})
		return
	}

	if intent.UserID != user.ID {
		log.WithFields(log.Fields{"intent_id": intent.ID, "user_id": user.ID}).Warn("Attempt to access intent belonging to another user")
		r.ErrorJSON(http.StatusForbidden, "Payment intent does not belong to you")
		return
	}

	if intent.FlowType != FlowTypeSolanaPay {
		log.WithFields(log.Fields{"intent_id": intent.ID, "flow": intent.FlowType}).Warn("Reference does not correspond to Solana Pay intent")
		r.ErrorJSON(http.StatusBadRequest, "Reference is not a Solana Pay intent")
		return
	}

	if intent.Status == IntentStatusConfirmed {
		resp := response.CheckSolanaPaymentResponse{Status: "confirmed", IntentID: intent.ID.String()}
		if intent.TransactionSignature != nil {
			resp.Transaction = *intent.TransactionSignature
		}
		r.SuccessJSON(resp)
		return
	}

	if intent.ExpiresAt != nil && time.Now().After(*intent.ExpiresAt) {
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, "intent expired")
		r.SuccessJSON(response.CheckSolanaPaymentResponse{Status: "failed", IntentID: intent.ID.String(), ErrorMessage: "Intent expired"})
		return
	}

	if err = r.State.SolanaPaymentIntentService.MarkProcessing(ctx, intent.ID); err != nil && !errors.Is(err, ErrIntentInvalidState) {
		log.WithError(err).Warn("Failed to move intent to processing state")
	}

	refPubkey, err := solana.PublicKeyFromBase58(req.Reference)
	if err != nil {
		log.WithError(err).Error("Invalid reference format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid reference format")
		return
	}

	limit := 10
	signatures, err := rpcService.GetSignaturesForAddress(ctx, refPubkey, &limit)
	if err != nil {
		log.WithError(err).Error("Failed to check for transactions")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to check payment status")
		return
	}

	if len(signatures) == 0 {
		r.SuccessJSON(response.CheckSolanaPaymentResponse{Status: "pending", IntentID: intent.ID.String()})
		return
	}

	signature := signatures[0].Signature
	signatureStr := signature.String()

	txResult, err := txService.VerifyTransactionWithContent(ctx, signatureStr, intent.ExpectedAmountLamports, intent.RecipientWallet, intent.TokenMint, "", intent.Reference)
	if err != nil {
		log.WithError(err).Error("Transaction verification for Solana Pay intent failed")
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, err.Error())
		r.SuccessJSON(response.CheckSolanaPaymentResponse{Status: "failed", IntentID: intent.ID.String(), ErrorMessage: "Transaction verification failed"})
		return
	}

	if err = r.State.SolanaPaymentIntentService.MarkConfirmed(ctx, intent.ID, signatureStr); err != nil {
		log.WithError(err).Warn("Failed to mark Solana Pay intent confirmed")
	}

	pay, err := r.State.SolanaPaymentService.Submit(ctx, user.ID, intent.ID, intent.PriceID, signatureStr, user.Email)
	if err != nil {
		log.WithError(err).Error("Failed to record Solana Pay purchase")
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, err.Error())
		r.ErrorJSON(http.StatusInternalServerError, "Failed to record payment")
		return
	}

	txDetails := extractTransactionDetails(txResult, "", intent.Token, intent.TokenMint)

	userIDCopy := user.ID
	solanaTransaction := &models.SolanaTransaction{
		ID:          uuid.New(),
		UserID:      &userIDCopy,
		Signature:   &signatureStr,
		Status:      "confirmed",
		Amount:      pay.Amount,
		Token:       txDetails.Token,
		TokenMint:   txDetails.TokenMint,
		FromAddress: txDetails.FromAddress,
		ToAddress:   intent.RecipientWallet,
		IntentID:    &intent.ID,
		BlockTime: func() *time.Time {
			if txResult.BlockTime != nil {
				t := time.Unix(int64(*txResult.BlockTime), 0)
				return &t
			}
			return nil
		}(),
		Slot: func() *int64 {
			if txResult.Slot > 0 {
				s := int64(txResult.Slot)
				return &s
			}
			return nil
		}(),
	}

	if _, err := r.State.DB.GetDB().NewInsert().Model(solanaTransaction).Exec(ctx); err != nil {
		log.WithError(err).Error("Failed to create SolanaTransaction record for Solana Pay intent")
	}

	resp := response.CheckSolanaPaymentResponse{
		Status:      "confirmed",
		PaymentID:   pay.ID.String(),
		IntentID:    intent.ID.String(),
		Transaction: signatureStr,
	}
	r.SuccessJSON(resp)
}
