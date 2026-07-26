package router

import (
	"embed"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// ThemeAssets holds the embedded frontend assets for both themes.
type ThemeAssets struct {
	DefaultBuildFS   embed.FS
	DefaultIndexPage []byte
	ClassicBuildFS   embed.FS
	ClassicIndexPage []byte
}

type currentWebAssetPaths struct {
	defaultIndexJS  string
	defaultIndexCSS string
	classicIndexJS  string
	classicIndexCSS string
}

var (
	indexJSAssetPattern  = regexp.MustCompile(`/assets/index-[^"']+\.js`)
	indexCSSAssetPattern = regexp.MustCompile(`/assets/index-[^"']+\.css`)
)

func SetWebRouter(router *gin.Engine, assets ThemeAssets) {
	defaultFS := common.EmbedFolder(assets.DefaultBuildFS, "web/default/dist")
	classicFS := common.EmbedFolder(assets.ClassicBuildFS, "web/classic/dist")
	themeFS := common.NewThemeAwareFS(defaultFS, classicFS)
	currentAssets := currentWebAssetPaths{
		defaultIndexJS:  findIndexAssetPath(assets.DefaultIndexPage, indexJSAssetPattern),
		defaultIndexCSS: findIndexAssetPath(assets.DefaultIndexPage, indexCSSAssetPattern),
		classicIndexJS:  findIndexAssetPath(assets.ClassicIndexPage, indexJSAssetPattern),
		classicIndexCSS: findIndexAssetPath(assets.ClassicIndexPage, indexCSSAssetPattern),
	}

	registerWebMiddleware(router, themeFS)
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if serveCurrentIndexAssetFallback(c, themeFS, currentAssets) {
			return
		}
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		if common.GetTheme() == "classic" {
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.ClassicIndexPage)
		} else {
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.DefaultIndexPage)
		}
	})
}

func findIndexAssetPath(indexPage []byte, pattern *regexp.Regexp) string {
	return pattern.FindString(string(indexPage))
}

func currentIndexAssetPath(requestPath string, assets currentWebAssetPaths) string {
	classic := common.GetTheme() == "classic"
	switch {
	case strings.HasPrefix(requestPath, "/assets/index-") && strings.HasSuffix(requestPath, ".js"):
		if classic {
			return assets.classicIndexJS
		}
		return assets.defaultIndexJS
	case strings.HasPrefix(requestPath, "/assets/index-") && strings.HasSuffix(requestPath, ".css"):
		if classic {
			return assets.classicIndexCSS
		}
		return assets.defaultIndexCSS
	default:
		return ""
	}
}

func serveCurrentIndexAssetFallback(c *gin.Context, themeFS static.ServeFileSystem, assets currentWebAssetPaths) bool {
	requestPath := c.Request.URL.Path
	currentPath := currentIndexAssetPath(requestPath, assets)
	if currentPath == "" || currentPath == requestPath {
		return false
	}

	file, err := themeFS.Open(currentPath)
	if err != nil {
		return false
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return false
	}

	contentType := "application/octet-stream"
	if strings.HasSuffix(currentPath, ".js") {
		contentType = "text/javascript; charset=utf-8"
	} else if strings.HasSuffix(currentPath, ".css") {
		contentType = "text/css; charset=utf-8"
	}

	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, contentType, data)
	return true
}

func registerWebMiddleware(router *gin.Engine, themeFS static.ServeFileSystem) {
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.Cache())
	router.Use(static.Serve("/", themeFS))
	router.Use(middleware.GlobalWebRateLimitWithAssetChecker(func(request *http.Request) bool {
		if request == nil || request.URL == nil {
			return false
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			return false
		}
		return themeFS.Exists("/", request.URL.Path)
	}))
}
