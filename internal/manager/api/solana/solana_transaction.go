package solana

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"strconv"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/solana-go"
	"github.com/doujins-org/solana-go/programs/system"
	"github.com/doujins-org/solana-go/programs/token"
	"github.com/doujins-org/solana-go/rpc"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// SolanaTransactionService builds real Solana transactions for payments
type SolanaTransactionService struct {
	db           *db.DB
	rpc          *SolanaRPCService
	cfg          *config.Config
	priceService *price.PriceService
	paymentSvc   *payment.PaymentService
}

// NewSolanaTransactionService creates a new transaction service
func NewSolanaTransactionService(db *db.DB, rpc *SolanaRPCService, cfg *config.Config, price *price.PriceService, payment *payment.PaymentService) *SolanaTransactionService {
	return &SolanaTransactionService{
		db:           db,
		rpc:          rpc,
		cfg:          cfg,
		priceService: price,
		paymentSvc:   payment,
	}
}

// TransactionRequest represents a payment transaction request
type TransactionRequest struct {
	PriceID    uuid.UUID
	TokenMint  solana.PublicKey
	FromWallet solana.PublicKey
	ToWallet   solana.PublicKey
	Amount     uint64 // Amount in token's smallest unit (lamports for SOL, smallest unit for SPL tokens)
}

// TransactionResponse contains the built transaction and metadata
type TransactionResponse struct {
	Transaction       *solana.Transaction
	TransactionBase64 string
	Amount            float64
	TokenAmount       uint64
	TokenSymbol       string
	ExpiresAt         time.Time
	Instructions      string
}

// BuildPaymentTransaction creates a real Solana transaction for payment
func (s *SolanaTransactionService) BuildPaymentTransaction(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, userWallet string) (*TransactionResponse, error) {
	// Get price information
	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	// Validate Solana configuration
	if s.cfg.Solana == nil {
		return nil, fmt.Errorf("solana configuration not found")
	}

	// Get token configuration
	tokenCfg, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		return nil, fmt.Errorf("token %s not supported", tokenSymbol)
	}

	// Parse wallet addresses
	fromWallet, err := solana.PublicKeyFromBase58(userWallet)
	if err != nil {
		return nil, fmt.Errorf("invalid user wallet address: %w", err)
	}

	merchantWallet := s.cfg.Solana.RecipientWallet

	if merchantWallet == "" {
		return nil, fmt.Errorf("merchant wallet not configured")
	}

	toWallet, err := solana.PublicKeyFromBase58(merchantWallet)
	if err != nil {
		return nil, fmt.Errorf("invalid merchant wallet address: %w", err)
	}

	// Calculate token amount in smallest units using current market quote
	tokenAmount, tokenAmountDecimal, err := calculateTokenQuote(ctx, tokenCfg, price.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate token amount: %w", err)
	}
	if tokenAmount == 0 {
		return nil, fmt.Errorf("calculated token amount is zero")
	}

	// Get latest blockhash
	blockhash, err := s.rpc.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get blockhash: %w", err)
	}

	var transaction *solana.Transaction
	var instructions []solana.Instruction

	if tokenSymbol == "SOL" {
		// Native SOL transfer
		instruction := system.NewTransferInstruction(
			tokenAmount,
			fromWallet,
			toWallet,
		).Build()
		instructions = append(instructions, instruction)
	} else {
		// SPL Token transfer
		var tokenMint solana.PublicKey
		tokenMint, err = solana.PublicKeyFromBase58(tokenCfg.Mint)
		if err != nil {
			return nil, fmt.Errorf("invalid token mint address: %w", err)
		}

		// Find associated token accounts
		var fromTokenAccount solana.PublicKey
		fromTokenAccount, _, err = solana.FindAssociatedTokenAddress(fromWallet, tokenMint)
		if err != nil {
			return nil, fmt.Errorf("failed to find from token account: %w", err)
		}

		var toTokenAccount solana.PublicKey
		toTokenAccount, _, err = solana.FindAssociatedTokenAddress(toWallet, tokenMint)
		if err != nil {
			return nil, fmt.Errorf("failed to find to token account: %w", err)
		}

		// Create transfer instruction
		instruction := token.NewTransferInstruction(
			tokenAmount,
			fromTokenAccount,
			toTokenAccount,
			fromWallet,
			[]solana.PublicKey{},
		).Build()
		instructions = append(instructions, instruction)
	}

	// Build transaction
	transaction, err = solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(fromWallet),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Serialize transaction for frontend
	txBytes, err := transaction.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	expiresAt := time.Now().Add(10 * time.Minute)

	log.WithFields(log.Fields{
		"user_id":      userID,
		"price_id":     priceID,
		"token":        tokenSymbol,
		"amount":       price.Amount,
		"token_amount": tokenAmountDecimal,
		"from_wallet":  userWallet,
		"to_wallet":    merchantWallet,
	}).Info("Built Solana payment transaction")

	return &TransactionResponse{
		Transaction:       transaction,
		TransactionBase64: base64.StdEncoding.EncodeToString(txBytes),
		Amount:            price.Amount,
		TokenAmount:       tokenAmount,
		TokenSymbol:       tokenSymbol,
		ExpiresAt:         expiresAt,
		Instructions:      fmt.Sprintf("Sign this transaction to pay %.2f %s using %s", price.Amount, price.Currency, tokenSymbol),
	}, nil
}

