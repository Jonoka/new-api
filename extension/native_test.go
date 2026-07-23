package extension

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestManifestValidatesNativeRender(t *testing.T) {
	manifest := validNativeManifest()
	require.NoError(t, manifest.Validate())
	require.NotNil(t, (Module{Manifest: manifest}).Public(false).UI.Pages[0].Render)

	manifest.Permissions.Capabilities = append(
		manifest.Permissions.Capabilities,
		CapabilityNotificationEventsPublish,
	)
	manifest.Notifications = NotificationContribution{Events: []NotificationEventContribution{{
		ID:              "ready",
		Label:           "就绪",
		DefaultTemplate: "{{mention}} 模块已就绪",
	}}}
	require.NoError(t, manifest.Validate())
}

func TestManifestRejectsInvalidNativeRender(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*Manifest)
	}{
		{
			name: "http runtime",
			apply: func(manifest *Manifest) {
				manifest.Runtime = Runtime{Type: RuntimeTypeHTTP, BaseURL: "http://127.0.0.1:39001"}
			},
		},
		{
			name: "missing capability",
			apply: func(manifest *Manifest) {
				manifest.Permissions.Capabilities = nil
			},
		},
		{
			name: "unsupported sdk",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.SDK = "v2"
			},
		},
		{
			name: "unsupported render type",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Type = "iframe"
			},
		},
		{
			name: "page key contains slash",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Key = "nested/page"
			},
		},
		{
			name: "missing default target",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Default = NativeRenderTarget{}
			},
		},
		{
			name: "missing classic target",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Classic = NativeRenderTarget{}
			},
		},
		{
			name: "relative entry",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Default.Entry = "native/default.mjs"
			},
		},
		{
			name: "entry query",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Default.Entry = "/native/default.mjs?v=1"
			},
		},
		{
			name: "entry fragment",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Default.Entry = "/native/default.mjs#main"
			},
		},
		{
			name: "entry traversal",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Default.Entry = "/native/../secret.mjs"
			},
		},
		{
			name: "entry volume",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Default.Entry = "/C:/secret.mjs"
			},
		},
		{
			name: "entry extension",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Default.Entry = "/native/default.css"
			},
		},
		{
			name: "style extension",
			apply: func(manifest *Manifest) {
				manifest.UI.Pages[0].Render.Targets.Default.Styles = []string{"/native/default.js"}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validNativeManifest()
			test.apply(&manifest)
			require.Error(t, manifest.Validate())
		})
	}
}

func TestManagerOpenNativeAssetRejectsStaticDirectorySymlinkOutsideModule(t *testing.T) {
	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	manifest := validNativeManifest()
	moduleDir := writeManifest(t, rootDir, manifest.ID, manifest)
	require.NoError(t, os.MkdirAll(filepath.Join(outsideDir, "native"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "native", "default.mjs"), []byte("export default {};"), 0644))
	if err := os.Symlink(outsideDir, filepath.Join(moduleDir, "public")); err != nil {
		t.Skipf("current environment cannot create directory symlinks: %v", err)
	}

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	module, exists := manager.Get(manifest.ID)
	require.True(t, exists)
	require.Contains(t, module.Error, "static directory escapes module path")
	_, err := manager.SetEnabled(manifest.ID, true)
	require.ErrorContains(t, err, "module manifest is invalid")
}

