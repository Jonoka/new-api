package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type privilegedAuthTestResponse struct {
	Success bool `json:"success"`
}

func setupPrivilegedAuthTestDB(t *testing.T, databaseRole int) *model.User {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	originalRedis := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	model.LOG_DB = db

	user := &model.User{
		Id:          42,
		Username:    "team-admin",
		Password:    "password123",
		DisplayName: "Team Admin",
		Role:        databaseRole,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "team-admin-aff-code",
	}
	require.NoError(t, db.Create(user).Error)

	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		common.RedisEnabled = originalRedis
	})

	return user
}

func performPrivilegedAuthTestRequest(t *testing.T, user *model.User, sessionRole int) privilegedAuthTestResponse {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("privileged-auth-role-test"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", user.Username)
		session.Set("role", sessionRole)
		session.Set("id", user.Id)
		session.Set("status", user.Status)
		session.Set("group", user.Group)
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	router.GET("/root", RootAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)

	request := httptest.NewRequest(http.MethodGet, "/root", nil)
	request.Header.Set("New-Api-User", fmt.Sprintf("%d", user.Id))
	for _, sessionCookie := range loginRecorder.Result().Cookies() {
		request.AddCookie(sessionCookie)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var response privilegedAuthTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestRootAuthAllowsDatabasePromotionWithStaleSession(t *testing.T) {
	user := setupPrivilegedAuthTestDB(t, common.RoleRootUser)

	response := performPrivilegedAuthTestRequest(t, user, common.RoleAdminUser)

	require.True(t, response.Success)
}

func TestRootAuthRejectsDatabaseDemotionWithStaleSession(t *testing.T) {
	user := setupPrivilegedAuthTestDB(t, common.RoleAdminUser)

	response := performPrivilegedAuthTestRequest(t, user, common.RoleRootUser)

	require.False(t, response.Success)
}
