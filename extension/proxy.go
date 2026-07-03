package extension

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
)

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

type ProxyContext struct {
	UserID         string
	Username       string
	Role           string
	Group          string
	UseAccessToken string
}

func (m *Manager) ProxyHandler(moduleID string, proxyPath string, role int, ctx ProxyContext) (http.Handler, error) {
	module, ok := m.Get(moduleID)
	if !ok {
		return nil, errors.New("module not found")
	}
	if module.Error != "" {
		return nil, errors.New("module manifest is invalid: " + module.Error)
	}
	if !module.Enabled {
		return nil, errors.New("module is disabled")
	}
	if !roleAllowed(role, module.Permissions.Roles) {
		return nil, errors.New("module permission denied")
	}
	target, err := url.Parse(strings.TrimSpace(module.Runtime.BaseURL))
	if err != nil || target == nil || target.Host == "" {
		return nil, errors.New("runtime.base_url is invalid")
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, errors.New("runtime.base_url only supports http or https")
	}

	cleanPath := cleanProxyPath(proxyPath)
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		req.URL.Path = cleanPath
		req.URL.RawPath = ""
		originalDirector(req)
		req.Host = target.Host
		for _, header := range hopByHopHeaders {
			req.Header.Del(header)
		}
		req.Header.Set("X-NewAPI-Module-ID", module.ID)
		req.Header.Set("X-NewAPI-User-ID", ctx.UserID)
		req.Header.Set("X-NewAPI-Username", ctx.Username)
		req.Header.Set("X-NewAPI-User-Role", ctx.Role)
		req.Header.Set("X-NewAPI-User-Group", ctx.Group)
		req.Header.Set("X-NewAPI-Use-Access-Token", ctx.UseAccessToken)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		for _, header := range hopByHopHeaders {
			resp.Header.Del(header)
		}
		return nil
	}
	return proxy, nil
}

func cleanProxyPath(value string) string {
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}
