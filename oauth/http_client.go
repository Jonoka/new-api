package oauth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// oauthHTTPClient is the shared HTTP client used by custom (generic) OAuth
// providers.
//
// We use a dedicated client (instead of http.DefaultTransport) so we can
// control timeouts and force HTTP/2 negotiation. Keep-alive is intentionally
// LEFT ENABLED so the connection profile matches a real browser — some
// upstream WAFs (e.g. WAFPRO on yaohuo.me) flag connections without
// keep-alive as bot-like and silently drop them.
var oauthHTTPClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
	},
}

// defaultBrowserUserAgent is a stable, recent Chrome-on-Windows UA used for
// upstream OAuth requests so the call profile matches a real browser. WAFs
// (e.g. WAFPRO) often flag Go's default `Go-http-client/1.1` UA as bot
// traffic and silently RST the connection (surfaces in Go as "Post ...: EOF").
const defaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// applyBrowserHeaders adds a full set of browser-like headers to h.
//
// Why: WAF policies frequently allow-list requests that look like browser
// XHR/fetch calls (User-Agent + Accept-Language + Sec-Ch-Ua + Sec-Fetch-*).
// Server-to-server OAuth calls don't naturally carry these, which is why the
// raw Go client gets RST'd by some upstreams.
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
	// and transparently decompresses; if we set "br" here, Go disables that
	// auto-handling and we'd get raw brotli bytes back.
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

	// brief backoff before the single retry; rebuild request because the
	// body reader from the first attempt may have been consumed.
	time.Sleep(200 * time.Millisecond)
	req2, buildErr := buildReq()
	if buildErr != nil {
		return nil, err
	}
	return oauthHTTPClient.Do(req2)
}

// isRetryableConnError reports whether err looks like a transport-level
// connection closure that is worth a single retry. We deliberately keep the
// matcher tight — we don't retry on context deadlines, TLS validation
// failures, or HTTP-level errors, since those won't recover on retry.
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
