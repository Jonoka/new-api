package controller

import (
	"net/http"
	"strconv"

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
