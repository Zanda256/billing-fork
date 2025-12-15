package solana

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"github.com/doujins-org/doujins-billing/internal/manager/web/response"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	solanago "github.com/doujins-org/solana-go"
	"github.com/doujins-org/solana-go/programs/memo"
	"github.com/doujins-org/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	log "github.com/sirupsen/logrus"

	solanaUtils "github.com/doujins-org/doujins-billing/internal/utils/solana"
)

// SubmitPayment accepts a signed transaction and processes payment with real on-chain verification
func SubmitPayment(r *request.Request) {
	req := new(request.SubmitPaymentRequest)
	if !r.BindJSON(req) {
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	intentID, err := uuid.Parse(req.IntentID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid intent ID format")
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

	intent, err := r.State.SolanaPaymentIntentService.GetByID(ctx, intentID)
	if err != nil {
		log.WithError(err).Error("Failed to load payment intent")
		r.ErrorJSON(http.StatusBadRequest, "Invalid payment intent")
		return
	}
	if intent.UserID != user.ID {
		log.WithFields(log.Fields{"intent_id": intent.ID, "user_id": user.ID}).Warn("Intent does not belong to user")
		r.ErrorJSON(http.StatusForbidden, "Payment intent does not belong to you")
		return
	}
	if intent.PriceID != priceID {
		log.WithFields(log.Fields{"intent_id": intent.ID, "expected_price": intent.PriceID, "provided_price": priceID}).Warn("Price mismatch for intent")
		r.ErrorJSON(http.StatusBadRequest, "Price does not match payment intent")
		return
	}
	if intent.FlowType != FlowTypeDirect {
		log.WithFields(log.Fields{"intent_id": intent.ID, "flow": intent.FlowType}).Warn("Submit called for non-direct intent")
		r.ErrorJSON(http.StatusBadRequest, "Payment intent is not for direct wallet flow")
		return
	}
	if intent.Status != IntentStatusPending && intent.Status != IntentStatusProcessing {
		log.WithFields(log.Fields{"intent_id": intent.ID, "status": intent.Status}).Warn("Intent already processed")
		r.ErrorJSON(http.StatusBadRequest, "Payment intent already processed")
		return
	}
	if intent.ExpiresAt != nil && time.Now().After(*intent.ExpiresAt) {
		log.WithFields(log.Fields{"intent_id": intent.ID}).Warn("Intent expired")
		r.ErrorJSON(http.StatusBadRequest, "Payment intent expired")
		return
	}

	if err = r.State.SolanaPaymentIntentService.MarkProcessing(ctx, intent.ID); err != nil && !errors.Is(err, ErrIntentInvalidState) {
		log.WithError(err).Warn("Failed to transition intent to processing state")
	}

	if intent.PayerWallet == nil || *intent.PayerWallet == "" {
		log.WithFields(log.Fields{"intent_id": intent.ID}).Error("Direct intent missing payer wallet")
		r.ErrorJSON(http.StatusInternalServerError, "Payment intent misconfigured")
		return
	}

	userWallet, err := r.State.SolanaWalletService.Get(ctx, user.ID, *intent.PayerWallet)
	if err != nil {
		log.WithError(err).Error("Failed to load payer wallet for intent")
		r.ErrorJSON(http.StatusBadRequest, "Linked wallet not available")
		return
	}
	if !userWallet.IsVerified {
		log.WithFields(log.Fields{"user_id": user.ID, "wallet": userWallet.Address}).Warn("Submit payment attempted with unverified wallet")
		r.ErrorJSON(http.StatusBadRequest, "Wallet must be verified before submitting a payment")
		return
	}

	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet" // fallback
	}
	rpcService := NewSolanaRPCService(rpcEndpoint, network)
	txService := NewSolanaTransactionService(r.State.DB, rpcService, r.State.Config, r.State.PriceService, r.State.PaymentService)

	signature := req.SignedTransaction
	var decodedTx *solanago.Transaction
	if txBytes, decodeErr := base64.StdEncoding.DecodeString(req.SignedTransaction); decodeErr == nil {
		if parsedTx, parseErr := solanago.TransactionFromBytes(txBytes); parseErr == nil {
			decodedTx = parsedTx
		} else {
			log.WithError(parseErr).Warn("Provided transaction payload could not be parsed; falling back to signature verification")
		}
	}

	if decodedTx != nil {
		if len(decodedTx.Message.AccountKeys) == 0 {
			r.ErrorJSON(http.StatusBadRequest, "Signed transaction missing fee payer")
			return
		}
		feePayer := decodedTx.Message.AccountKeys[0].String()
		if feePayer != userWallet.Address {
			log.WithFields(log.Fields{
				"user_id":           user.ID,
				"linked_wallet":     userWallet.Address,
				"transaction_payer": feePayer,
			}).Warn("Signed transaction does not originate from linked wallet")
			r.ErrorJSON(http.StatusBadRequest, "Signed transaction must use your linked wallet as fee payer")
			return
		}

		if len(decodedTx.Signatures) == 0 {
			r.ErrorJSON(http.StatusBadRequest, "Signed transaction does not contain a signature")
			return
		}

		if sentSig, sendErr := rpcService.SendTransaction(ctx, decodedTx); sendErr != nil {
			log.WithError(sendErr).Warn("Failed to broadcast signed transaction; continuing with local signature")
			signature = decodedTx.Signatures[0].String()
		} else {
			signature = sentSig.String()
		}
	}

	if err = solanaUtils.ValidateSignature(signature); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid transaction signature format")
		return
	}

	txResult, err := txService.VerifyTransactionWithContent(ctx, signature, intent.ExpectedAmountLamports, intent.RecipientWallet, intent.TokenMint, userWallet.Address, intent.Reference)
	if err != nil {
		log.WithError(err).Error("Failed to verify transaction on-chain")
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, err.Error())
		r.ErrorJSON(http.StatusBadRequest, fmt.Sprintf("Transaction verification failed: %v", err))
		return
	}

	if err = r.State.SolanaPaymentIntentService.MarkConfirmed(ctx, intent.ID, signature); err != nil {
		log.WithError(err).Warn("Failed to mark payment intent confirmed")
	}

	pay, err := r.State.SolanaPaymentService.Submit(ctx, user.ID, intent.ID, priceID, signature, user.Email)
	if err != nil {
		log.WithError(err).Error("Failed to process payment and grant entitlements")
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, err.Error())
		r.ErrorJSON(http.StatusInternalServerError, "Failed to process payment")
		return
	}

	txDetails := extractTransactionDetails(txResult, userWallet.Address, intent.Token, intent.TokenMint)

	if req.Memo != "" {
		if err := validateQRReference(req.Memo, txDetails.References, user.ID, priceID); err != nil {
			log.WithFields(log.Fields{
				"memo":       req.Memo,
				"references": txDetails.References,
				"error":      err.Error(),
			}).Warn("QR reference validation failed")
		}
	}

	userIDCopy := user.ID
	solanaTransaction := &models.SolanaTransaction{
		ID:          uuid.New(),
		UserID:      &userIDCopy,
		Signature:   &signature,
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
		log.WithError(err).Error("Failed to create SolanaTransaction record")

	}

	resp := response.SubmitPaymentResponse{
		PurchaseID:    pay.ID.String(),
		TransactionID: pay.TransactionID,
		Status:        "confirmed",
		Amount:        pay.Amount,
		Currency:      pay.Currency,
		ProcessedAt:   time.Now(),
		Message:       "Payment verified on-chain and processed successfully.",
		IntentID:      intent.ID.String(),
	}

	log.WithFields(log.Fields{
		"payment_id":     pay.ID,
		"user_id":        user.ID,
		"transaction_id": signature,
		"amount":         pay.Amount,
		"slot":           txResult.Slot,
	}).Info("Solana payment successfully processed")

	r.SuccessJSON(resp)
}