func TestManagerOpenNativeAssetUsesManifestWhitelist(t *testing.T) {
	rootDir := t.TempDir()
	manifest := validNativeManifest()
	manifest.UI.Pages[0].Render.Targets.Default.Styles = []string{
		"/native/default.css",
		"/native/directory.css",
	}
	manifest.UI.Pages[0].Render.Targets.Classic.Entry = "/native/missing-classic.mjs"
	moduleDir := writeManifest(t, rootDir, manifest.ID, manifest)
	writeNativeModuleAssets(t, moduleDir, manifest, "export const ready = true;", "")
	publicDir := filepath.Join(moduleDir, manifest.Runtime.StaticDir)
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("fallback must not be served"), 0644))

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())

	_, err := manager.OpenNativeAsset(manifest.ID, "index", "default", "entry", common.RoleRootUser)
	require.ErrorContains(t, err, "disabled")
	_, err = manager.SetEnabled(manifest.ID, true)
	require.NoError(t, err)

	_, err = manager.OpenNativeAsset(manifest.ID, "index", "default", "entry", common.RoleAdminUser)
	require.ErrorContains(t, err, "permission denied")

	entry, err := manager.OpenNativeAsset(manifest.ID, "index", "default", "entry", common.RoleRootUser)
	require.NoError(t, err)
	require.Equal(t, "text/javascript; charset=utf-8", entry.ContentType)
	entryBody, err := io.ReadAll(entry.File)
	require.NoError(t, err)
	require.NoError(t, entry.File.Close())
	require.Equal(t, "export const ready = true;", string(entryBody))

	style, err := manager.OpenNativeAsset(manifest.ID, "index", "default", "style-0", common.RoleRootUser)
	require.NoError(t, err)
	require.Equal(t, "text/css; charset=utf-8", style.ContentType)
	require.NoError(t, style.File.Close())

	require.NoError(t, os.Remove(filepath.Join(publicDir, "native", "missing-classic.mjs")))
	_, err = manager.OpenNativeAsset(manifest.ID, "index", "classic", "entry", common.RoleRootUser)
	require.ErrorIs(t, err, ErrNativeAssetNotFound)
	_, err = manager.OpenNativeAsset(manifest.ID, "index", "default", "style-2", common.RoleRootUser)
	require.ErrorIs(t, err, ErrNativeAssetNotFound)
	_, err = manager.OpenNativeAsset(manifest.ID, "index", "default", "style-+0", common.RoleRootUser)
	require.ErrorIs(t, err, ErrNativeAssetNotFound)
	directoryStyle := filepath.Join(publicDir, "native", "directory.css")
	require.NoError(t, os.Remove(directoryStyle))
	require.NoError(t, os.Mkdir(directoryStyle, 0755))
	_, err = manager.OpenNativeAsset(manifest.ID, "index", "default", "style-1", common.RoleRootUser)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrNativeAssetNotFound))
	_, err = manager.OpenNativeAsset(manifest.ID, "index", "mobile", "entry", common.RoleRootUser)
	require.ErrorIs(t, err, ErrNativeAssetNotFound)
	_, err = manager.OpenNativeAsset(manifest.ID, "missing", "default", "entry", common.RoleRootUser)
	require.ErrorIs(t, err, ErrNativeAssetNotFound)
}

func TestManagerScanComputesContentBasedNativeAssetRevision(t *testing.T) {
	rootDir := t.TempDir()
	manifest := validNativeManifest()
	moduleDir := writeManifest(t, rootDir, manifest.ID, manifest)
	writeNativeModuleAssets(t, moduleDir, manifest, "export const value = 'first';", "")

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	first, exists := manager.Get(manifest.ID)
	require.True(t, exists)
	require.Empty(t, first.Error)
	require.Len(t, first.AssetRevision, sha256HexLength)
	require.Equal(t, first.AssetRevision, first.Public(true).AssetRevision)

	entryPath := filepath.Join(moduleDir, "public", "native", "default.mjs")
	require.NoError(t, os.WriteFile(entryPath, []byte("export const value = 'second';"), 0644))
	require.NoError(t, manager.Scan())
	second, exists := manager.Get(manifest.ID)
	require.True(t, exists)
	require.NotEqual(t, first.AssetRevision, second.AssetRevision)
	require.Equal(t, manifest.Version, second.Version)
}

func TestManagerScanComputesNativeAssetRevisionWithRelativeRoot(t *testing.T) {
	rootDir, err := os.MkdirTemp(".", ".native-relative-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(rootDir)
	})

	manifest := validNativeManifest()
	moduleDir := writeManifest(t, rootDir, manifest.ID, manifest)
	writeNativeModuleAssets(t, moduleDir, manifest, "export default {};", "")

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	module, exists := manager.Get(manifest.ID)
	require.True(t, exists)
	require.Empty(t, module.Error)
	require.Len(t, module.AssetRevision, sha256HexLength)
}

func TestManagerScanLeavesNonNativeAssetRevisionEmpty(t *testing.T) {
	rootDir := t.TempDir()
	writeManifest(t, rootDir, "static-demo", Manifest{
		ID:      "static-demo",
		Name:    "Static Demo",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI: UIContribution{Pages: []Page{{
			Key: "index", Path: "/", Embed: true,
		}}},
	})

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	module, exists := manager.Get("static-demo")
	require.True(t, exists)
	require.Empty(t, module.Error)
	require.Empty(t, module.AssetRevision)
	require.Empty(t, module.Public(true).AssetRevision)
}

func TestManagerScanReportsMissingNativeAsset(t *testing.T) {
	rootDir := t.TempDir()
	manifest := validNativeManifest()
	moduleDir := writeManifest(t, rootDir, manifest.ID, manifest)
	missingPath := manifest.UI.Pages[0].Render.Targets.Classic.Entry
	writeNativeModuleAssets(t, moduleDir, manifest, "export default {};", missingPath)

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	module, exists := manager.Get(manifest.ID)
	require.True(t, exists)
	require.Empty(t, module.AssetRevision)
	require.Contains(t, module.Error, missingPath)
	require.Contains(t, module.Error, "file does not exist")
	_, err := manager.SetEnabled(manifest.ID, true)
	require.ErrorContains(t, err, "module manifest is invalid")
}

