package solana

import (
	"context"
	"fmt"
	"math"

	"github.com/doujins-org/doujins-billing/config"
)

// calculateTokenQuote converts a fiat USD amount into token units/decimal amount based on live prices.
// Returns the amount in the token's smallest unit (lamports for SOL) and as a human-readable decimal.
func calculateTokenQuote(ctx context.Context, tokenCfg config.SolanaToken, amountUSD float64) (uint64, float64, error) {
	if amountUSD <= 0 {
		return 0, 0, nil
	}

	mint := tokenCfg.MainnetMint
	if mint == "" {
		mint = tokenCfg.Mint
	}
	if mint == "" {
		return 0, 0, fmt.Errorf("token %s missing mint configuration", tokenCfg.Symbol)
	}

	prices, err := FetchJupiterPrices(ctx, []string{mint})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch token price: %w", err)
	}

	tokenPriceUSD, ok := prices[mint]
	if !ok || tokenPriceUSD <= 0 {
		return 0, 0, fmt.Errorf("token price unavailable for %s", tokenCfg.Symbol)
	}

	scale := math.Pow10(tokenCfg.Decimals)
	tokenAmountFloat := amountUSD / tokenPriceUSD
	tokenUnits := uint64(math.Round(tokenAmountFloat * scale))
	tokenDecimal := float64(tokenUnits) / scale

	return tokenUnits, tokenDecimal, nil
}
