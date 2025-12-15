package solana

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/doujins-org/solana-go"
	"github.com/doujins-org/solana-go/rpc"
	log "github.com/sirupsen/logrus"
)

// SolanaRPCService handles interactions with the Solana blockchain
type SolanaRPCService struct {
	client   *rpc.Client
	endpoint string
	network  string // "mainnet", "devnet", "testnet"
}

// NewSolanaRPCService creates a new Solana RPC service
func NewSolanaRPCService(endpoint, network string) *SolanaRPCService {
	if endpoint == "" {
		switch network {
		case "mainnet":
			endpoint = "https://api.mainnet-beta.solana.com"
		case "devnet":
			endpoint = "https://api.devnet.solana.com"
		case "testnet":
			endpoint = "https://api.testnet.solana.com"
		default:
			endpoint = "https://api.devnet.solana.com"
			network = "devnet"
		}
	}

	client := rpc.New(endpoint)
	log.WithFields(log.Fields{
		"endpoint": endpoint,
		"network":  network,
	}).Info("Initialized Solana RPC client")

	return &SolanaRPCService{
		client:   client,
		endpoint: endpoint,
		network:  network,
	}
}

// GetBalance returns the SOL balance for an address
func (s *SolanaRPCService) GetBalance(ctx context.Context, address solana.PublicKey) (uint64, error) {
	balance, err := s.client.GetBalance(ctx, address, rpc.CommitmentFinalized)
	if err != nil {
		return 0, fmt.Errorf("failed to get balance for %s: %w", address.String(), err)
	}
	return balance.Value, nil
}

// GetTokenBalance returns the SPL token balance for an address and mint
func (s *SolanaRPCService) GetTokenBalance(ctx context.Context, tokenAccount solana.PublicKey) (*rpc.UiTokenAmount, error) {
	resp, err := s.client.GetTokenAccountBalance(ctx, tokenAccount, rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get token balance for %s: %w", tokenAccount.String(), err)
	}
	return resp.Value, nil
}

// SimulateTransaction simulates a transaction to check if it would succeed
func (s *SolanaRPCService) SimulateTransaction(ctx context.Context, tx *solana.Transaction) (*rpc.SimulateTransactionResponse, error) {
	resp, err := s.client.SimulateTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to simulate transaction: %w", err)
	}
	return resp, nil
}

// SendTransaction submits a transaction to the blockchain
func (s *SolanaRPCService) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	sig, err := s.client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	log.WithFields(log.Fields{
		"signature": sig.String(),
		"network":   s.network,
	}).Info("Transaction sent to Solana")

	return sig, nil
}

// GetTransaction retrieves transaction details by signature
func (s *SolanaRPCService) GetTransaction(ctx context.Context, signature solana.Signature) (*rpc.GetTransactionResult, error) {
	resp, err := s.client.GetTransaction(ctx, signature, &rpc.GetTransactionOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction %s: %w", signature.String(), err)
	}
	return resp, nil
}

func (s *SolanaRPCService) GetTransactionWithRetry(ctx context.Context, signature solana.Signature, attempts int, delay time.Duration) (*rpc.GetTransactionResult, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		resp, err := s.GetTransaction(ctx, signature)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isNotFoundError(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// ConfirmTransaction waits for a transaction to be confirmed
func (s *SolanaRPCService) ConfirmTransaction(ctx context.Context, signature solana.Signature, commitment rpc.CommitmentType) error {
	timeout := 60 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("transaction confirmation timeout for %s", signature.String())
		case <-ticker.C:
			status, err := s.client.GetSignatureStatuses(ctx, true, signature)
			if err != nil {
				log.WithError(err).Warn("Failed to get signature status")
				continue
			}

			if len(status.Value) > 0 && status.Value[0] != nil {
				sigStatus := status.Value[0]

				// Check for errors
				if sigStatus.Err != nil {
					return fmt.Errorf("transaction failed: %v", sigStatus.Err)
				}

				// Check if confirmed at desired level
				if sigStatus.ConfirmationStatus != "" {
					switch commitment {
					case rpc.CommitmentProcessed:
						if sigStatus.ConfirmationStatus == rpc.ConfirmationStatusProcessed ||
							sigStatus.ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
							sigStatus.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
							return nil
						}
					case rpc.CommitmentConfirmed:
						if sigStatus.ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
							sigStatus.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
							return nil
						}
					case rpc.CommitmentFinalized:
						if sigStatus.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
							return nil
						}
					}
				}
			}
		}
	}
}

// GetLatestBlockhash gets the latest blockhash for transaction creation
func (s *SolanaRPCService) GetLatestBlockhash(ctx context.Context) (solana.Hash, error) {
	resp, err := s.client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return solana.Hash{}, fmt.Errorf("failed to get latest blockhash: %w", err)
	}
	return resp.Value.Blockhash, nil
}

// GetMinimumBalanceForRentExemption returns the minimum balance needed for rent exemption
func (s *SolanaRPCService) GetMinimumBalanceForRentExemption(ctx context.Context, dataSize uint64) (uint64, error) {
	balance, err := s.client.GetMinimumBalanceForRentExemption(ctx, dataSize, rpc.CommitmentFinalized)
	if err != nil {
		return 0, fmt.Errorf("failed to get minimum balance for rent exemption: %w", err)
	}
	return balance, nil
}

// IsValidAddress checks if a public key string is valid
func (s *SolanaRPCService) IsValidAddress(address string) bool {
	_, err := solana.PublicKeyFromBase58(address)
	return err == nil
}

// ParseAddress converts a base58 string to PublicKey
func (s *SolanaRPCService) ParseAddress(address string) (solana.PublicKey, error) {
	return solana.PublicKeyFromBase58(address)
}

// GetNetwork returns the current network
func (s *SolanaRPCService) GetNetwork() string {
	return s.network
}

// GetEndpoint returns the current RPC endpoint
func (s *SolanaRPCService) GetEndpoint() string {
	return s.endpoint
}

// GetSignaturesForAddress finds transactions that reference a specific address
func (s *SolanaRPCService) GetSignaturesForAddress(ctx context.Context, address solana.PublicKey, limit *int) ([]*rpc.TransactionSignature, error) {
	opts := &rpc.GetSignaturesForAddressOpts{
		Commitment: rpc.CommitmentFinalized,
	}
	if limit != nil {
		limitVal := *limit
		opts.Limit = &limitVal
	}

	resp, err := s.client.GetSignaturesForAddressWithOpts(ctx, address, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get signatures for address %s: %w", address.String(), err)
	}

	return resp, nil
}

// GetClient returns the underlying RPC client for direct access when needed
func (s *SolanaRPCService) GetClient() *rpc.Client {
	return s.client
}
