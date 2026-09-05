package service

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

const (
	timeoutTestDelayedHeadersPath = "/delayed-headers"
	timeoutTestSlowBodyPath       = "/slow-body"
	timeoutTestSlowUploadPath     = "/slow-upload"
	timeoutTestBoundaryDelay      = 1250 * time.Millisecond
	timeoutTestHeaderDelay        = 2 * time.Second
	timeoutTestRequestDeadline    = 5 * time.Second
)

type responseHeaderTimeoutFactoryClient struct {
	name   string
	client *http.Client
}

type delayedTimeoutTestUpload struct {
	content []byte
	delay   time.Duration
	offset  int
	waited  bool
}

type timeoutTestCancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *timeoutTestCancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func (r *delayedTimeoutTestUpload) Read(p []byte) (int, error) {
	if r.offset < len(r.content) {
		n := copy(p, r.content[r.offset:])
		r.offset += n
		return n, nil
	}
	if !r.waited {
		r.waited = true
		time.Sleep(r.delay)
	}
	return 0, io.EOF
}

func withRelayResponseHeaderTimeoutTestSettings(t *testing.T, seconds int) {
	t.Helper()

	previousHeaderTimeout := common.RelayResponseHeaderTimeout
	previousRelayTimeout := common.RelayTimeout
	previousHTTPClient := httpClient
	previousProtectedClient := ssrfProtectedHTTPClient
	common.RelayResponseHeaderTimeout = seconds
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
}

func newResponseHeaderTimeoutFactoryClients(t *testing.T) []responseHeaderTimeoutFactoryClient {
	t.Helper()

	httpProxy := newClaudeCodeTestHTTPProxy(t)
	t.Cleanup(httpProxy.Close)
	socksProxy := newTimeoutTestSOCKS5Proxy(t)
	t.Cleanup(socksProxy.Close)

	withClaudeCodeInsecureTLS(t)
	withRelayResponseHeaderTimeoutTestSettings(t, 1)
	InitHttpClient()

	httpProxyClient, err := NewProxyHttpClient(httpProxy.URL)
	require.NoError(t, err)
	socksProxyClient, err := NewProxyHttpClient(socksProxy.URL())
	require.NoError(t, err)
	claudeClient, err := NewClaudeCodeTransportHttpClient("")
	require.NoError(t, err)

	clients := []responseHeaderTimeoutFactoryClient{
		{name: "direct", client: GetHttpClient()},
		{name: "http-connect", client: httpProxyClient},
		{name: "socks5", client: socksProxyClient},
		{name: "claude-custom", client: claudeClient},
	}
	t.Cleanup(func() {
		for _, factoryClient := range clients {
			factoryClient.client.CloseIdleConnections()
		}
	})
	return clients
}

func newResponseHeaderTimeoutTLSServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case timeoutTestDelayedHeadersPath:
			time.Sleep(timeoutTestHeaderDelay)
			_, _ = w.Write([]byte("late"))
		case timeoutTestSlowBodyPath:
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			time.Sleep(timeoutTestBoundaryDelay)
			_, _ = w.Write([]byte("complete"))
		case timeoutTestSlowUploadPath:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if string(body) != "request body" {
				http.Error(w, "unexpected upload", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("uploaded"))
		default:
			http.NotFound(w, r)
		}
	})

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{newClaudeCodeTestCertificate(t)},
		NextProtos:   []string{"http/1.1"},
	}
	server.StartTLS()
	return server
}

func doBoundedTimeoutTestRequest(
	t *testing.T,
	client *http.Client,
	method string,
	url string,
	body io.Reader,
) (*http.Response, error, time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), timeoutTestRequestDeadline)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	require.NoError(t, err)

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		cancel()
	} else {
		resp.Body = &timeoutTestCancelBody{ReadCloser: resp.Body, cancel: cancel}
	}
	return resp, err, time.Since(started)
}

type timeoutTestSOCKS5Proxy struct {
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
}

func newTimeoutTestSOCKS5Proxy(t *testing.T) *timeoutTestSOCKS5Proxy {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxy := &timeoutTestSOCKS5Proxy{
		listener: listener,
		conns:    make(map[net.Conn]struct{}),
	}
	proxy.wg.Add(1)
	go proxy.serve()
	return proxy
}

func (p *timeoutTestSOCKS5Proxy) URL() string {
	return "socks5://" + p.listener.Addr().String()
}

func (p *timeoutTestSOCKS5Proxy) Close() {
	_ = p.listener.Close()
	p.mu.Lock()
	for conn := range p.conns {
		_ = conn.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *timeoutTestSOCKS5Proxy) serve() {
	defer p.wg.Done()
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		p.conns[conn] = struct{}{}
		p.mu.Unlock()
		p.wg.Add(1)
		go p.handle(conn)
	}
}

func (p *timeoutTestSOCKS5Proxy) handle(conn net.Conn) {
	defer p.wg.Done()
	defer func() {
		_ = conn.Close()
		p.mu.Lock()
		delete(p.conns, conn)
		p.mu.Unlock()
	}()

	_ = conn.SetDeadline(time.Now().Add(timeoutTestRequestDeadline))
	targetAddress, err := readTimeoutTestSOCKS5Target(conn)
	if err != nil {
		return
	}
	target, err := net.DialTimeout("tcp", targetAddress, timeoutTestRequestDeadline)
	if err != nil {
		_, _ = conn.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer target.Close()
	if _, err := conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(target, conn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, target)
		done <- struct{}{}
	}()
	<-done
	_ = conn.Close()
	_ = target.Close()
	<-done
}

func readTimeoutTestSOCKS5Target(conn net.Conn) (string, error) {
	var greeting [2]byte
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		return "", err
	}
	if greeting[0] != 5 || greeting[1] == 0 {
		return "", fmt.Errorf("invalid SOCKS5 greeting")
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", err
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return "", err
	}

	var request [4]byte
	if _, err := io.ReadFull(conn, request[:]); err != nil {
		return "", err
	}
	if request[0] != 5 || request[1] != 1 {
		return "", fmt.Errorf("unsupported SOCKS5 request")
	}

	var host string
	switch request[3] {
	case 1:
		address := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, address); err != nil {
			return "", err
		}
		host = net.IP(address).String()
	case 3:
		var length [1]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return "", err
		}
		address := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, address); err != nil {
			return "", err
		}
		host = string(address)
	case 4:
		address := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, address); err != nil {
			return "", err
		}
		host = net.IP(address).String()
	default:
		return "", fmt.Errorf("unsupported SOCKS5 address type")
	}

	var portBytes [2]byte
	if _, err := io.ReadFull(conn, portBytes[:]); err != nil {
		return "", err
	}
	port := strconv.Itoa(int(binary.BigEndian.Uint16(portBytes[:])))
	return net.JoinHostPort(host, port), nil
}