// SimulateTransaction simulates the transaction to check if it would succeed
func (s *SolanaTransactionService) SimulateTransaction(ctx context.Context, tx *solana.Transaction) error {
	resp, err := s.rpc.SimulateTransaction(ctx, tx)
	if err != nil {
		return fmt.Errorf("transaction simulation failed: %w", err)
	}

	if resp.Value.Err != nil {
		return fmt.Errorf("transaction would fail: %v", resp.Value.Err)
	}

	log.WithFields(log.Fields{
		"units_consumed": resp.Value.UnitsConsumed,
		"logs":           resp.Value.Logs,
	}).Info("Transaction simulation successful")

	return nil
}

// VerifyTransactionSignature verifies a signed transaction on-chain without enforcing content checks.
func (s *SolanaTransactionService) VerifyTransactionSignature(ctx context.Context, signature string) (*rpc.GetTransactionResult, error) {
	return s.fetchConfirmedTransaction(ctx, signature)
}

// VerifyTransactionWithContent verifies a transaction against expected recipient, payer, and optional reference.
func (s *SolanaTransactionService) VerifyTransactionWithContent(ctx context.Context, signature string, expectedAmount uint64, expectedRecipient string, expectedTokenMint string, expectedPayer string, expectedReference *string) (*rpc.GetTransactionResult, error) {
	txResult, err := s.fetchConfirmedTransaction(ctx, signature)
	if err != nil {
		return nil, err
	}

	if expectedAmount > 0 && expectedRecipient != "" {
		if err := s.validateTransactionContent(txResult, expectedAmount, expectedRecipient, expectedTokenMint, expectedPayer, expectedReference); err != nil {
			return nil, fmt.Errorf("transaction content validation failed: %w", err)
		}
	}

	log.WithFields(log.Fields{
		"signature": signature,
		"slot":      txResult.Slot,
		"fee":       txResult.Meta.Fee,
	}).Info("Transaction verified on-chain")

	return txResult, nil
}

