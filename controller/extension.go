package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/gin-gonic/gin"
)

type setExtensionEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

func ListExtensions(c *gin.Context) {
	role := c.GetInt("role")
	includeDisabled := role >= common.RoleRootUser && c.Query("all") == "true"
	common.ApiSuccess(c, gin.H{
		"root":    extension.DefaultManager.RootDir(),
		"modules": extension.DefaultManager.List(role, includeDisabled),
	})
}

func RefreshExtensions(c *gin.Context) {
	if err := extension.DefaultManager.Scan(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"root":    extension.DefaultManager.RootDir(),
		"modules": extension.DefaultManager.List(c.GetInt("role"), true),
	})
}

func UploadExtension(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ApiError(c, errors.New("module zip file is required"))
		return
	}
	if !strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".zip") {
		common.ApiError(c, errors.New("only .zip module archives are supported"))
		return
	}
	if fileHeader.Size > extension.MaxInstallArchiveBytes {
		common.ApiError(c, errors.New("module zip file is too large"))
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer file.Close()

	readerAt, ok := file.(interface {
		ReadAt(p []byte, off int64) (n int, err error)
	})
	if !ok {
		common.ApiError(c, errors.New("module zip file cannot be read"))
		return
	}

	module, err := extension.DefaultManager.InstallArchive(readerAt, fileHeader.Size)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"root":   extension.DefaultManager.RootDir(),
		"module": module.Public(true),
	})
}

func SetExtensionEnabled(c *gin.Context) {
	var req setExtensionEnabledRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	module, err := extension.DefaultManager.SetEnabled(c.Param("id"), req.Enabled)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, module.Public(true))
}

func UninstallExtension(c *gin.Context) {
	if err := extension.DefaultManager.Uninstall(c.Param("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"root":    extension.DefaultManager.RootDir(),
		"modules": extension.DefaultManager.List(c.GetInt("role"), true),
	})
}

func ProxyExtension(c *gin.Context) {
	proxy, err := extension.DefaultManager.ProxyHandler(
		c.Param("id"),
		c.Param("path"),
		c.GetInt("role"),
		extension.ProxyContext{
			UserID:         strconv.Itoa(c.GetInt("id")),
			Username:       c.GetString("username"),
			Role:           strconv.Itoa(c.GetInt("role")),
			Group:          c.GetString("group"),
			UseAccessToken: strconv.FormatBool(c.GetBool("use_access_token")),
		},
	)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func GetExtensionNativeAsset(c *gin.Context) {
	asset, err := extension.DefaultManager.OpenNativeAsset(
		c.Param("id"),
		c.Param("pageKey"),
		c.Param("target"),
		c.Param("asset"),
		c.GetInt("role"),
	)
	if err != nil {
		status := http.StatusForbidden
		if errors.Is(err, extension.ErrNativeAssetNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	defer asset.File.Close()

	c.Header("Content-Type", asset.ContentType)
	c.Header("Content-Length", strconv.FormatInt(asset.Size, 10))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, asset.File); err != nil {
		_ = c.Error(err)
	}
}

func GetExtensionHostContext(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"user_id":          c.GetInt("id"),
		"username":         c.GetString("username"),
		"role":             c.GetInt("role"),
		"group":            c.GetString("group"),
		"use_access_token": c.GetBool("use_access_token"),
		"version":          common.Version,
	})
}
