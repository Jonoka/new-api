package extension

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func TestResolveRootDirHonorsEnv(t *testing.T) {
	t.Setenv("EXTENSIONS_ROOT", "/custom/extensions")
	t.Setenv("MODULES_ROOT", "/custom/modules")

	if got := resolveRootDir(); got != "/custom/extensions" {
		t.Fatalf("expected EXTENSIONS_ROOT to win, got %q", got)
	}
}

func TestHasInstalledModules(t *testing.T) {
	rootDir := t.TempDir()
	if hasInstalledModules(rootDir) {
		t.Fatal("empty root should not report installed modules")
	}
	writeManifest(t, rootDir, "echo", Manifest{
		ID:      "echo",
		Name:    "Echo",
		Version: "0.1.0",
		Runtime: Runtime{
			BaseURL: "http://127.0.0.1:39001",
		},
	})
	if !hasInstalledModules(rootDir) {
		t.Fatal("root with manifest should report installed modules")
	}
}

func TestManagerScanAndEnableModule(t *testing.T) {
	rootDir := t.TempDir()
	writeManifest(t, rootDir, "echo", Manifest{
		ID:      "echo",
		Name:    "Echo",
		Version: "0.1.0",
		Runtime: Runtime{
			BaseURL: "http://127.0.0.1:39001",
		},
		UI: UIContribution{
			Nav: []NavItem{{Title: "Echo", Page: "index"}},
			Pages: []Page{{
				Key:   "index",
				Path:  "/ui",
				Embed: true,
			}},
		},
		Permissions: PermissionConfig{Roles: []string{"root"}},
	})

	manager := NewManager(rootDir)
	if err := manager.Scan(); err != nil {
		t.Fatalf("scan module: %v", err)
	}

	if modules := manager.List(common.RoleRootUser, true); len(modules) != 1 {
		t.Fatalf("expected one scanned module, got %d", len(modules))
	}
	if modules := manager.List(common.RoleRootUser, false); len(modules) != 0 {
		t.Fatalf("disabled module should not be visible, got %d", len(modules))
	}

	enabled, err := manager.SetEnabled("echo", true)
	if err != nil {
		t.Fatalf("enable module: %v", err)
	}
	if !enabled.Enabled {
		t.Fatal("module should be enabled")
	}

	userModules := manager.List(common.RoleCommonUser, false)
	if len(userModules) != 0 {
		t.Fatalf("root-only module should not be visible to normal user, got %d", len(userModules))
	}
	rootModules := manager.List(common.RoleRootUser, false)
	if len(rootModules) != 1 || rootModules[0].Runtime.Type != RuntimeTypeHTTP {
		t.Fatalf("expected enabled http module, got %#v", rootModules)
	}
}

