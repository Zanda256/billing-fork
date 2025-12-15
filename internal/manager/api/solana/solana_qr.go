package solana

import (
	"context"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"github.com/doujins-org/doujins-billing/internal/manager/web/response"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// GenerateSolanaPayQR generates a Solana Pay URL for wallet apps to scan
func GenerateSolanaPayQR(r *request.Request) {
	req := new(request.GenerateSolanaPayQRRequest)
	if !r.BindJSON(req) {
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": req.PriceID, "error": err.Error()}).Error("Invalid price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 10*time.Second)
	defer cancel()

	price, err := r.State.PriceService.GetByID(ctx, priceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": priceID, "error": err.Error()}).Error("Failed to get price information")
		r.ErrorJSON(http.StatusNotFound, "Price not found")
		return
	}

	solCfg := r.State.Config.Solana
	if solCfg == nil {
		log.Error("Solana configuration not found")
		r.ErrorJSON(http.StatusInternalServerError, "Payment system configuration error")
		return
	}

	tokenSymbol := req.Token
	tokenCfg, ok := solCfg.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		r.ErrorJSON(http.StatusBadRequest, "Invalid or unsupported token")
		return
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}
	user := userCtx.User

	amount, _, tokenAmountUnits, exp, err := r.State.SolanaPaymentService.Generate(ctx, user.ID, price.ID, tokenSymbol, req.UserWallet)
	if err != nil {
		log.WithError(err).Error("Failed to create Solana Pay intent")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	intent, err := r.State.SolanaPaymentIntentService.CreateSolanaPayIntent(ctx, user.ID, price.ID, tokenSymbol)
	if err != nil {
		log.WithError(err).Error("Failed to create Solana Pay intent")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	if intent.Reference == nil {
		log.WithField("intent_id", intent.ID).Error("Solana Pay intent missing reference")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	merchantWallet := intent.RecipientWallet
	params := url.Values{}
	params.Set("amount", formatTokenAmount(tokenAmountUnits, tokenCfg.Decimals))
	if tokenCfg.Mint != "" && strings.ToUpper(tokenSymbol) != "SOL" {
		params.Set("spl-token", tokenCfg.Mint)
	}
	params.Add("reference", *intent.Reference)
	params.Set("label", "Doujins Purchase")
	if intent.Memo != nil {
		params.Set("memo", *intent.Memo)
	}
	solanaURL := fmt.Sprintf("solana:%s", merchantWallet)
	if encoded := params.Encode(); encoded != "" {
		solanaURL = fmt.Sprintf("%s?%s", solanaURL, encoded)
	}

	resp := response.SolanaPayQRResponse{
		URL:         solanaURL,
		Amount:      amount,
		TokenAmount: formatTokenAmount(tokenAmountUnits, tokenCfg.Decimals),
		TokenSymbol: tokenSymbol,
		Label:       "Doujins Purchase",
		Message: func() string {
			if intent.Memo != nil {
				return *intent.Memo
			}
			return ""
		}(),
		ExpiresAt: exp.Unix(),
		Reference: *intent.Reference,
		IntentID:  intent.ID.String(),
	}

	log.WithFields(log.Fields{
		"price_id":        priceID,
		"amount":          price.Amount,
		"token_symbol":    tokenSymbol,
		"merchant_wallet": merchantWallet,
		"reference":       *intent.Reference,
	}).Info("Generated Solana Pay QR code URL")

	r.SuccessJSON(resp)
}

func formatTokenAmount(units uint64, decimals int) string {
	if decimals <= 0 {
		return fmt.Sprintf("%d", units)
	}
	numerator := new(big.Int).SetUint64(units)
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return new(big.Rat).SetFrac(numerator, denominator).FloatString(decimals)
}
