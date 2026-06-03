package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/common"

	"golang.org/x/net/proxy"
)

const claudeCodeTransportHTTP2Protocol = "h2"

var claudeCodeTransportClients = make(map[string]*http.Client)

func newClaudeCodeTLSConfig() *tls.Config {
	var tlsConfig *tls.Config
	if common.TLSInsecureSkipVerify {
		tlsConfig = common.InsecureTLSConfig.Clone()
	} else {
		tlsConfig = &tls.Config{}
	}
	tlsConfig.NextProtos = []string{claudeCodeTransportHTTP2Protocol, "http/1.1"}
	return tlsConfig
}

func newClaudeCodeTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		ForceAttemptHTTP2:   true,
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     newClaudeCodeTLSConfig(),
	}
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
		case "http", "https":
			transport.Proxy = http.ProxyURL(parsedURL)
		case "socks5", "socks5h":
			var auth *proxy.Auth
			if parsedURL.User != nil {
				auth = &proxy.Auth{
					User:     parsedURL.User.Username(),
					Password: "",
				}
				if password, ok := parsedURL.User.Password(); ok {
					auth.Password = password
				}
			}

			dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
			if err != nil {
				return nil, err
			}
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
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
