package solana

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const jupiterPriceEndpoint = "https://lite-api.jup.ag/price/v3"

var jupiterHTTPClient = &http.Client{Timeout: 5 * time.Second}

type jupiterPriceResponse map[string]struct {
	USDPrice       float64 `json:"usdPrice"`
	Decimals       int     `json:"decimals"`
	PriceChange24h float64 `json:"priceChange24h"`
}

// FetchJupiterPrices returns a map of mint -> USD price using Jupiter's lite price API.
func FetchJupiterPrices(ctx context.Context, mints []string) (map[string]float64, error) {
	result := make(map[string]float64, len(mints))
	if len(mints) == 0 {
		return result, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jupiterPriceEndpoint, nil)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("ids", strings.Join(mints, ","))
	req.URL.RawQuery = q.Encode()

	resp, err := jupiterHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected Jupiter response status: %s", resp.Status)
	}

	var pr jupiterPriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}

	for mint, data := range pr {
		result[mint] = data.USDPrice
	}

	return result, nil
}
