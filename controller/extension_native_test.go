package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetExtensionNativeAssetServesExactProtectedResource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := installNativeControllerTestModule(t)

	originalManager := extension.DefaultManager
	extension.DefaultManager = manager
	t.Cleanup(func() {
		extension.DefaultManager = originalManager
	})

	recorder := callNativeAssetController(t, common.RoleRootUser, "entry")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "export default {};", recorder.Body.String())
	require.Equal(t, "text/javascript; charset=utf-8", recorder.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "same-origin", recorder.Header().Get("Cross-Origin-Resource-Policy"))
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))

	forbidden := callNativeAssetController(t, common.RoleAdminUser, "entry")
	require.Equal(t, http.StatusForbidden, forbidden.Code)

	missing := callNativeAssetController(t, common.RoleRootUser, "style-1")
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.NotContains(t, missing.Body.String(), "fallback must not be served")

	_, err := manager.SetEnabled("native-controller", false)
	require.NoError(t, err)
	disabled := callNativeAssetController(t, common.RoleRootUser, "entry")
	require.Equal(t, http.StatusForbidden, disabled.Code)
}

func installNativeControllerTestModule(t *testing.T) *extension.Manager {
	t.Helper()
	manager := extension.NewManager(t.TempDir())
	moduleDir := filepath.Join(manager.RootDir(), "native-controller")
	publicDir := filepath.Join(moduleDir, "public", "native")
	require.NoError(t, os.MkdirAll(publicDir, 0755))
	manifest := extension.Manifest{
		ID:      "native-controller",
		Name:    "Native Controller",
		Version: "1.0.0",
		Runtime: extension.Runtime{Type: extension.RuntimeTypeStatic, StaticDir: "public"},
		UI: extension.UIContribution{Pages: []extension.Page{{
			Key:  "index",
			Path: "/",
			Render: &extension.PageRender{
				Type: extension.RenderTypeNative,
				SDK:  extension.NativeSDKV1,
				Targets: extension.NativeRenderTargets{
					Default: extension.NativeRenderTarget{
						Entry:  "/native/default.mjs",
						Styles: []string{"/native/default.css"},
					},
					Classic: extension.NativeRenderTarget{Entry: "/native/classic.mjs"},
				},
			},
		}}},
		Permissions: extension.PermissionConfig{
			Roles:        []string{"root"},
			Capabilities: []string{extension.CapabilityUINative},
		},
	}
	manifestData, err := common.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "manifest.json"), manifestData, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "public", "index.html"), []byte("fallback must not be served"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "default.mjs"), []byte("export default {};"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "default.css"), []byte(":root {}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "classic.mjs"), []byte("export default {};"), 0644))
	require.NoError(t, manager.Scan())
	_, err = manager.SetEnabled(manifest.ID, true)
	require.NoError(t, err)
	return manager
}

func callNativeAssetController(t *testing.T, role int, asset string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/extensions/native-controller/native/index/default/"+asset, nil)
	ctx.Params = gin.Params{
		{Key: "id", Value: "native-controller"},
		{Key: "pageKey", Value: "index"},
		{Key: "target", Value: "default"},
		{Key: "asset", Value: asset},
	}
	ctx.Set("role", role)
	GetExtensionNativeAsset(ctx)
	return recorder
}
