package extension

import (
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
