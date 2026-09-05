package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const canvasCodeTTL = 60 * time.Second

type canvasAuthorizeRequest struct {
	State string `json:"state"`
	Challenge string `json:"code_challenge"`
	Method string `json:"code_challenge_method"`
	Audience string `json:"audience"`
}

type canvasExchangeRequest struct {
	Code string `json:"code"`
	State string `json:"state"`
	Verifier string `json:"code_verifier"`
	Audience string `json:"audience"`
}

type canvasCodePayload struct {
	UserID int `json:"user_id"`
	State string `json:"state"`
	Challenge string `json:"code_challenge"`
	Audience string `json:"audience"`
}

type canvasIdentity struct {
	ID int `json:"id"`
	Username string `json:"username"`
	DisplayName string `json:"display_name"`
	Role int `json:"role"`
	Status int `json:"status"`
}

func canvasAuthError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"success": false, "message": message})
}

// This middleware is registered before the broad relay CORS middleware.
func CanvasAuthBoundary(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if common.CanvasSSOOrigin == "" {
		canvasAuthError(c, http.StatusNotFound, "canvas SSO is disabled")
		return
	}
	if c.Request.URL.Path == "/canvas/auth/authorize" {
		c.Header("Vary", "Origin")
		if c.GetHeader("Origin") != common.CanvasSSOOrigin {
			canvasAuthError(c, http.StatusForbidden, "canvas origin is not allowed")
			return
		}
		c.Header("Access-Control-Allow-Origin", common.CanvasSSOOrigin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "POST")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			if c.GetHeader("Access-Control-Request-Method") != http.MethodPost {
				canvasAuthError(c, http.StatusForbidden, "method is not allowed")
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
	}
	c.Next()
}

func readCanvasAuthJSON(c *gin.Context, target any) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		canvasAuthError(c, http.StatusBadRequest, "JSON body is required")
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 2048))
	if err != nil || common.Unmarshal(body, target) != nil {
		canvasAuthError(c, http.StatusBadRequest, "invalid authentication request")
		return false
	}
	return true
}

func validCanvasProof(value string, min, max int) bool {
	if len(value) < min || len(value) > max {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.' || ch == '~') {
			return false
		}
	}
	return true
}

func canvasCodeKey(code string) string {
	digest := sha256.Sum256([]byte(code))
	return "canvas:sso:code:" + hex.EncodeToString(digest[:])
}

func loadCanvasIdentity(ctx context.Context, id int) (*canvasIdentity, error) {
	var user model.User
	// Do not use GetUserCache: revoked access and role changes must be visible here.
	err := model.DB.WithContext(ctx).Select("id", "username", "display_name", "role", "status").First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &canvasIdentity{user.Id, user.Username, user.DisplayName, user.Role, user.Status}, nil
}

func validCanvasIdentity(user *canvasIdentity) bool {
	return user.ID > 0 && user.Username != "" && user.Status == common.UserStatusEnabled &&
		(user.Role == common.RoleCommonUser || user.Role == common.RoleAdminUser || user.Role == common.RoleRootUser)
}

