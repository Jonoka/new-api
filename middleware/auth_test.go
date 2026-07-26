package middleware

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
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

func newAuthRedisHashTestClient(tokenFields map[string]string, userFields map[string]string) *redis.Client {
	return redis.NewClient(&redis.Options{
		MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go serveAuthRedisHashOnce(serverConn, tokenFields, userFields)
			return clientConn, nil
		},
	})
}

func serveAuthRedisHashOnce(conn net.Conn, tokenFields map[string]string, userFields map[string]string) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	header, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	count, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(header), "*"))
	if err != nil {
		return
	}
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		lengthHeader, readErr := reader.ReadString('\n')
		if readErr != nil {
			return
		}
		length, parseErr := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(lengthHeader), "$"))
		if parseErr != nil || length < 0 {
			return
		}
		payload := make([]byte, length+2)
		if _, readErr = io.ReadFull(reader, payload); readErr != nil {
			return
		}
		args = append(args, string(payload[:length]))
	}

	if len(args) == 0 {
		return
	}
	command := strings.ToLower(args[0])
	if command == "del" {
		_, _ = io.WriteString(conn, ":0\r\n")
		return
	}
	if command != "hgetall" || len(args) < 2 {
		_, _ = io.WriteString(conn, "-ERR unsupported test command\r\n")
		return
	}
	fields := userFields
	if strings.HasPrefix(args[1], "token:") {
		fields = tokenFields
	}
	var response strings.Builder
	fmt.Fprintf(&response, "*%d\r\n", len(fields)*2)
	for key, value := range fields {
		fmt.Fprintf(&response, "$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(value), value)
	}
	_, _ = io.WriteString(conn, response.String())
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

func TestTokenAuthRejectsSkippedInvalidLegacyGroupBeforeDownstream(t *testing.T) {
	db := setupAuthMiddlewareTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.Token{},
		&model.Group{},
		&model.TokenGroupBinding{},
	))
	require.NoError(t, db.Create(&model.Group{
		Code:   "default",
		Name:   "default",
		Ratio:  1,
		Status: model.GroupStatusActive,
	}).Error)
	require.NoError(t, db.Create(&model.User{
		Id:       1256,
		Username: "invalid-token-user",
		Group:    "default",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Quota:    100,
	}).Error)
	const invalidGroup = "1' AND SUBSTRING((SELECT username FROM users WHERE role=1 LIMIT 1)"
	const tokenKey = "invalidtoken1256"
	require.NoError(t, db.Create(&model.Token{
		Id:             1256,
		UserId:         1256,
		Key:            tokenKey,
		Name:           "invalid-legacy-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		Group:          invalidGroup,
		GroupMode:      model.TokenGroupModeInherit,
	}).Error)
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = newAuthRedisHashTestClient(map[string]string{
		"Id":                 "1256",
		"UserId":             "1256",
		"Status":             strconv.Itoa(common.TokenStatusEnabled),
		"Name":               "invalid-legacy-token",
		"ExpiredTime":        "-1",
		"RemainQuota":        "100",
		"UnlimitedQuota":     "true",
		"ModelLimitsEnabled": "false",
		"UsedQuota":          "0",
		"Group":              invalidGroup,
		"GroupMode":          model.TokenGroupModeInherit,
		"GroupRatioLimits":   "",
		"CrossGroupRetry":    "false",
	}, map[string]string{
		"Id":       "1256",
		"Group":    "default",
		"GroupId":  "1",
		"Quota":    "100",
		"Status":   strconv.Itoa(common.UserStatusEnabled),
		"Role":     strconv.Itoa(common.RoleCommonUser),
		"Username": "invalid-token-user",
		"Setting":  "",
		"Email":    "",
	})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB
	})

	previousUsableGroups := setting.UserUsableGroups2JSONString()
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios)
	})

	gin.SetMode(gin.TestMode)
	downstreamCalled := false
	router := gin.New()
	router.GET("/v1/models", TokenAuth(), func(c *gin.Context) {
		downstreamCalled = true
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("Authorization", "Bearer sk-"+tokenKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.False(t, downstreamCalled)
	require.Contains(t, recorder.Body.String(), "无权访问")
}

func TestTokenAuthReturns503ForHistoricalExclusiveGroupConflict(t *testing.T) {
	db := setupAuthMiddlewareTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.Token{},
		&model.Group{},
		&model.TokenGroupBinding{},
	))
	defaultGroup := &model.Group{Code: "default", Name: "默认分组", Ratio: 1, Status: model.GroupStatusActive}
	exclusiveGroup := &model.Group{Code: "hack", Name: "Hack", Ratio: 1, Exclusive: true, Status: model.GroupStatusActive}
	require.NoError(t, db.Create(defaultGroup).Error)
	require.NoError(t, db.Create(exclusiveGroup).Error)
	require.NoError(t, db.Create(&model.User{
		Id: 1357, Username: "exclusive-conflict-user", Group: "default",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 100,
	}).Error)
	const tokenKey = "exclusiveconflict1357"
	token := &model.Token{
		Id: 1357, UserId: 1357, Key: tokenKey, Name: "historical-conflict",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
		Group: "hack,default", GroupMode: model.TokenGroupModeExplicit,
	}
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, db.Create([]model.TokenGroupBinding{
		{TokenId: token.Id, GroupId: exclusiveGroup.Id, Position: 0},
		{TokenId: token.Id, GroupId: defaultGroup.Id, Position: 1},
	}).Error)
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = newAuthRedisHashTestClient(map[string]string{
		"Id":                 "1357",
		"UserId":             "1357",
		"Status":             strconv.Itoa(common.TokenStatusEnabled),
		"Name":               "historical-conflict",
		"ExpiredTime":        "-1",
		"RemainQuota":        "100",
		"UnlimitedQuota":     "true",
		"ModelLimitsEnabled": "false",
		"UsedQuota":          "0",
		"Group":              "hack,default",
		"GroupMode":          model.TokenGroupModeExplicit,
		"GroupRatioLimits":   "",
		"CrossGroupRetry":    "true",
	}, map[string]string{
		"Id":       "1357",
		"Group":    "default",
		"GroupId":  "1",
		"Quota":    "100",
		"Status":   strconv.Itoa(common.UserStatusEnabled),
		"Role":     strconv.Itoa(common.RoleCommonUser),
		"Username": "exclusive-conflict-user",
		"Setting":  "",
		"Email":    "",
	})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	downstreamCalled := false
	router.GET("/v1/models", TokenAuth(), func(c *gin.Context) {
		downstreamCalled = true
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set("Authorization", "Bearer sk-"+tokenKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "当前绑定分组违规")
	require.False(t, downstreamCalled)
}

func TestTokenAuthAllowsVirtualAutoWithoutLegacyUsableKey(t *testing.T) {
	db := setupAuthMiddlewareTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.Token{},
		&model.Group{},
		&model.TokenGroupBinding{},
	))
	require.NoError(t, db.Create(&model.User{
		Id:       1301,
		Username: "auto-token-user",
		Group:    "default",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Quota:    100,
	}).Error)
	const tokenKey = "autotoken1301"
	require.NoError(t, db.Create(&model.Token{
		Id:             1301,
		UserId:         1301,
		Key:            tokenKey,
		Name:           "virtual-auto-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		Group:          model.TokenGroupModeAuto,
		GroupMode:      model.TokenGroupModeAuto,
	}).Error)
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = newAuthRedisHashTestClient(map[string]string{
		"Id":                 "1301",
		"UserId":             "1301",
		"Status":             strconv.Itoa(common.TokenStatusEnabled),
		"Name":               "virtual-auto-token",
		"ExpiredTime":        "-1",
		"RemainQuota":        "100",
		"UnlimitedQuota":     "true",
		"ModelLimitsEnabled": "false",
		"UsedQuota":          "0",
		"Group":              model.TokenGroupModeAuto,
		"GroupMode":          model.TokenGroupModeAuto,
		"GroupRatioLimits":   "",
		"CrossGroupRetry":    "false",
	}, map[string]string{
		"Id":       "1301",
		"Group":    "default",
		"GroupId":  "1",
		"Quota":    "100",
		"Status":   strconv.Itoa(common.UserStatusEnabled),
		"Role":     strconv.Itoa(common.RoleCommonUser),
		"Username": "auto-token-user",
		"Setting":  "",
		"Email":    "",
	})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB
	})

	previousUsableGroups := setting.UserUsableGroups2JSONString()
	previousAutoGroups := setting.AutoGroups2JsonString()
	previousAutoConfig := setting.AutoGroupConfig2JsonString()
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	require.NoError(t, setting.UpdateAutoGroupConfigByJsonString(`{"user_selectable":true,"description":"自动选择"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups)
		_ = setting.UpdateAutoGroupsByJsonString(previousAutoGroups)
		_ = setting.UpdateAutoGroupConfigByJsonString(previousAutoConfig)
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios)
	})

	gin.SetMode(gin.TestMode)
	downstreamCalled := false
	router := gin.New()
	router.GET("/v1/models", TokenAuth(), func(c *gin.Context) {
		downstreamCalled = true
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("Authorization", "Bearer sk-"+tokenKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	require.True(t, downstreamCalled)
}
