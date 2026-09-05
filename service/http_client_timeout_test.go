package service

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
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

func TestRelayResponseHeaderTimeoutEnvironmentParsing(t *testing.T) {
	const envName = "RELAY_RESPONSE_HEADER_TIMEOUT"
	maxInt := int(^uint(0) >> 1)

	tests := []struct {
		name     string
		value    string
		want     time.Duration
		fallback bool
	}{
		{name: "empty uses 1800 second default", value: "", want: 1800 * time.Second},
		{name: "zero disables", value: "0", want: 0},
		{name: "negative disables", value: "-7", want: 0},
		{name: "positive seconds", value: "37", want: 37 * time.Second},
		{name: "maximum int clamps safely", value: strconv.Itoa(maxInt), want: relayResponseHeaderTimeout(maxInt)},
		{name: "integer overflow uses default", value: strings.Repeat("9", 100), want: 1800 * time.Second, fallback: true},
		{name: "malformed value uses default", value: "not-a-duration", want: 1800 * time.Second, fallback: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envName, tt.value)
			parsed := common.GetEnvOrDefault(envName, 1800)
			if tt.fallback {
				require.Equal(t, 1800, parsed)
			}
			require.Equal(t, tt.want, relayResponseHeaderTimeout(parsed))
		})
	}
}

func TestRelayTransportsApplyResponseHeaderTimeout(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{name: "positive", seconds: 37, want: 37 * time.Second},
		{name: "zero disabled", seconds: 0, want: 0},
		{name: "maximum int clamped", seconds: maxInt, want: relayResponseHeaderTimeout(maxInt)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withRelayResponseHeaderTimeoutTestSettings(t, tt.seconds)
			assertTimeout := func(t *testing.T, client *http.Client) {
				t.Helper()
				transport, ok := client.Transport.(*http.Transport)
				require.True(t, ok)
				require.Equal(t, tt.want, transport.ResponseHeaderTimeout)
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
			t.Cleanup(protectedClient.CloseIdleConnections)
			protectedTransport, ok := protectedClient.Transport.(*ssrfProtectedRoundTripper)
			require.True(t, ok)
			require.Equal(t, tt.want, protectedTransport.transportFor(nil).ResponseHeaderTimeout)
			require.Equal(t, tt.want, protectedTransport.transportFor(mustParseURL(t, "http://127.0.0.1:3128")).ResponseHeaderTimeout)

			for _, proxyURL := range []string{
				"",
				"http://127.0.0.1:28080",
				"socks5://127.0.0.1:21080",
			} {
				client, err := NewClaudeCodeTransportHttpClient(proxyURL)
				require.NoError(t, err)
				assertTimeout(t, client)
			}
		})
	}
}

func TestResponseHeaderTimeoutFactoryClientsRejectDelayedHeaders(t *testing.T) {
	server := newResponseHeaderTimeoutTLSServer(t)
	defer server.Close()
	clients := newResponseHeaderTimeoutFactoryClients(t)

	for _, factoryClient := range clients {
		t.Run(factoryClient.name, func(t *testing.T) {
			resp, err, _ := doBoundedTimeoutTestRequest(
				t,
				factoryClient.client,
				http.MethodGet,
				server.URL+timeoutTestDelayedHeadersPath,
				nil,
			)
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "timeout awaiting response headers")
			var netErr interface{ Timeout() bool }
			require.True(t, errors.As(err, &netErr))
			require.True(t, netErr.Timeout())
		})
	}
}

func TestResponseHeaderTimeoutFactoryClientsAllowSlowBody(t *testing.T) {
	server := newResponseHeaderTimeoutTLSServer(t)
	defer server.Close()
	clients := newResponseHeaderTimeoutFactoryClients(t)

	for _, factoryClient := range clients {
		t.Run(factoryClient.name, func(t *testing.T) {
			resp, err, _ := doBoundedTimeoutTestRequest(
				t,
				factoryClient.client,
				http.MethodGet,
				server.URL+timeoutTestSlowBodyPath,
				nil,
			)
			require.NoError(t, err)
			require.NotNil(t, resp)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, "complete", string(body))
		})
	}
}

func TestResponseHeaderTimeoutFactoryClientsAllowSlowUpload(t *testing.T) {
	server := newResponseHeaderTimeoutTLSServer(t)
	defer server.Close()
	clients := newResponseHeaderTimeoutFactoryClients(t)

	for _, factoryClient := range clients {
		t.Run(factoryClient.name, func(t *testing.T) {
			upload := &delayedTimeoutTestUpload{
				content: []byte("request body"),
				delay:   timeoutTestBoundaryDelay,
			}
			resp, err, elapsed := doBoundedTimeoutTestRequest(
				t,
				factoryClient.client,
				http.MethodPost,
				server.URL+timeoutTestSlowUploadPath,
				upload,
			)
			require.NoError(t, err)
			require.NotNil(t, resp)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, "uploaded", string(body))
			require.GreaterOrEqual(t, elapsed, time.Second)
		})
	}
}
