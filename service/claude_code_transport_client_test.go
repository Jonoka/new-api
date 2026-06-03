package service

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestClaudeCodeTransportClientUsesClaudeCodeNode24TLSProfile(t *testing.T) {
	ResetProxyClientCache()
	client, err := NewClaudeCodeTransportHttpClient("")
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.False(t, transport.ForceAttemptHTTP2)
	require.NotNil(t, transport.Proxy)
	require.NotNil(t, transport.DialTLSContext)
	require.Nil(t, transport.TLSClientConfig)
}

func TestClaudeCodeTransportClientKeepsHTTPProxy(t *testing.T) {
	ResetProxyClientCache()
	client, err := NewClaudeCodeTransportHttpClient("http://127.0.0.1:18080")
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.Proxy)
	require.NotNil(t, transport.DialTLSContext)
	require.Nil(t, transport.TLSClientConfig)
}

func TestClaudeCodeTransportClientSendsNode24ALPN(t *testing.T) {
	clientHello := make(chan []string, 1)
	server := newClaudeCodeTestTLSServer(t, clientHello)
	defer server.Close()

	withClaudeCodeInsecureTLS(t)
	ResetProxyClientCache()
	client, err := NewClaudeCodeTransportHttpClient("")
	require.NoError(t, err)

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.Equal(t, []string{"http/1.1"}, <-clientHello)
}

func TestClaudeCodeTransportClientSendsNode24ALPNThroughHTTPProxy(t *testing.T) {
	clientHello := make(chan []string, 1)
	server := newClaudeCodeTestTLSServer(t, clientHello)
	defer server.Close()

	proxyServer := newClaudeCodeTestHTTPProxy(t)
	defer proxyServer.Close()

	withClaudeCodeInsecureTLS(t)
	ResetProxyClientCache()
	client, err := NewClaudeCodeTransportHttpClient(proxyServer.URL)
	require.NoError(t, err)

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.Equal(t, []string{"http/1.1"}, <-clientHello)
}

func TestBuildClaudeCodeClientHelloSpec(t *testing.T) {
	spec := buildClaudeCodeClientHelloSpec()
	require.NotEmpty(t, spec.CipherSuites)
	require.NotEmpty(t, spec.Extensions)
	require.Equal(t, uint16(0x0304), spec.TLSVersMax)
	require.Equal(t, uint16(0x0301), spec.TLSVersMin)
}

func newClaudeCodeTestTLSServer(t *testing.T, clientHello chan<- []string) *httptest.Server {
	t.Helper()

	certificate := newClaudeCodeTestCertificate(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		NextProtos:   []string{"http/1.1"},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			clientHello <- append([]string(nil), hello.SupportedProtos...)
			return nil, nil
		},
	}
	server.StartTLS()
	return server
}

func newClaudeCodeTestHTTPProxy(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodConnect, r.Method)

		targetConn, err := net.DialTimeout("tcp", r.Host, 5*time.Second)
		require.NoError(t, err)
		defer targetConn.Close()

		hijacker, ok := w.(http.Hijacker)
		require.True(t, ok)
		clientConn, _, err := hijacker.Hijack()
		require.NoError(t, err)
		defer clientConn.Close()

		_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		require.NoError(t, err)

		errCh := make(chan error, 2)
		go func() {
			_, copyErr := netCopy(targetConn, clientConn)
			errCh <- copyErr
		}()
		go func() {
			_, copyErr := netCopy(clientConn, targetConn)
			errCh <- copyErr
		}()
		<-errCh
	}))
}

func netCopy(dst net.Conn, src net.Conn) (int64, error) {
	return io.Copy(dst, src)
}

func newClaudeCodeTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return certificate
}

func withClaudeCodeInsecureTLS(t *testing.T) {
	t.Helper()

	previous := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = true
	t.Cleanup(func() {
		common.TLSInsecureSkipVerify = previous
		ResetProxyClientCache()
	})
}

func TestClaudeCodeProxyAddress(t *testing.T) {
	tests := []struct {
		rawURL string
		want   string
	}{
		{rawURL: "http://127.0.0.1", want: "127.0.0.1:80"},
		{rawURL: "https://127.0.0.1", want: "127.0.0.1:443"},
		{rawURL: "socks5://127.0.0.1", want: "127.0.0.1:1080"},
		{rawURL: "http://127.0.0.1:18080", want: "127.0.0.1:18080"},
	}

	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.rawURL, "://", "-"), func(t *testing.T) {
			parsedURL, err := url.Parse(tt.rawURL)
			require.NoError(t, err)
			require.Equal(t, tt.want, claudeCodeProxyAddress(parsedURL))
		})
	}
}

func TestClaudeCodeProxyAuthorization(t *testing.T) {
	parsedURL, err := url.Parse("http://user:pass@127.0.0.1:18080")
	require.NoError(t, err)
	require.Equal(t, "Basic dXNlcjpwYXNz", claudeCodeProxyAuthorization(parsedURL))

	parsedURL, err = url.Parse("http://127.0.0.1:18080")
	require.NoError(t, err)
	require.Empty(t, claudeCodeProxyAuthorization(parsedURL))
}

func TestClaudeCodeHTTPProxyTunnelRejectsNon200(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodConnect, r.Method)
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer proxyServer.Close()

	parsedURL, err := url.Parse(proxyServer.URL)
	require.NoError(t, err)
	_, err = dialClaudeCodeHTTPProxyTunnel(t.Context(), "tcp", "example.com:443", parsedURL, (&net.Dialer{}).DialContext)
	require.Error(t, err)
	require.Contains(t, fmt.Sprint(err), "403")
}
