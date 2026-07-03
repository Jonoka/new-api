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

func setupAuthMiddlewareTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSubscription{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
	})

	return db
}

func performAdminAuthRequestWithSessionRole(t *testing.T, sessionRole int) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("auth-middleware-test"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 1001)
		session.Set("username", "session-user")
		session.Set("role", sessionRole)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	router.GET("/admin", AdminAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"role":  c.GetInt("role"),
			"group": c.GetString("group"),
		})
	})

	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.Header.Set("New-Api-User", "1001")
	for _, item := range loginRecorder.Result().Cookies() {
		request.AddCookie(item)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminAuthUsesLatestRoleAfterPromotion(t *testing.T) {
	db := setupAuthMiddlewareTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       1001,
		Username: "session-user",
		Group:    "vip",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
	}).Error)

	recorder := performAdminAuthRequestWithSessionRole(t, common.RoleCommonUser)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"role":10`)
	require.Contains(t, recorder.Body.String(), `"group":"vip"`)
}

func TestAdminAuthUsesLatestRoleAfterDemotion(t *testing.T) {
	db := setupAuthMiddlewareTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       1001,
		Username: "session-user",
		Group:    "default",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}).Error)

	recorder := performAdminAuthRequestWithSessionRole(t, common.RoleAdminUser)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":false`)
}