func TestManagerScanAndEnableStaticModule(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := writeManifest(t, rootDir, "static-demo", Manifest{
		ID:      "static-demo",
		Name:    "Static Demo",
		Version: "0.1.0",
		Runtime: Runtime{
			Type:      RuntimeTypeStatic,
			StaticDir: "public",
		},
		UI: UIContribution{
			Nav: []NavItem{{Title: "Static Demo", Page: "index"}},
			Pages: []Page{{
				Key:   "index",
				Path:  "/",
				Embed: true,
			}},
		},
		Permissions: PermissionConfig{Roles: []string{"root"}},
	})
	publicDir := filepath.Join(moduleDir, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatalf("make static dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("static index"), 0644); err != nil {
		t.Fatalf("write static index: %v", err)
	}

	manager := NewManager(rootDir)
	if err := manager.Scan(); err != nil {
		t.Fatalf("scan static module: %v", err)
	}

	module, err := manager.SetEnabled("static-demo", true)
	if err != nil {
		t.Fatalf("enable static module: %v", err)
	}
	if module.Runtime.Type != RuntimeTypeStatic {
		t.Fatalf("expected static runtime, got %q", module.Runtime.Type)
	}
}

func TestInstallBuiltinModulesInstallsOkxAlipayRate(t *testing.T) {
	rootDir := t.TempDir()
	if err := validateBuiltinModules(); err != nil {
		t.Fatalf("validate builtin modules: %v", err)
	}
	if err := installBuiltinModules(rootDir); err != nil {
		t.Fatalf("install builtin modules: %v", err)
	}

	manager := NewManager(rootDir)
	if err := manager.Scan(); err != nil {
		t.Fatalf("scan builtin modules: %v", err)
	}
	module, ok := manager.Get("okx-alipay-rate")
	if !ok {
		t.Fatal("okx-alipay-rate builtin module was not installed")
	}
	if module.Runtime.Type != RuntimeTypeStatic {
		t.Fatalf("expected static builtin module, got %q", module.Runtime.Type)
	}
	if !regularFileExists(filepath.Join(rootDir, "okx-alipay-rate", "public", "index.html")) {
		t.Fatal("builtin module static entry was not installed")
	}
}

func TestInstallBuiltinModulesRefreshesOlderBuiltinVersion(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := filepath.Join(rootDir, "okx-alipay-rate")
	if err := os.MkdirAll(filepath.Join(moduleDir, "public"), 0755); err != nil {
		t.Fatalf("make old builtin dir: %v", err)
	}
	oldManifest := []byte(`{
		"id":"okx-alipay-rate",
		"name":"OKX 支付宝汇率",
		"version":"0.1.0",
		"runtime":{"type":"static","static_dir":"public"},
		"permissions":{"roles":["root"]}
	}`)
	if err := os.WriteFile(filepath.Join(moduleDir, "manifest.json"), oldManifest, 0644); err != nil {
		t.Fatalf("write old manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "public", "index.html"), []byte("old page"), 0644); err != nil {
		t.Fatalf("write old page: %v", err)
	}

	if err := installBuiltinModules(rootDir); err != nil {
		t.Fatalf("refresh builtin modules: %v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(moduleDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read refreshed manifest: %v", err)
	}
	var manifest Manifest
	if err := common.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("parse refreshed manifest: %v", err)
	}
	if manifest.Version != "0.2.0" {
		t.Fatalf("expected refreshed builtin version 0.2.0, got %q", manifest.Version)
	}
	pageBytes, err := os.ReadFile(filepath.Join(moduleDir, "public", "index.html"))
	if err != nil {
		t.Fatalf("read refreshed page: %v", err)
	}
	if string(pageBytes) == "old page" {
		t.Fatal("old builtin page was not refreshed")
	}
}

func TestStaticProxyServesIndexAndRejectsTraversal(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := writeManifest(t, rootDir, "static-demo", Manifest{
		ID:      "static-demo",
		Name:    "Static Demo",
		Version: "0.1.0",
		Runtime: Runtime{
			Type:      RuntimeTypeStatic,
			StaticDir: "public",
		},
		UI: UIContribution{
			Pages: []Page{{Key: "index", Path: "/", Embed: true}},
		},
		Permissions: PermissionConfig{Roles: []string{"root"}},
	})
	publicDir := filepath.Join(moduleDir, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatalf("make static dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("static index"), 0644); err != nil {
		t.Fatalf("write static index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	manager := NewManager(rootDir)
	if err := manager.Scan(); err != nil {
		t.Fatalf("scan static module: %v", err)
	}
	if _, err := manager.SetEnabled("static-demo", true); err != nil {
		t.Fatalf("enable static module: %v", err)
	}

	handler, err := manager.ProxyHandler("static-demo", `..\secret.txt`, common.RoleRootUser, ProxyContext{})
	if err != nil {
		t.Fatalf("create static proxy handler: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/extensions/static-demo/proxy/../secret.txt", nil)
	request.Header.Set("If-Modified-Since", time.Now().Add(24*time.Hour).UTC().Format(http.TimeFormat))
	request.Header.Set("If-None-Match", `"stale-module"`)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected stale conditional request to return current file, got %d", recorder.Code)
	}
	if body := recorder.Body.String(); body != "static index" {
		t.Fatalf("expected traversal to fall back to index, got %q", body)
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store, no-cache, must-revalidate, private, max-age=0" {
		t.Fatalf("expected static module response to disable caching, got %q", cacheControl)
	}
	if moduleVersion := recorder.Header().Get("X-NewAPI-Module-Version"); moduleVersion != "0.1.0" {
		t.Fatalf("expected static module version header, got %q", moduleVersion)
	}
}

func TestManagerScanReportsInvalidManifest(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := filepath.Join(rootDir, "broken")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("make module dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "manifest.json"), []byte(`{"id":""}`), 0644); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}

	manager := NewManager(rootDir)
	if err := manager.Scan(); err != nil {
		t.Fatalf("scan module: %v", err)
	}

	modules := manager.List(common.RoleRootUser, true)
	if len(modules) != 1 {
		t.Fatalf("expected invalid module to be listed for root, got %d", len(modules))
	}
	if modules[0].Error == "" {
		t.Fatal("invalid module should include error message")
	}
	if _, err := manager.SetEnabled("broken", true); err == nil {
		t.Fatal("invalid module should not be enabled")
	}
}

func TestManagerInstallArchive(t *testing.T) {
	rootDir := t.TempDir()
	archive := buildModuleArchive(t, "", "uploaded")

	manager := NewManager(rootDir)
	module, err := manager.InstallArchive(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("install module archive: %v", err)
	}
	if module.ID != "uploaded" {
		t.Fatalf("expected uploaded module, got %q", module.ID)
	}
	if !regularFileExists(filepath.Join(rootDir, "uploaded", "manifest.json")) {
		t.Fatal("installed manifest was not written")
	}

	modules := manager.List(common.RoleRootUser, true)
	if len(modules) != 1 || modules[0].ID != "uploaded" {
		t.Fatalf("expected installed module in registry, got %#v", modules)
	}
}

func TestManagerInstallArchiveWithTopLevelDirectory(t *testing.T) {
	rootDir := t.TempDir()
	archive := buildModuleArchive(t, "package", "nested")

	manager := NewManager(rootDir)
	module, err := manager.InstallArchive(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("install nested module archive: %v", err)
	}
	if module.ID != "nested" {
		t.Fatalf("expected nested module, got %q", module.ID)
	}
	if !regularFileExists(filepath.Join(rootDir, "nested", "manifest.json")) {
		t.Fatal("nested archive was not installed by manifest id")
	}
}

func TestManagerUninstallModuleRemovesDirectoryAndState(t *testing.T) {
	rootDir := t.TempDir()
	writeManifest(t, rootDir, "uploaded", Manifest{
		ID:      "uploaded",
		Name:    "Uploaded",
		Version: "1.0.0",
		Runtime: Runtime{
			BaseURL: "http://127.0.0.1:39001",
		},
		UI: UIContribution{
			Pages: []Page{{Key: "index", Path: "/ui", Embed: true}},
		},
	})

	manager := NewManager(rootDir)
	if err := manager.Scan(); err != nil {
		t.Fatalf("scan module: %v", err)
	}
	if _, err := manager.SetEnabled("uploaded", true); err != nil {
		t.Fatalf("enable module: %v", err)
	}
	if err := manager.Uninstall("uploaded"); err != nil {
		t.Fatalf("uninstall module: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "uploaded")); !os.IsNotExist(err) {
		t.Fatalf("module directory should be removed, got err=%v", err)
	}
	if modules := manager.List(common.RoleRootUser, true); len(modules) != 0 {
		t.Fatalf("expected no modules after uninstall, got %#v", modules)
	}

	reloaded := NewManager(rootDir)
	if err := reloaded.Scan(); err != nil {
		t.Fatalf("rescan after uninstall: %v", err)
	}
	if len(reloaded.state.Modules) != 0 {
		t.Fatalf("module state should be removed, got %#v", reloaded.state.Modules)
	}
}

func TestManagerInstallArchiveRejectsZipSlip(t *testing.T) {
	rootDir := t.TempDir()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("../manifest.json")
	if err != nil {
		t.Fatalf("create archive entry: %v", err)
	}
	if _, err := file.Write([]byte(`{}`)); err != nil {
		t.Fatalf("write archive entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	manager := NewManager(rootDir)
	if _, err := manager.InstallArchive(bytes.NewReader(buffer.Bytes()), int64(buffer.Len())); err == nil {
		t.Fatal("zip-slip archive should be rejected")
	}
}

func writeManifest(t *testing.T, rootDir, id string, manifest Manifest) string {
	t.Helper()
	moduleDir := filepath.Join(rootDir, id)
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("make module dir: %v", err)
	}
	data, err := common.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "manifest.json"), data, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return moduleDir
}

func buildModuleArchive(t *testing.T, prefix, id string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	manifest := Manifest{
		ID:      id,
		Name:    "Uploaded",
		Version: "1.0.0",
		Runtime: Runtime{
			BaseURL: "http://127.0.0.1:39001",
		},
		UI: UIContribution{
			Nav:   []NavItem{{Title: "Uploaded", Page: "index"}},
			Pages: []Page{{Key: "index", Path: "/ui", Embed: true}},
		},
	}
	data, err := common.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	path := "manifest.json"
	if prefix != "" {
		path = filepath.ToSlash(filepath.Join(prefix, path))
	}
	file, err := writer.Create(path)
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buffer.Bytes()
}