func CanvasAuthorize(c *gin.Context) {
	var req canvasAuthorizeRequest
	if !readCanvasAuthJSON(c, &req) {
		return
	}
	challenge, err := base64.RawURLEncoding.Strict().DecodeString(req.Challenge)
	if !validCanvasProof(req.State, 32, 128) || err != nil || len(challenge) != sha256.Size ||
		req.Method != "S256" || req.Audience != common.CanvasSSOOrigin {
		canvasAuthError(c, http.StatusBadRequest, "invalid authentication request")
		return
	}
	if common.RDB == nil || !common.RedisEnabled {
		canvasAuthError(c, http.StatusServiceUnavailable, "canvas authentication is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	user, err := loadCanvasIdentity(ctx, c.GetInt("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !validCanvasIdentity(user) {
		canvasAuthError(c, http.StatusForbidden, "user is not allowed")
		return
	}
	if err != nil {
		canvasAuthError(c, http.StatusServiceUnavailable, "canvas authentication is unavailable")
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		canvasAuthError(c, http.StatusServiceUnavailable, "canvas authentication is unavailable")
		return
	}
	code := base64.RawURLEncoding.EncodeToString(random[:])
	payload, err := common.Marshal(canvasCodePayload{user.ID, req.State, req.Challenge, req.Audience})
	if err != nil {
		canvasAuthError(c, http.StatusServiceUnavailable, "canvas authentication is unavailable")
		return
	}
	stored, err := common.RDB.SetNX(ctx, canvasCodeKey(code), payload, canvasCodeTTL).Result()
	if err != nil || !stored {
		canvasAuthError(c, http.StatusServiceUnavailable, "canvas authentication is unavailable")
		return
	}
	common.ApiSuccess(c, gin.H{"code": code, "state": req.State, "expires_in": 60})
}

func CanvasExchange(c *gin.Context) {
	var req canvasExchangeRequest
	if !readCanvasAuthJSON(c, &req) {
		return
	}
	if !validCanvasProof(req.Code, 43, 43) || !validCanvasProof(req.State, 32, 128) ||
		!validCanvasProof(req.Verifier, 43, 128) || req.Audience != common.CanvasSSOOrigin {
		canvasAuthError(c, http.StatusBadRequest, "invalid authentication request")
		return
	}
	if common.RDB == nil || !common.RedisEnabled {
		canvasAuthError(c, http.StatusServiceUnavailable, "canvas authentication is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	// Invalid proofs burn the code too; never split GET and DEL or retry consumption.
	raw, err := common.RDB.GetDel(ctx, canvasCodeKey(req.Code)).Bytes()
	if errors.Is(err, redis.Nil) {
		canvasAuthError(c, http.StatusBadRequest, "authentication code is invalid or expired")
		return
	}
	if err != nil {
		canvasAuthError(c, http.StatusServiceUnavailable, "canvas authentication is unavailable")
		return
	}
	var payload canvasCodePayload
	digest := sha256.Sum256([]byte(req.Verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	if common.Unmarshal(raw, &payload) != nil || payload.UserID <= 0 || payload.State != req.State ||
		payload.Audience != req.Audience || subtle.ConstantTimeCompare([]byte(payload.Challenge), []byte(challenge)) != 1 {
		canvasAuthError(c, http.StatusBadRequest, "authentication proof is invalid")
		return
	}
	user, err := loadCanvasIdentity(ctx, payload.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !validCanvasIdentity(user) {
		canvasAuthError(c, http.StatusForbidden, "user is not allowed")
		return
	}
	if err != nil {
		canvasAuthError(c, http.StatusServiceUnavailable, "canvas authentication is unavailable")
		return
	}
	common.ApiSuccess(c, user)
}

func validCanvasDestination(destination string) bool {
	if len(destination) > 1024 || !strings.HasPrefix(destination, "/") || strings.HasPrefix(destination, "//") || strings.Contains(destination, "\\") {
		return false
	}
	decoded, err := url.PathUnescape(destination)
	if err != nil || strings.HasPrefix(decoded, "//") || strings.Contains(decoded, "\\") {
		return false
	}
	for _, ch := range decoded {
		if ch < 32 || ch == 127 {
			return false
		}
	}
	u, err := url.Parse(destination)
	return err == nil && !u.IsAbs() && u.Host == ""
}

// Theme-neutral return target used by Canvas 2, including canvas-only logout.
func CanvasLaunch(c *gin.Context) {
	destination := "/canvas"
	if common.GetTheme() == "classic" {
		destination = "/console/canvas"
	}
	query := url.Values{}
	if c.Query("canvas_resume") == "1" && common.CanvasSSOLaunchEnabled {
		query.Set("canvas_resume", "1")
		if next := c.Query("canvas_next"); validCanvasDestination(next) {
			query.Set("canvas_next", next)
		}
		if group := c.Query("group"); len(group) <= 64 && !strings.ContainsAny(group, "\r\n\x00") {
			query.Set("group", group)
		}
	}
	if len(query) != 0 {
		destination += "?" + query.Encode()
	}
	c.Redirect(http.StatusSeeOther, destination)
}
