package extension

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
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
	if module.Runtime.Type == RuntimeTypeStatic {
		return staticHandler(module, proxyPath, ctx)
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

func staticHandler(module Module, proxyPath string, ctx ProxyContext) (http.Handler, error) {
	staticDir := strings.TrimSpace(module.Runtime.StaticDir)
	if staticDir == "" {
		staticDir = DefaultStaticDir
	}
	root := filepath.Join(module.Path, filepath.FromSlash(staticDir))
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("not a directory")
		}
		return nil, fmt.Errorf("static module directory is unavailable: %w", err)
	}

	fileServer := http.FileServer(http.Dir(root))
	cleanPath := cleanProxyPath(proxyPath)
	targetPath, err := staticTargetPath(root, cleanPath)
	if err != nil || !regularFileExists(targetPath) {
		cleanPath = "/"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = cleanPath
		r.URL.RawPath = ""
		for _, header := range hopByHopHeaders {
			r.Header.Del(header)
		}
		r.Header.Set("X-NewAPI-Module-ID", module.ID)
		r.Header.Set("X-NewAPI-User-ID", ctx.UserID)
		r.Header.Set("X-NewAPI-Username", ctx.Username)
		r.Header.Set("X-NewAPI-User-Role", ctx.Role)
		r.Header.Set("X-NewAPI-User-Group", ctx.Group)
		r.Header.Set("X-NewAPI-Use-Access-Token", ctx.UseAccessToken)
		fileServer.ServeHTTP(w, r)
	}), nil
}

func cleanProxyPath(value string) string {
	if value == "" {
		return "/"
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func staticTargetPath(root string, cleanPath string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if cleanPath == "/" {
		cleanPath = "/index.html"
	}
	target := filepath.Join(rootAbs, filepath.FromSlash(strings.TrimPrefix(cleanPath, "/")))
	if err := ensurePathInside(rootAbs, target); err != nil {
		return "", err
	}
	return target, nil
}
