package extension

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestHTTPProxyStripsSensitiveHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Empty(t, request.Header.Get("Cookie"))
		require.Empty(t, request.Header.Get("Authorization"))
		require.Empty(t, request.Header.Get("Proxy-Authorization"))
		require.Equal(t, "orders", request.Header.Get("X-NewAPI-Module-ID"))
		require.Equal(t, "7", request.Header.Get("X-NewAPI-User-ID"))
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	manager := NewManager(t.TempDir())
	writeManifest(t, manager.RootDir(), "orders", Manifest{
		ID:      "orders",
		Name:    "Orders",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeHTTP, BaseURL: backend.URL},
		Permissions: PermissionConfig{
			Roles: []string{"root"},
		},
	})
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("orders", true)
	require.NoError(t, err)

	handler, err := manager.ProxyHandler("orders", "/health", common.RoleRootUser, ProxyContext{
		UserID:   "7",
		Username: "root",
		Role:     "100",
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/api/extensions/orders/proxy/health", nil)
	request.Header.Set("Cookie", "session=secret")
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Proxy-Authorization", "Basic secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestManifestRejectsUnsafePagePaths(t *testing.T) {
	validPaths := []string{"/", "/ui", "/assets/app.min.js", "/.well-known/status", "/运维/概览"}
	for _, pagePath := range validPaths {
		t.Run("valid_"+pagePath, func(t *testing.T) {
			manifest := Manifest{
				ID:      "page-path",
				Name:    "Page Path",
				Version: "1.0.0",
				Runtime: Runtime{Type: RuntimeTypeHTTP, BaseURL: "http://127.0.0.1:39001"},
				UI:      UIContribution{Pages: []Page{{Key: "index", Path: pagePath}}},
			}
			require.NoError(t, manifest.Validate())
		})
	}

	invalidPaths := []string{
		"ui",
		"//example.com/ui",
		"/ui//index.html",
		"/ui/../admin",
		"/ui/./index.html",
		`/ui\..\admin`,
		"/ui?mode=admin",
		"/ui#admin",
		"/%2e%2e/status",
		"/ui/",
		" /ui",
		"/ui\nadmin",
	}
	for _, pagePath := range invalidPaths {
		t.Run("invalid_"+pagePath, func(t *testing.T) {
			manifest := Manifest{
				ID:      "page-path",
				Name:    "Page Path",
				Version: "1.0.0",
				Runtime: Runtime{Type: RuntimeTypeHTTP, BaseURL: "http://127.0.0.1:39001"},
				UI:      UIContribution{Pages: []Page{{Key: "index", Path: pagePath}}},
			}
			require.ErrorContains(t, manifest.Validate(), "ui.pages[index].path is invalid")
		})
	}
}

func TestManifestRejectsDotPathsForStaticRuntime(t *testing.T) {
	manifest := Manifest{
		ID:      "static-dot-path",
		Name:    "Static Dot Path",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI:      UIContribution{Pages: []Page{{Key: "index", Path: "/.well-known/status"}}},
	}
	require.ErrorContains(t, manifest.Validate(), "static paths must not contain dot-prefixed segments")

	manifest.UI.Pages[0].Path = "/index.html"
	manifest.Runtime.StaticDir = ".public"
	require.ErrorContains(t, manifest.Validate(), "runtime.static_dir is invalid")
}

func TestStaticProxyRejectsDotFilesAndSymbolicLinks(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := writeManifest(t, rootDir, "static-security", Manifest{
		ID:      "static-security",
		Name:    "Static Security",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI:      UIContribution{Pages: []Page{{Key: "index", Path: "/"}}},
		Permissions: PermissionConfig{
			Roles: []string{"root"},
		},
	})
	publicDir := filepath.Join(moduleDir, "public")
	require.NoError(t, os.MkdirAll(publicDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(publicDir, "assets"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("static index"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, ".env"), []byte("SECRET=value"), 0644))

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("static-security", true)
	require.NoError(t, err)

	_, err = manager.ProxyHandler("static-security", "/.env", common.RoleRootUser, ProxyContext{})
	require.Error(t, err)
	_, err = manager.ProxyHandler("static-security", "/assets", common.RoleRootUser, ProxyContext{})
	require.Error(t, err)

	t.Run("file symbolic link escapes static root", func(t *testing.T) {
		outsideFile := filepath.Join(t.TempDir(), "secret.txt")
		require.NoError(t, os.WriteFile(outsideFile, []byte("outside secret"), 0644))
		linkPath := filepath.Join(publicDir, "linked-secret.txt")
		if err := os.Symlink(outsideFile, linkPath); err != nil {
			t.Skipf("current environment cannot create file symlinks: %v", err)
		}
		_, err := manager.ProxyHandler("static-security", "/linked-secret.txt", common.RoleRootUser, ProxyContext{})
		require.Error(t, err)
	})
}

func TestStaticProxyRejectsSymbolicStaticRoot(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := writeManifest(t, rootDir, "static-root-link", Manifest{
		ID:      "static-root-link",
		Name:    "Static Root Link",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI:      UIContribution{Pages: []Page{{Key: "index", Path: "/"}}},
		Permissions: PermissionConfig{
			Roles: []string{"root"},
		},
	})
	outsideDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "index.html"), []byte("outside index"), 0644))
	if err := os.Symlink(outsideDir, filepath.Join(moduleDir, "public")); err != nil {
		t.Skipf("current environment cannot create directory symlinks: %v", err)
	}

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("static-root-link", true)
	require.NoError(t, err)
	_, err = manager.ProxyHandler("static-root-link", "/", common.RoleRootUser, ProxyContext{})
	require.Error(t, err)
}
