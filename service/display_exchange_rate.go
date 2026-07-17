package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const coinGeckoUSDTToCNYURL = "https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true"

const (
	displayExchangeRateCacheTTL = 5 * time.Minute
	displayExchangeRateRetryTTL = time.Minute

	displayExchangeRateSourceConfigured     = "configured"
	displayExchangeRateSourceCoinGecko      = "coingecko"
	displayExchangeRateSourceCoinGeckoStale = "coingecko-stale"
)

type DisplayExchangeRate struct {
	Rate          float64
	Source        string
	LastUpdatedAt int64
	IsFallback    bool
}

type coinGeckoSimplePriceResponse struct {
	Tether struct {
		CNY           float64 `json:"cny"`
		LastUpdatedAt int64   `json:"last_updated_at"`
	} `json:"tether"`
}

type displayExchangeRateCacheEntry struct {
	Endpoint      string
	Rate          float64
	LastUpdatedAt int64
	ExpiresAt     time.Time
	RetryAfter    time.Time
}

var (
	displayExchangeRateMu    sync.Mutex
	displayExchangeRateCache displayExchangeRateCacheEntry
	displayExchangeRateHTTP  = &http.Client{Timeout: 3 * time.Second}
)

func parseCoinGeckoUSDTToCNYRate(body []byte) (float64, int64, error) {
	var payload coinGeckoSimplePriceResponse
	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, 0, err
	}
	if payload.Tether.CNY <= 0 || math.IsNaN(payload.Tether.CNY) || math.IsInf(payload.Tether.CNY, 0) {
		return 0, 0, fmt.Errorf("invalid tether cny price")
	}
	return payload.Tether.CNY, payload.Tether.LastUpdatedAt, nil
}

func fetchCoinGeckoUSDTToCNYRate(ctx context.Context, endpoint string, client *http.Client) (float64, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "new-api/exchange-rate")

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, 0, fmt.Errorf("coingecko rate api http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return 0, 0, err
	}
	return parseCoinGeckoUSDTToCNYRate(body)
}

func normalizeDisplayExchangeRateFallback(rate float64) float64 {
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 1
	}
	return rate
}

func displayExchangeRateFallback(rate float64, isFallback bool) DisplayExchangeRate {
	return DisplayExchangeRate{
		Rate:       normalizeDisplayExchangeRateFallback(rate),
		Source:     displayExchangeRateSourceConfigured,
		IsFallback: isFallback,
	}
}

func resolveDisplayExchangeRate(
	ctx context.Context,
	autoEnabled bool,
	fallbackRate float64,
	endpoint string,
	client *http.Client,
	now time.Time,
) DisplayExchangeRate {
	if !autoEnabled {
		return displayExchangeRateFallback(fallbackRate, false)
	}

	displayExchangeRateMu.Lock()
	defer displayExchangeRateMu.Unlock()

	if displayExchangeRateCache.Endpoint == endpoint {
		if displayExchangeRateCache.Rate > 0 && now.Before(displayExchangeRateCache.ExpiresAt) {
			return DisplayExchangeRate{
				Rate:          displayExchangeRateCache.Rate,
				Source:        displayExchangeRateSourceCoinGecko,
				LastUpdatedAt: displayExchangeRateCache.LastUpdatedAt,
			}
		}
		if now.Before(displayExchangeRateCache.RetryAfter) {
			if displayExchangeRateCache.Rate > 0 {
				return DisplayExchangeRate{
					Rate:          displayExchangeRateCache.Rate,
					Source:        displayExchangeRateSourceCoinGeckoStale,
					LastUpdatedAt: displayExchangeRateCache.LastUpdatedAt,
					IsFallback:    true,
				}
			}
			return displayExchangeRateFallback(fallbackRate, true)
		}
	}

	rate, lastUpdatedAt, err := fetchCoinGeckoUSDTToCNYRate(ctx, endpoint, client)
	if err != nil {
		if displayExchangeRateCache.Endpoint != endpoint {
			displayExchangeRateCache = displayExchangeRateCacheEntry{Endpoint: endpoint}
		}
		displayExchangeRateCache.RetryAfter = now.Add(displayExchangeRateRetryTTL)
		if displayExchangeRateCache.Rate > 0 {
			return DisplayExchangeRate{
				Rate:          displayExchangeRateCache.Rate,
				Source:        displayExchangeRateSourceCoinGeckoStale,
				LastUpdatedAt: displayExchangeRateCache.LastUpdatedAt,
				IsFallback:    true,
			}
		}
		return displayExchangeRateFallback(fallbackRate, true)
	}

	displayExchangeRateCache = displayExchangeRateCacheEntry{
		Endpoint:      endpoint,
		Rate:          rate,
		LastUpdatedAt: lastUpdatedAt,
		ExpiresAt:     now.Add(displayExchangeRateCacheTTL),
	}
	return DisplayExchangeRate{
		Rate:          rate,
		Source:        displayExchangeRateSourceCoinGecko,
		LastUpdatedAt: lastUpdatedAt,
	}
}

func ResolveDisplayExchangeRate(ctx context.Context, autoEnabled bool, fallbackRate float64) DisplayExchangeRate {
	return resolveDisplayExchangeRate(
		ctx,
		autoEnabled,
		fallbackRate,
		coinGeckoUSDTToCNYURL,
		displayExchangeRateHTTP,
		time.Now(),
	)
}
