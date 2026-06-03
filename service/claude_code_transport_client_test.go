package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaudeCodeTransportClientEnablesHTTP2ALPN(t *testing.T) {
	client, err := NewClaudeCodeTransportHttpClient("")
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.True(t, transport.ForceAttemptHTTP2)
	require.NotNil(t, transport.Proxy)
	require.NotNil(t, transport.TLSClientConfig)
	require.Contains(t, transport.TLSClientConfig.NextProtos, "h2")
	require.Contains(t, transport.TLSClientConfig.NextProtos, "http/1.1")
}

func TestClaudeCodeTransportClientKeepsHTTPProxy(t *testing.T) {
	client, err := NewClaudeCodeTransportHttpClient("http://127.0.0.1:18080")
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.Proxy)
	require.NotNil(t, transport.TLSClientConfig)
	require.Contains(t, transport.TLSClientConfig.NextProtos, "h2")
}
