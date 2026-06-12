package oauth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
)

// oauthHTTPClient is the shared HTTP client used by custom (generic) OAuth
// providers.
//
// Two adaptations are layered here to survive aggressive upstream WAFs
// (observed concretely with WAFPRO on yaohuo.me, which silently RSTs Go's
// default net/http connections, surfacing as `Post ...: EOF` in logs):
//
//  1. TLS ClientHello is forged via uTLS to look like Chrome — Go's default
//     crypto/tls fingerprint (JA3) is distinct from any real browser and
//     gets flagged before any HTTP byte is sent.
//  2. A full browser-style header preset is layered on top (applied by
//     callers via applyBrowserHeaders) so the request also passes the
//     HTTP-layer signature checks.
var oauthHTTPClient = newOAuthHTTPClient()

func newOAuthHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// DialTLSContext takes over TLS so we can drive uTLS instead of
		// crypto/tls. ForceAttemptHTTP2 is intentionally LEFT OFF — when a
		// custom DialTLS is set, Go won't auto-enable HTTP/2 even if the
		// ALPN negotiates "h2", which keeps us on HTTP/1.1 and avoids
		// HTTP/2-layer fingerprints differing from Chrome.
		DialTLSContext: dialUTLSChrome(dialer),
	}

	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
	}
}

// dialUTLSChrome returns a DialTLSContext that performs the TLS handshake
// with a Chrome-flavoured ClientHello via uTLS. The host:port and ServerName
// are derived from `addr`. Errors from the underlying TCP dial or the uTLS
// handshake are surfaced as-is so the transport can decide whether to retry
// the request.
func dialUTLSChrome(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		rawConn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		host, _, splitErr := net.SplitHostPort(addr)
		if splitErr != nil {
			host = addr
		}

		uconn := utls.UClient(rawConn, &utls.Config{
			ServerName: host,
		}, utls.HelloChrome_Auto)

		// Build the Chrome ClientHello once so we can mutate it, then force
		// ALPN to HTTP/1.1 only.
		//
		// Why: Chrome's real ClientHello advertises ALPN ["h2", "http/1.1"],
		// and HelloChrome_Auto faithfully copies that. But our outer
		// *http.Transport only speaks HTTP/1.x, so if the server picks h2
		// (yaohuo does), the Transport sees a raw HTTP/2 SETTINGS frame and
		// fails with "malformed HTTP response". The TLS-level JA3
		// fingerprint is what the WAF actually keys on; ALPN doesn't change
		// it. Setting utls.Config.NextProtos does NOT propagate into a
		// pre-baked spec like HelloChrome_Auto, so we patch the extension
		// list directly.
		if err := uconn.BuildHandshakeState(); err != nil {
			_ = rawConn.Close()
			return nil, err
		}
		for _, ext := range uconn.Extensions {
			if alpn, ok := ext.(*utls.ALPNExtension); ok {
				alpn.AlpnProtocols = []string{"http/1.1"}
			}
		}
		if err := uconn.BuildHandshakeState(); err != nil {
			_ = rawConn.Close()
			return nil, err
		}
		if err := uconn.HandshakeContext(ctx); err != nil {
			_ = rawConn.Close()
			return nil, err
		}
		return uconn, nil
	}
}

// defaultBrowserUserAgent is a stable, recent Chrome-on-Windows UA used for
// upstream OAuth requests so the call profile matches a real browser.
const defaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// applyBrowserHeaders adds a full set of browser-like headers to h.
//
// Why: WAF policies frequently allow-list requests that look like browser
// XHR/fetch calls (User-Agent + Accept-Language + Sec-Ch-Ua + Sec-Fetch-*).
// Server-to-server OAuth calls don't naturally carry these.
//
// refererURL — typically the OAuth endpoint URL itself. Its scheme+host is
// used to derive Referer (root path) and Origin so they match the request's
// own host, which is what a real same-origin browser fetch would send.
//
// Callers should invoke applyBrowserHeaders FIRST and then set the
// request-specific headers (Content-Type, Authorization, Accept, …) — those
// will overwrite the browser-style defaults where they conflict.
func applyBrowserHeaders(h http.Header, refererURL string) {
	if h == nil {
		return
	}
	h.Set("User-Agent", defaultBrowserUserAgent)
	h.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7")
	// Intentionally omit Accept-Encoding — Go's net/http auto-injects "gzip"
	// and transparently decompresses; setting it manually disables that.
	h.Set("Sec-Ch-Ua", `"Chromium";v="126", "Not(A:Brand";v="24", "Google Chrome";v="126"`)
	h.Set("Sec-Ch-Ua-Mobile", "?0")
	h.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("Cache-Control", "no-cache")
	h.Set("Pragma", "no-cache")

	if refererURL != "" {
		if u, err := url.Parse(refererURL); err == nil && u.Scheme != "" && u.Host != "" {
			origin := u.Scheme + "://" + u.Host
			h.Set("Origin", origin)
			h.Set("Referer", origin+"/")
		}
	}
}

// doOAuthRequest issues an HTTP request via oauthHTTPClient and retries once
// if the connection is closed mid-flight (io.EOF / connection reset).
//
// Headers passed in are sent verbatim; callers are responsible for invoking
// applyBrowserHeaders first when targeting WAF-protected upstreams. Body
// bytes are owned by the caller; the helper wraps them in a new bytes.Reader
// for each attempt so the retry can replay the body.
func doOAuthRequest(ctx context.Context, method, urlStr string, headers http.Header, body []byte) (*http.Response, error) {
	buildReq := func() (*http.Request, error) {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, urlStr, reader)
		if err != nil {
			return nil, err
		}
		for k, vs := range headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		return req, nil
	}

	req, err := buildReq()
	if err != nil {
		return nil, err
	}
	res, err := oauthHTTPClient.Do(req)
	if err == nil {
		return res, nil
	}
	if !isRetryableConnError(err) {
		return nil, err
	}

	time.Sleep(200 * time.Millisecond)
	req2, buildErr := buildReq()
	if buildErr != nil {
		return nil, err
	}
	return oauthHTTPClient.Do(req2)
}

// isRetryableConnError reports whether err looks like a transport-level
// connection closure that is worth a single retry.
func isRetryableConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") {
		return true
	}
	return false
}
