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

var sensitiveForwardHeaders = []string{
	"Authorization",
	"Cookie",
	"Proxy-Authorization",
}

var (
	errStaticAssetNotFound = errors.New("static extension asset not found")
	errStaticAssetUnsafe   = errors.New("static extension asset path is unsafe")
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
		for _, header := range sensitiveForwardHeaders {
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
	root, err := secureStaticRoot(module)
	if err != nil {
		return nil, err
	}
	cleanPath, err := validateStaticProxyPath(proxyPath)
	if err != nil {
		return nil, err
	}
	if _, err := secureStaticAssetPath(root, cleanPath); err != nil {
		if !errors.Is(err, errStaticAssetNotFound) {
			return nil, err
		}
		cleanPath = "/index.html"
		if _, err := secureStaticAssetPath(root, cleanPath); err != nil {
			return nil, errors.New("static module entry is unavailable")
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 静态模块允许在同一 URL 上热更新或回滚。若浏览器继续复用旧的
		// index.html、app.js 或 app.css，跨版本资源混用会直接破坏页面。
		// 因此这里既禁止存储，也忽略旧文件留下的条件请求，确保每次打开
		// 模块都读取当前已安装版本的完整文件。
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("X-NewAPI-Module-Version", module.Version)
		r.Header.Del("If-Modified-Since")
		r.Header.Del("If-None-Match")
		for _, header := range hopByHopHeaders {
			r.Header.Del(header)
		}
		r.Header.Set("X-NewAPI-Module-ID", module.ID)
		r.Header.Set("X-NewAPI-User-ID", ctx.UserID)
		r.Header.Set("X-NewAPI-Username", ctx.Username)
		r.Header.Set("X-NewAPI-User-Role", ctx.Role)
		r.Header.Set("X-NewAPI-User-Group", ctx.Group)
		r.Header.Set("X-NewAPI-Use-Access-Token", ctx.UseAccessToken)

		targetPath, err := secureStaticAssetPath(root, cleanPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		file, err := os.Open(targetPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, filepath.Base(targetPath), info.ModTime(), file)
	}), nil
}

func secureStaticRoot(module Module) (string, error) {
	if strings.TrimSpace(module.Path) == "" {
		return "", errors.New("static module directory is unavailable")
	}
	modulePath, err := filepath.Abs(module.Path)
	if err != nil {
		return "", errors.New("static module directory is unavailable")
	}
	staticDir := strings.TrimSpace(module.Runtime.StaticDir)
	if staticDir == "" {
		staticDir = DefaultStaticDir
	}
	staticDir = filepath.Clean(filepath.FromSlash(staticDir))
	if filepath.IsAbs(staticDir) || staticDir == "." || staticDir == ".." || strings.HasPrefix(staticDir, ".."+string(filepath.Separator)) || hasDotPathSegment(filepath.ToSlash(staticDir)) {
		return "", errors.New("static module directory is invalid")
	}
	root := filepath.Join(modulePath, staticDir)
	if err := ensurePathInside(modulePath, root); err != nil {
		return "", errors.New("static module directory escapes module path")
	}
	if err := rejectSymlinkComponents(modulePath, root); err != nil {
		if errors.Is(err, errStaticAssetUnsafe) {
			return "", errors.New("static module directory must not contain symbolic links")
		}
		return "", errors.New("static module directory is unavailable")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() {
		return "", errors.New("static module directory is unavailable")
	}
	resolvedModulePath, err := filepath.EvalSymlinks(modulePath)
	if err != nil {
		return "", errors.New("static module directory is unavailable")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("static module directory is unavailable")
	}
	if err := ensurePathInside(resolvedModulePath, resolvedRoot); err != nil {
		return "", errors.New("static module directory escapes module path")
	}
	return resolvedRoot, nil
}

func validateStaticProxyPath(value string) (string, error) {
	if value == "" {
		return "/", nil
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if hasDotPathSegment(value) {
		return "", errors.New("static module request path must not contain dot-prefixed segments")
	}
	if err := validatePagePath(value); err != nil {
		return "", fmt.Errorf("static module request path is invalid: %w", err)
	}
	return value, nil
}

func secureStaticAssetPath(root, requestPath string) (string, error) {
	if hasDotPathSegment(requestPath) {
		return "", fmt.Errorf("%w: dot-prefixed segments are not allowed", errStaticAssetUnsafe)
	}
	if err := validatePagePath(requestPath); err != nil {
		return "", fmt.Errorf("%w: %v", errStaticAssetUnsafe, err)
	}
	if requestPath == "/" {
		requestPath = "/index.html"
	}
	target := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(requestPath, "/")))
	if err := ensurePathInside(root, target); err != nil {
		return "", errStaticAssetUnsafe
	}
	if err := rejectSymlinkComponents(root, target); err != nil {
		return "", err
	}
	targetInfo, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errStaticAssetNotFound
		}
		return "", errors.New("static extension asset is unavailable")
	}
	if !targetInfo.Mode().IsRegular() {
		return "", errStaticAssetUnsafe
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", errors.New("static extension asset is unavailable")
	}
	if err := ensurePathInside(root, resolvedTarget); err != nil {
		return "", errStaticAssetUnsafe
	}
	return resolvedTarget, nil
}

func rejectSymlinkComponents(base, target string) error {
	if err := ensurePathInside(base, target); err != nil {
		return errStaticAssetUnsafe
	}
	baseInfo, err := os.Lstat(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errStaticAssetNotFound
		}
		return errors.New("static extension asset is unavailable")
	}
	if baseInfo.Mode()&os.ModeSymlink != 0 {
		return errStaticAssetUnsafe
	}
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return errStaticAssetUnsafe
	}
	current := base
	if relative == "." {
		return nil
	}
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return errStaticAssetNotFound
			}
			return errors.New("static extension asset is unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errStaticAssetUnsafe
		}
	}
	return nil
}

func hasDotPathSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
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
