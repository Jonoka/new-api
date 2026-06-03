package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/common"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

var claudeCodeTransportClients = make(map[string]*http.Client)

type claudeCodeTLSDialer struct {
	tcpDialer func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (d *claudeCodeTLSDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := d.tcpDialer(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	tlsConn := utls.UClient(conn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: common.TLSInsecureSkipVerify,
	}, utls.HelloCustom)
	if err := tlsConn.ApplyPreset(buildClaudeCodeClientHelloSpec()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("apply tls preset: %w", err)
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("tls handshake failed: %w", err)
	}

	return tlsConn, nil
}

func buildClaudeCodeClientHelloSpec() *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		CipherSuites: []uint16{
			0x1301, 0x1302, 0x1303,
			0xc02b, 0xc02f, 0xc02c, 0xc030,
			0xcca9, 0xcca8,
			0xc009, 0xc013, 0xc00a, 0xc014,
			0x009c, 0x009d,
			0x002f, 0x0035,
		},
		CompressionMethods: []uint8{0},
		Extensions: []utls.TLSExtension{
			&utls.SNIExtension{},
			&utls.GREASEEncryptedClientHelloExtension{},
			&utls.ExtendedMasterSecretExtension{},
			&utls.RenegotiationInfoExtension{},
			&utls.SupportedCurvesExtension{Curves: []utls.CurveID{utls.X25519, utls.CurveP256, utls.CurveP384}},
			&utls.SupportedPointsExtension{SupportedPoints: []uint8{0}},
			&utls.SessionTicketExtension{},
			&utls.ALPNExtension{AlpnProtocols: []string{"http/1.1"}},
			&utls.StatusRequestExtension{},
			&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{
				0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501, 0x0806, 0x0601, 0x0201,
			}},
			&utls.SCTExtension{},
			&utls.KeyShareExtension{KeyShares: []utls.KeyShare{{Group: utls.X25519}}},
			&utls.PSKKeyExchangeModesExtension{Modes: []uint8{uint8(utls.PskModeDHE)}},
			&utls.SupportedVersionsExtension{Versions: []uint16{utls.VersionTLS13, utls.VersionTLS12}},
		},
		TLSVersMax: utls.VersionTLS13,
		TLSVersMin: utls.VersionTLS10,
	}
}

func newClaudeCodeTransport() *http.Transport {
	netDialer := &net.Dialer{}
	tlsDialer := &claudeCodeTLSDialer{
		tcpDialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			proxyURL, err := claudeCodeProxyFromEnvironment(addr)
			if err != nil {
				return nil, err
			}
			return dialClaudeCodeTargetTCP(ctx, network, addr, proxyURL, netDialer.DialContext)
		},
	}
	return &http.Transport{
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		ForceAttemptHTTP2:   false,
		Proxy: func(req *http.Request) (*url.URL, error) {
			if req.URL.Scheme == "https" {
				return nil, nil
			}
			return http.ProxyFromEnvironment(req)
		},
		DialContext:           netDialer.DialContext,
		DialTLSContext:        tlsDialer.DialTLSContext,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func claudeCodeProxyFromEnvironment(targetAddr string) (*url.URL, error) {
	return http.ProxyFromEnvironment(&http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   targetAddr,
		},
	})
}

func newClaudeCodeTransportWithProxy(proxyURL *url.URL) *http.Transport {
	netDialer := &net.Dialer{}
	proxyAwareTLSDialer := &claudeCodeTLSDialer{
		tcpDialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialClaudeCodeTargetTCP(ctx, network, addr, proxyURL, netDialer.DialContext)
		},
	}

	return &http.Transport{
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		ForceAttemptHTTP2:   false,
		Proxy: func(req *http.Request) (*url.URL, error) {
			if req.URL.Scheme == "https" {
				return nil, nil
			}
			return proxyURL, nil
		},
		DialContext:           netDialer.DialContext,
		DialTLSContext:        proxyAwareTLSDialer.DialTLSContext,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func dialClaudeCodeTargetTCP(ctx context.Context, network, targetAddr string, proxyURL *url.URL, directDial func(ctx context.Context, network, addr string) (net.Conn, error)) (net.Conn, error) {
	if proxyURL == nil {
		return directDial(ctx, network, targetAddr)
	}

	switch proxyURL.Scheme {
	case "http", "https":
		return dialClaudeCodeHTTPProxyTunnel(ctx, network, targetAddr, proxyURL, directDial)
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if proxyURL.User != nil {
			auth = &proxy.Auth{
				User:     proxyURL.User.Username(),
				Password: "",
			}
			if password, ok := proxyURL.User.Password(); ok {
				auth.Password = password
			}
		}

		dialer, err := proxy.SOCKS5(network, claudeCodeProxyAddress(proxyURL), auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		return dialer.Dial(network, targetAddr)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", proxyURL.Scheme)
	}
}

func dialClaudeCodeHTTPProxyTunnel(ctx context.Context, network, targetAddr string, proxyURL *url.URL, directDial func(ctx context.Context, network, addr string) (net.Conn, error)) (net.Conn, error) {
	conn, err := directDial(ctx, network, claudeCodeProxyAddress(proxyURL))
	if err != nil {
		return nil, err
	}

	if proxyURL.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         proxyURL.Hostname(),
			InsecureSkipVerify: common.TLSInsecureSkipVerify,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("https proxy tls handshake failed: %w", err)
		}
		conn = tlsConn
	}

	connectReq := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: targetAddr},
		Host:   targetAddr,
		Header: make(http.Header),
	}
	if authHeader := claudeCodeProxyAuthorization(proxyURL); authHeader != "" {
		connectReq.Header.Set("Proxy-Authorization", authHeader)
	}

	if err := connectReq.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write proxy CONNECT: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), connectReq)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read proxy CONNECT response: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy CONNECT failed: %s", resp.Status)
	}

	return conn, nil
}

func claudeCodeProxyAddress(proxyURL *url.URL) string {
	host := proxyURL.Hostname()
	port := proxyURL.Port()
	if port == "" {
		switch proxyURL.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(host, port)
}

func claudeCodeProxyAuthorization(proxyURL *url.URL) string {
	if proxyURL.User == nil {
		return ""
	}

	password, _ := proxyURL.User.Password()
	token := proxyURL.User.Username() + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(token))
}

func newClaudeCodeTransportClient(transport *http.Transport) *http.Client {
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
	if common.RelayTimeout > 0 {
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
	}
	return client
}

// NewClaudeCodeTransportHttpClient 创建 Claude Code Transport 指纹专用客户端。
// 这里保持进程内实现和现有代理链路，不引入外置 sidecar。
func NewClaudeCodeTransportHttpClient(proxyURL string) (*http.Client, error) {
	proxyClientLock.Lock()
	if client, ok := claudeCodeTransportClients[proxyURL]; ok {
		proxyClientLock.Unlock()
		return client, nil
	}
	proxyClientLock.Unlock()

	transport := newClaudeCodeTransport()
	if proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}

		switch parsedURL.Scheme {
		case "http", "https", "socks5", "socks5h":
			transport = newClaudeCodeTransportWithProxy(parsedURL)
		default:
			return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", parsedURL.Scheme)
		}
	}

	client := newClaudeCodeTransportClient(transport)
	proxyClientLock.Lock()
	claudeCodeTransportClients[proxyURL] = client
	proxyClientLock.Unlock()
	return client, nil
}
