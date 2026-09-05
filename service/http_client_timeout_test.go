package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRelayResponseHeaderTimeoutConversion(t *testing.T) {
	maxSeconds := int64((1<<63)-1) / int64(time.Second)
	maxInt := int(^uint(0) >> 1)
	expectedMax := int64(maxInt)
	if expectedMax > maxSeconds {
		expectedMax = maxSeconds
	}

	tests := []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{name: "zero disables", seconds: 0, want: 0},
		{name: "negative disables", seconds: -1, want: 0},
		{name: "normal", seconds: 1800, want: 1800 * time.Second},
		{name: "maximum int clamps before conversion", seconds: maxInt, want: time.Duration(expectedMax) * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, relayResponseHeaderTimeout(tt.seconds))
		})
	}
}

func TestRelayTransportsApplyResponseHeaderTimeout(t *testing.T) {
	previousHeaderTimeout := common.RelayResponseHeaderTimeout
	previousRelayTimeout := common.RelayTimeout
	previousHTTPClient := httpClient
	previousProtectedClient := ssrfProtectedHTTPClient
	common.RelayResponseHeaderTimeout = 37
	common.RelayTimeout = 0
	ResetProxyClientCache()
	t.Cleanup(func() {
		if httpClient != nil && httpClient != previousHTTPClient {
			httpClient.CloseIdleConnections()
		}
		if ssrfProtectedHTTPClient != nil && ssrfProtectedHTTPClient != previousProtectedClient {
			ssrfProtectedHTTPClient.CloseIdleConnections()
		}
		httpClient = previousHTTPClient
		ssrfProtectedHTTPClient = previousProtectedClient
		common.RelayResponseHeaderTimeout = previousHeaderTimeout
		common.RelayTimeout = previousRelayTimeout
		ResetProxyClientCache()
	})

	assertTimeout := func(t *testing.T, client *http.Client) {
		t.Helper()
		transport, ok := client.Transport.(*http.Transport)
		require.True(t, ok)
		require.Equal(t, 37*time.Second, transport.ResponseHeaderTimeout)
	}

	InitHttpClient()
	assertTimeout(t, GetHttpClient())

	for _, proxyURL := range []string{
		"http://127.0.0.1:18080",
		"socks5://127.0.0.1:11080",
	} {
		client, err := NewProxyHttpClient(proxyURL)
		require.NoError(t, err)
		assertTimeout(t, client)
	}

	protectedClient := newProtectedFetchHTTPClientWithDialer(nil, nil, nil)
	defer protectedClient.CloseIdleConnections()
	protectedTransport, ok := protectedClient.Transport.(*ssrfProtectedRoundTripper)
	require.True(t, ok)
	require.Equal(t, 37*time.Second, protectedTransport.transportFor(nil).ResponseHeaderTimeout)
	require.Equal(t, 37*time.Second, protectedTransport.transportFor(mustParseURL(t, "http://127.0.0.1:3128")).ResponseHeaderTimeout)

	for _, proxyURL := range []string{
		"",
		"http://127.0.0.1:28080",
		"socks5://127.0.0.1:21080",
	} {
		client, err := NewClaudeCodeTransportHttpClient(proxyURL)
		require.NoError(t, err)
		assertTimeout(t, client)
	}
}

func TestResponseHeaderTimeoutStopsDelayedHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1200 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer server.Close()

	client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: relayResponseHeaderTimeout(1)}}
	defer client.CloseIdleConnections()

	resp, err := client.Get(server.URL)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "timeout awaiting response headers")
}

func TestResponseHeaderTimeoutDoesNotLimitBodyAfterQuickHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(1200 * time.Millisecond)
		_, _ = w.Write([]byte("complete"))
	}))
	defer server.Close()

	client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: relayResponseHeaderTimeout(1)}}
	defer client.CloseIdleConnections()

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "complete", string(body))
}