func TestManagerInstallArchiveRefreshesNativeAssetRevisionAtSameVersion(t *testing.T) {
	rootDir := t.TempDir()
	manifest := validNativeManifest()
	manager := NewManager(rootDir)

	firstArchive := buildNativeModuleArchive(t, manifest, "export const value = 'first';", "")
	first, err := manager.InstallArchive(bytes.NewReader(firstArchive), int64(len(firstArchive)))
	require.NoError(t, err)
	require.Len(t, first.AssetRevision, sha256HexLength)

	secondArchive := buildNativeModuleArchive(t, manifest, "export const value = 'second';", "")
	second, err := manager.InstallArchive(bytes.NewReader(secondArchive), int64(len(secondArchive)))
	require.NoError(t, err)
	require.Equal(t, first.Version, second.Version)
	require.NotEqual(t, first.AssetRevision, second.AssetRevision)
}

func TestManagerInstallArchiveRejectsMissingNativeAssetBeforeReplacement(t *testing.T) {
	rootDir := t.TempDir()
	manifest := validNativeManifest()
	manager := NewManager(rootDir)

	validArchive := buildNativeModuleArchive(t, manifest, "export const value = 'installed';", "")
	installed, err := manager.InstallArchive(bytes.NewReader(validArchive), int64(len(validArchive)))
	require.NoError(t, err)

	missingPath := manifest.UI.Pages[0].Render.Targets.Classic.Entry
	invalidArchive := buildNativeModuleArchive(t, manifest, "export const value = 'replacement';", missingPath)
	_, err = manager.InstallArchive(bytes.NewReader(invalidArchive), int64(len(invalidArchive)))
	require.ErrorContains(t, err, missingPath)

	current, exists := manager.Get(manifest.ID)
	require.True(t, exists)
	require.Equal(t, installed.AssetRevision, current.AssetRevision)
	entry, err := os.ReadFile(filepath.Join(rootDir, manifest.ID, "public", "native", "default.mjs"))
	require.NoError(t, err)
	require.Equal(t, "export const value = 'installed';", string(entry))
}

func validNativeManifest() Manifest {
	return Manifest{
		ID:      "native-demo",
		Name:    "Native Demo",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI: UIContribution{Pages: []Page{{
			Key:   "index",
			Title: "Native Demo",
			Path:  "/",
			Embed: true,
			Render: &PageRender{
				Type: RenderTypeNative,
				SDK:  NativeSDKV1,
				Targets: NativeRenderTargets{
					Default: NativeRenderTarget{
						Entry:  "/native/default.mjs",
						Styles: []string{"/native/default.css"},
					},
					Classic: NativeRenderTarget{
						Entry:  "/native/classic.js",
						Styles: []string{"/native/classic.css"},
					},
				},
			},
		}}},
		Permissions: PermissionConfig{
			Roles:        []string{"root"},
			Capabilities: []string{CapabilityUINative},
		},
	}
}

const sha256HexLength = 64

func writeNativeModuleAssets(t *testing.T, moduleDir string, manifest Manifest, defaultEntry, omittedPath string) {
	t.Helper()
	for assetPath, body := range nativeModuleAssetContents(manifest, defaultEntry) {
		if assetPath == omittedPath {
			continue
		}
		target := filepath.Join(moduleDir, manifest.Runtime.StaticDir, filepath.FromSlash(strings.TrimPrefix(assetPath, "/")))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
		require.NoError(t, os.WriteFile(target, []byte(body), 0644))
	}
}

func buildNativeModuleArchive(t *testing.T, manifest Manifest, defaultEntry, omittedPath string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	manifestBytes, err := common.Marshal(manifest)
	require.NoError(t, err)
	manifestFile, err := writer.Create("manifest.json")
	require.NoError(t, err)
	_, err = manifestFile.Write(manifestBytes)
	require.NoError(t, err)

	contents := nativeModuleAssetContents(manifest, defaultEntry)
	paths := make([]string, 0, len(contents))
	for assetPath := range contents {
		paths = append(paths, assetPath)
	}
	sort.Strings(paths)
	for _, assetPath := range paths {
		if assetPath == omittedPath {
			continue
		}
		archivePath := filepath.ToSlash(filepath.Join(
			manifest.Runtime.StaticDir,
			filepath.FromSlash(strings.TrimPrefix(assetPath, "/")),
		))
		assetFile, err := writer.Create(archivePath)
		require.NoError(t, err)
		_, err = assetFile.Write([]byte(contents[assetPath]))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func nativeModuleAssetContents(manifest Manifest, defaultEntry string) map[string]string {
	contents := map[string]string{}
	for _, page := range manifest.UI.Pages {
		if page.Render == nil || page.Render.Type != RenderTypeNative {
			continue
		}
		for _, target := range []NativeRenderTarget{page.Render.Targets.Default, page.Render.Targets.Classic} {
			contents[target.Entry] = "export default {};"
			for _, style := range target.Styles {
				contents[style] = ":root { color: black; }"
			}
		}
		contents[page.Render.Targets.Default.Entry] = defaultEntry
	}
	return contents
}
