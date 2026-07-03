package extension

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

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

func writeManifest(t *testing.T, rootDir, id string, manifest Manifest) {
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