func (s *SolanaTransactionService) fetchConfirmedTransaction(ctx context.Context, signature string) (*rpc.GetTransactionResult, error) {
	sig, err := solana.SignatureFromBase58(signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature format: %w", err)
	}

	if err = s.rpc.ConfirmTransaction(ctx, sig, rpc.CommitmentConfirmed); err != nil {
		return nil, fmt.Errorf("transaction confirmation failed: %w", err)
	}

	txResult, err := s.rpc.GetTransactionWithRetry(ctx, sig, 5, 1*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	if txResult.Meta == nil {
		return nil, fmt.Errorf("transaction metadata not available")
	}

	if txResult.Meta.Err != nil {
		return nil, fmt.Errorf("transaction failed on-chain: %v", txResult.Meta.Err)
	}

	return txResult, nil
}

func (s *SolanaTransactionService) validateTransactionContent(txResult *rpc.GetTransactionResult, expectedAmount uint64, expectedRecipient string, expectedTokenMint string, expectedPayer string, expectedReference *string) error {
	if txResult.Transaction == nil {
		return fmt.Errorf("transaction data not available")
	}

	tx, err := txResult.Transaction.GetTransaction()
	if err != nil {
		return fmt.Errorf("failed to decode transaction: %w", err)
	}

	if expectedPayer != "" {
		payerPub, err := solana.PublicKeyFromBase58(expectedPayer)
		if err != nil {
			return fmt.Errorf("invalid expected payer: %w", err)
		}
		if len(tx.Message.AccountKeys) == 0 || !tx.Message.AccountKeys[0].Equals(payerPub) {
			return fmt.Errorf("transaction fee payer does not match expected wallet")
		}
	}

	if expectedReference != nil && *expectedReference != "" {
		referencePub, err := solana.PublicKeyFromBase58(*expectedReference)
		if err != nil {
			return fmt.Errorf("invalid reference key: %w", err)
		}
		if !messageContainsKey(&tx.Message, referencePub, txResult.Meta.LoadedAddresses) {
			return fmt.Errorf("reference key not included in transaction")
		}
	}

	recipientCandidates := make(map[string]struct{})
	recipientCandidates[expectedRecipient] = struct{}{}
	if derived, err := s.deriveRecipientTokenAccount(expectedRecipient, expectedTokenMint); err == nil && derived != "" {
		recipientCandidates[derived] = struct{}{}
	}

	match, err := s.findTransferMatch(tx, txResult, recipientCandidates, expectedTokenMint, expectedAmount, expectedPayer)
	if err != nil {
		return err
	}
	if match == nil {
		return fmt.Errorf("no qualifying transfer found for recipient %s", expectedRecipient)
	}

	if err := verifyBalanceChanges(txResult, match.accountIndex, match.destination.String(), expectedAmount); err != nil {
		return err
	}

	return nil
}

func (s *SolanaTransactionService) deriveRecipientTokenAccount(recipient string, tokenMint string) (string, error) {
	if strings.TrimSpace(tokenMint) == "" {
		return "", nil
	}
	recipientPub, err := solana.PublicKeyFromBase58(recipient)
	if err != nil {
		return "", err
	}
	mintPub, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return "", err
	}
	ata, _, err := solana.FindAssociatedTokenAddress(recipientPub, mintPub)
	if err != nil {
		return "", err
	}
	return ata.String(), nil
}

type transferMatch struct {
	program      string
	amount       uint64
	source       solana.PublicKey
	destination  solana.PublicKey
	mint         string
	accountIndex int
}

func (s *SolanaTransactionService) findTransferMatch(tx *solana.Transaction, txResult *rpc.GetTransactionResult, recipientCandidates map[string]struct{}, expectedTokenMint string, expectedAmount uint64, expectedPayer string) (*transferMatch, error) {
	if tx == nil {
		return nil, errors.New("transaction message unavailable")
	}

	if len(recipientCandidates) == 0 {
		return nil, errors.New("no recipient candidates provided")
	}

	candidateKeys := make(map[string]solana.PublicKey, len(recipientCandidates))
	for addr := range recipientCandidates {
		pub, err := solana.PublicKeyFromBase58(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient candidate %s: %w", addr, err)
		}
		candidateKeys[addr] = pub
	}

	var bestMatch *transferMatch
	expectedMintNorm := normalizeMint(expectedTokenMint)

	for instIdx, inst := range tx.Message.Instructions {
		programID, err := tx.ResolveProgramIDIndex(inst.ProgramIDIndex)
		if err != nil {
			continue
		}
		accounts, err := inst.ResolveInstructionAccounts(&tx.Message)
		if err != nil {
			continue
		}

		switch {
		case programID.Equals(system.ProgramID):
			sysInstr, err := system.DecodeInstruction(accounts, inst.Data)
			if err != nil || sysInstr == nil {
				continue
			}
			transfer, ok := sysInstr.Impl.(*system.Transfer)
			if !ok {
				continue
			}
			match := s.evaluateSystemTransfer(transfer, accountIndexFromInstruction(&inst, 1), candidateKeys, expectedAmount, expectedPayer)
			if match != nil {
				bestMatch = pickBetterMatch(bestMatch, match, instIdx)
			}
		case programID.Equals(token.ProgramID):
			tokenInstr, err := token.DecodeInstruction(accounts, inst.Data)
			if err != nil || tokenInstr == nil {
				continue
			}
			switch dec := tokenInstr.Impl.(type) {
			case *token.Transfer:
				match := s.evaluateTokenTransfer(txResult, accounts, dec.Amount, accountIndexFromInstruction(&inst, 1), candidateKeys, expectedMintNorm, expectedAmount, expectedPayer)
				if match != nil {
					bestMatch = pickBetterMatch(bestMatch, match, instIdx)
				}
			case *token.TransferChecked:
				match := s.evaluateTokenTransferChecked(txResult, accounts, dec.Amount, accountIndexFromInstruction(&inst, 2), candidateKeys, expectedMintNorm, expectedAmount, expectedPayer)
				if match != nil {
					bestMatch = pickBetterMatch(bestMatch, match, instIdx)
				}
			}
		default:
			continue
		}
	}

	return bestMatch, nil
}

