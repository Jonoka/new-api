package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func resetDisplayExchangeRateCacheForTest() {
	displayExchangeRateMu.Lock()
	defer displayExchangeRateMu.Unlock()
	displayExchangeRateCache = displayExchangeRateCacheEntry{}
}

func TestParseCoinGeckoUSDTToCNYRate(t *testing.T) {
	rate, updatedAt, err := parseCoinGeckoUSDTToCNYRate([]byte(`{"tether":{"cny":6.77,"last_updated_at":1784324511}}`))

	require.NoError(t, err)
	require.InDelta(t, 6.77, rate, 0.000001)
	require.Equal(t, int64(1784324511), updatedAt)
}

func TestParseCoinGeckoUSDTToCNYRateRejectsInvalidPrice(t *testing.T) {
	_, _, err := parseCoinGeckoUSDTToCNYRate([]byte(`{"tether":{"cny":0}}`))
	require.Error(t, err)
}

func TestResolveDisplayExchangeRateUsesCache(t *testing.T) {
	resetDisplayExchangeRateCacheForTest()
	t.Cleanup(resetDisplayExchangeRateCacheForTest)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"tether":{"cny":6.76,"last_updated_at":1784324511}}`))
	}))
	defer server.Close()

	now := time.Unix(1784324600, 0)
	first := resolveDisplayExchangeRate(context.Background(), true, 7.3, server.URL, server.Client(), now)
	second := resolveDisplayExchangeRate(context.Background(), true, 7.3, server.URL, server.Client(), now.Add(time.Minute))

	require.Equal(t, int32(1), requests.Load())
	require.Equal(t, displayExchangeRateSourceCoinGecko, first.Source)
	require.False(t, first.IsFallback)
	require.InDelta(t, 6.76, first.Rate, 0.000001)
	require.Equal(t, first, second)
}

func TestResolveDisplayExchangeRateFallsBackAndThrottlesRetry(t *testing.T) {
	resetDisplayExchangeRateCacheForTest()
	t.Cleanup(resetDisplayExchangeRateCacheForTest)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	now := time.Unix(1784324600, 0)
	first := resolveDisplayExchangeRate(context.Background(), true, 7.3, server.URL, server.Client(), now)
	second := resolveDisplayExchangeRate(context.Background(), true, 7.3, server.URL, server.Client(), now.Add(30*time.Second))

	require.Equal(t, int32(1), requests.Load())
	require.Equal(t, displayExchangeRateSourceConfigured, first.Source)
	require.True(t, first.IsFallback)
	require.InDelta(t, 7.3, first.Rate, 0.000001)
	require.Equal(t, first, second)
}

func TestResolveDisplayExchangeRateUsesStaleCacheWhenRefreshFails(t *testing.T) {
	resetDisplayExchangeRateCacheForTest()
	t.Cleanup(resetDisplayExchangeRateCacheForTest)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"tether":{"cny":6.76,"last_updated_at":1784324511}}`))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	now := time.Unix(1784324600, 0)
	fresh := resolveDisplayExchangeRate(context.Background(), true, 7.3, server.URL, server.Client(), now)
	stale := resolveDisplayExchangeRate(
		context.Background(),
		true,
		7.3,
		server.URL,
		server.Client(),
		now.Add(displayExchangeRateCacheTTL),
	)
	reused := resolveDisplayExchangeRate(
		context.Background(),
		true,
		7.3,
		server.URL,
		server.Client(),
		now.Add(displayExchangeRateCacheTTL+30*time.Second),
	)

	require.Equal(t, int32(2), requests.Load())
	require.Equal(t, displayExchangeRateSourceCoinGecko, fresh.Source)
	require.Equal(t, displayExchangeRateSourceCoinGeckoStale, stale.Source)
	require.True(t, stale.IsFallback)
	require.InDelta(t, fresh.Rate, stale.Rate, 0.000001)
	require.Equal(t, stale, reused)
}

func TestResolveDisplayExchangeRateCanUseConfiguredRateOnly(t *testing.T) {
	resetDisplayExchangeRateCacheForTest()
	t.Cleanup(resetDisplayExchangeRateCacheForTest)

	rate := resolveDisplayExchangeRate(context.Background(), false, 7.2, "::invalid::", http.DefaultClient, time.Now())

	require.Equal(t, displayExchangeRateSourceConfigured, rate.Source)
	require.False(t, rate.IsFallback)
	require.InDelta(t, 7.2, rate.Rate, 0.000001)
}