// TransactionDetails holds extracted transaction information
type TransactionDetails struct {
	Token       string
	TokenMint   string
	FromAddress string
	References  []string
}

// extractTransactionDetails extracts basic details from the confirmed transaction for record keeping.
func extractTransactionDetails(txResult *rpc.GetTransactionResult, defaultFrom string, tokenSymbol string, tokenMint string) TransactionDetails {
	details := TransactionDetails{Token: tokenSymbol, TokenMint: tokenMint, FromAddress: defaultFrom}

	if txResult != nil && txResult.Transaction != nil {
		if tx, err := txResult.Transaction.GetTransaction(); err == nil && tx != nil {
			if len(tx.Message.AccountKeys) > 0 {
				details.FromAddress = tx.Message.AccountKeys[0].String()
			}
			details.References = gatherTransactionReferences(tx, txResult.Meta)
		}
	}

	return details
}

func gatherTransactionReferences(tx *solanago.Transaction, meta *rpc.TransactionMeta) []string {
	if tx == nil {
		return nil
	}
	referenceSet := make(map[string]struct{})
	appendRef := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		referenceSet[value] = struct{}{}
	}

	for _, key := range tx.Message.AccountKeys {
		appendRef(key.String())
	}
	if meta != nil {
		for _, key := range meta.LoadedAddresses.ReadOnly {
			appendRef(key.String())
		}
		for _, key := range meta.LoadedAddresses.Writable {
			appendRef(key.String())
		}
	}

	for _, inst := range tx.Message.Instructions {
		programID, err := tx.ResolveProgramIDIndex(inst.ProgramIDIndex)
		if err != nil {
			continue
		}
		if programID.Equals(memo.ProgramID) {
			if memoText := strings.TrimSpace(string(inst.Data)); memoText != "" {
				appendRef(memoText)
			}
		}
	}

	if len(referenceSet) == 0 {
		return nil
	}
	refs := make([]string, 0, len(referenceSet))
	for value := range referenceSet {
		refs = append(refs, value)
	}
	sort.Strings(refs)
	return refs
}

// validateQRReference validates that the transaction came from a legitimate QR code
func validateQRReference(memo string, references []string, userID string, priceID uuid.UUID) error {

	parts := strings.Split(memo, ":")
	if len(parts) < 2 {
		return errors.New("invalid memo format")
	}

	timestampStr := parts[len(parts)-1]
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse timestamp from memo: %w", err)
	}

	if time.Now().Unix()-timestamp > 600 {
		return errors.New("QR code expired")
	}

	referenceData := fmt.Sprintf("%s:%s:%d", userID, priceID.String(), timestamp)
	referenceHash := sha256.Sum256([]byte(referenceData))
	expectedReference := base58.Encode(referenceHash[:])

	for _, ref := range references {
		if ref == expectedReference {
			log.WithFields(log.Fields{
				"user_id":   userID,
				"price_id":  priceID,
				"timestamp": timestamp,
				"reference": ref,
			}).Info("Valid QR reference found")
			return nil
		}
	}

	if len(references) == 0 {
		return errors.New("no QR reference accounts found in transaction")
	}

	return errors.New("no matching QR reference found")
}