func (s *SolanaTransactionService) evaluateSystemTransfer(dec *system.Transfer, accountIdx int, candidates map[string]solana.PublicKey, expectedAmount uint64, expectedPayer string) *transferMatch {
	if dec == nil || dec.Lamports == nil {
		return nil
	}
	if accountIdx < 0 {
		return nil
	}

	sourceMeta := dec.GetFundingAccount()
	destMeta := dec.GetRecipientAccount()

	if sourceMeta == nil || destMeta == nil {
		return nil
	}
	if expectedPayer != "" && sourceMeta.PublicKey.String() != expectedPayer {
		return nil
	}
	if _, ok := candidates[destMeta.PublicKey.String()]; !ok {
		return nil
	}
	if *dec.Lamports < expectedAmount {
		return nil
	}

	return &transferMatch{
		program:      "system",
		amount:       *dec.Lamports,
		source:       sourceMeta.PublicKey,
		destination:  destMeta.PublicKey,
		mint:         "",
		accountIndex: accountIdx,
	}
}

func (s *SolanaTransactionService) evaluateTokenTransfer(txResult *rpc.GetTransactionResult, accounts []*solana.AccountMeta, amountPtr *uint64, accountIdx int, candidates map[string]solana.PublicKey, expectedMint string, expectedAmount uint64, expectedPayer string) *transferMatch {
	if amountPtr == nil || accountIdx < 0 {
		return nil
	}
	if len(accounts) < 3 {
		return nil
	}
	dest := accounts[1].PublicKey
	if _, ok := candidates[dest.String()]; !ok {
		return nil
	}
	if expectedPayer != "" && accounts[2].PublicKey.String() != expectedPayer {
		return nil
	}
	mint := mintForAccount(txResult, accountIdx)
	if expectedMint != "" && mint != "" && !mintMatches(expectedMint, mint) {
		return nil
	}
	if *amountPtr < expectedAmount {
		return nil
	}
	return &transferMatch{
		program:      "token",
		amount:       *amountPtr,
		source:       accounts[0].PublicKey,
		destination:  dest,
		mint:         mint,
		accountIndex: accountIdx,
	}
}

func (s *SolanaTransactionService) evaluateTokenTransferChecked(txResult *rpc.GetTransactionResult, accounts []*solana.AccountMeta, amountPtr *uint64, accountIdx int, candidates map[string]solana.PublicKey, expectedMint string, expectedAmount uint64, expectedPayer string) *transferMatch {
	if amountPtr == nil || accountIdx < 0 {
		return nil
	}
	if len(accounts) < 4 {
		return nil
	}
	dest := accounts[2].PublicKey
	if _, ok := candidates[dest.String()]; !ok {
		return nil
	}
	if expectedPayer != "" && accounts[3].PublicKey.String() != expectedPayer {
		return nil
	}
	mint := ""
	if len(accounts) > 1 {
		mint = accounts[1].PublicKey.String()
	}
	if mint == "" {
		mint = mintForAccount(txResult, accountIdx)
	}
	if expectedMint != "" && mint != "" && !mintMatches(expectedMint, mint) {
		return nil
	}
	if *amountPtr < expectedAmount {
		return nil
	}
	return &transferMatch{
		program:      "token",
		amount:       *amountPtr,
		source:       accounts[0].PublicKey,
		destination:  dest,
		mint:         mint,
		accountIndex: accountIdx,
	}
}

func pickBetterMatch(current, candidate *transferMatch, _ int) *transferMatch {
	if candidate == nil {
		return current
	}
	if current == nil {
		return candidate
	}
	if candidate.amount > current.amount {
		return candidate
	}
	return current
}

func accountIndexFromInstruction(inst *solana.CompiledInstruction, accountPosition int) int {
	if inst == nil || accountPosition >= len(inst.Accounts) {
		return -1
	}
	return int(inst.Accounts[accountPosition])
}

func mintForAccount(txResult *rpc.GetTransactionResult, accountIndex int) string {
	if txResult == nil || txResult.Meta == nil {
		return ""
	}
	for i := range txResult.Meta.PostTokenBalances {
		if int(txResult.Meta.PostTokenBalances[i].AccountIndex) == accountIndex {
			return txResult.Meta.PostTokenBalances[i].Mint.String()
		}
	}
	return ""
}

func normalizeMint(m string) string {
	return strings.ToUpper(strings.TrimSpace(m))
}

func mintMatches(expected, actual string) bool {
	exp := normalizeMint(expected)
	act := normalizeMint(actual)
	if exp == "" || act == "" {
		return exp == act
	}
	if isNativeSOLMint(exp) && isNativeSOLMint(act) {
		return true
	}
	return exp == act
}

func messageContainsKey(msg *solana.Message, key solana.PublicKey, loaded rpc.LoadedAddresses) bool {
	if msg != nil {
		for _, k := range msg.AccountKeys {
			if k.Equals(key) {
				return true
			}
		}
	}
	for _, k := range loaded.Writable {
		if k.Equals(key) {
			return true
		}
	}
	for _, k := range loaded.ReadOnly {
		if k.Equals(key) {
			return true
		}
	}
	return false
}

func findAccountIndex(msg *solana.Message, key solana.PublicKey, loaded rpc.LoadedAddresses) int {
	offset := 0
	if msg != nil {
		for idx, k := range msg.AccountKeys {
			if k.Equals(key) {
				return idx
			}
		}
		offset = len(msg.AccountKeys)
	}
	for i, k := range loaded.Writable {
		if k.Equals(key) {
			return offset + i
		}
	}
	offset += len(loaded.Writable)
	for i, k := range loaded.ReadOnly {
		if k.Equals(key) {
			return offset + i
		}
	}
	return -1
}

func verifyBalanceChanges(txResult *rpc.GetTransactionResult, accountIndex int, account string, expectedAmount uint64) error {
	if accountIndex < len(txResult.Meta.PostBalances) && accountIndex < len(txResult.Meta.PreBalances) {
		post := txResult.Meta.PostBalances[accountIndex]
		pre := txResult.Meta.PreBalances[accountIndex]
		if post >= pre {
			delta := post - pre
			if delta >= expectedAmount {
				return nil
			}
		}
	}

	postToken, preToken, err := tokenBalanceDelta(txResult, accountIndex)
	if err != nil {
		return fmt.Errorf("unable to confirm balance change for account %s: %w", account, err)
	}
	if postToken < preToken {
		return fmt.Errorf("token balance decreased for account %s", account)
	}
	if postToken-preToken < expectedAmount {
		return fmt.Errorf("token transfer amount insufficient: expected >= %d, observed %d", expectedAmount, postToken-preToken)
	}
	return nil
}

func tokenBalanceDelta(txResult *rpc.GetTransactionResult, accountIndex int) (uint64, uint64, error) {
	var (
		postAmount uint64
		preAmount  uint64
		found      bool
	)
	for _, post := range txResult.Meta.PostTokenBalances {
		if int(post.AccountIndex) == accountIndex {
			if post.UiTokenAmount == nil {
				return 0, 0, fmt.Errorf("post token amount missing")
			}
			amt, err := strconv.ParseUint(post.UiTokenAmount.Amount, 10, 64)
			if err != nil {
				return 0, 0, err
			}
			postAmount = amt
			found = true
			break
		}
	}
	if !found {
		return 0, 0, fmt.Errorf("token balance not found")
	}
	for _, pre := range txResult.Meta.PreTokenBalances {
		if int(pre.AccountIndex) == accountIndex {
			if pre.UiTokenAmount == nil {
				return 0, 0, fmt.Errorf("pre token amount missing")
			}
			amt, err := strconv.ParseUint(pre.UiTokenAmount.Amount, 10, 64)
			if err != nil {
				return 0, 0, err
			}
			preAmount = amt
			break
		}
	}
	return postAmount, preAmount, nil
}

const wrappedSOLMint = "So11111111111111111111111111111111111111112"

var nativeSOLMintAliases = map[string]struct{}{
	"":                              {},
	strings.ToUpper(wrappedSOLMint): {},
}

func isNativeSOLMint(tokenMint string) bool {
	mint := strings.ToUpper(strings.TrimSpace(tokenMint))
	if _, ok := nativeSOLMintAliases[mint]; ok {
		return true
	}
	return false
}
