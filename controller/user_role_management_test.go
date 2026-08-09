package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type userRoleManagementResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Role int `json:"role"`
	} `json:"data"`
}

func setupUserRoleManagementTestDB(t *testing.T) *gorm.DB {
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
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	model.LOG_DB = db

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

	return db
}

func createUserRoleManagementFixture(t *testing.T, db *gorm.DB, id int, role int) *model.User {
	t.Helper()

	user := &model.User{
		Id:          id,
		Username:    fmt.Sprintf("role-user-%d", id),
		Password:    "password123",
		DisplayName: fmt.Sprintf("Role User %d", id),
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     fmt.Sprintf("role-aff-%d", id),
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

func newUserRoleManagementContext(t *testing.T, actorID int, actorRole int, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", actorID)
	ctx.Set("role", actorRole)
	ctx.Set("username", fmt.Sprintf("actor-%d", actorID))
	return ctx, recorder
}

func decodeUserRoleManagementResponse(t *testing.T, recorder *httptest.ResponseRecorder) userRoleManagementResponse {
	t.Helper()

	var response userRoleManagementResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestCreateUserAllowsRootToCreateAnotherRoot(t *testing.T) {
	db := setupUserRoleManagementTestDB(t)
	ctx, recorder := newUserRoleManagementContext(t, 1, common.RoleRootUser, map[string]any{
		"username":     "new-team-root",
		"display_name": "New Team Root",
		"password":     "password123",
		"role":         common.RoleRootUser,
	})

	CreateUser(ctx)

	response := decodeUserRoleManagementResponse(t, recorder)
	require.True(t, response.Success)
	var created model.User
	require.NoError(t, db.First(&created, "username = ?", "new-team-root").Error)
	require.Equal(t, common.RoleRootUser, created.Role)
}

func TestCreateUserRejectsRootRoleFromAdmin(t *testing.T) {
	db := setupUserRoleManagementTestDB(t)
	ctx, recorder := newUserRoleManagementContext(t, 1, common.RoleAdminUser, map[string]any{
		"username":     "forbidden-team-root",
		"display_name": "Forbidden Team Root",
		"password":     "password123",
		"role":         common.RoleRootUser,
	})

	CreateUser(ctx)

	response := decodeUserRoleManagementResponse(t, recorder)
	require.False(t, response.Success)
	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "forbidden-team-root").Count(&count).Error)
	require.Zero(t, count)
}

func TestCreateUserRejectsUnknownRole(t *testing.T) {
	db := setupUserRoleManagementTestDB(t)
	ctx, recorder := newUserRoleManagementContext(t, 1, common.RoleRootUser, map[string]any{
		"username":     "invalid-role-user",
		"display_name": "Invalid Role User",
		"password":     "password123",
		"role":         999,
	})

	CreateUser(ctx)

	response := decodeUserRoleManagementResponse(t, recorder)
	require.False(t, response.Success)
	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "invalid-role-user").Count(&count).Error)
	require.Zero(t, count)
}

func TestManageUserRootRoleLifecycle(t *testing.T) {
	t.Run("root promotes administrator to root", func(t *testing.T) {
		db := setupUserRoleManagementTestDB(t)
		createUserRoleManagementFixture(t, db, 1, common.RoleRootUser)
		target := createUserRoleManagementFixture(t, db, 2, common.RoleAdminUser)
		ctx, recorder := newUserRoleManagementContext(t, 1, common.RoleRootUser, ManageRequest{
			Id:     target.Id,
			Action: "promote_root",
		})

		ManageUser(ctx)

		response := decodeUserRoleManagementResponse(t, recorder)
		require.True(t, response.Success)
		require.Equal(t, common.RoleRootUser, response.Data.Role)
		require.NoError(t, db.First(target, target.Id).Error)
		require.Equal(t, common.RoleRootUser, target.Role)
	})

	t.Run("admin cannot promote user to root", func(t *testing.T) {
		db := setupUserRoleManagementTestDB(t)
		createUserRoleManagementFixture(t, db, 1, common.RoleAdminUser)
		target := createUserRoleManagementFixture(t, db, 2, common.RoleCommonUser)
		ctx, recorder := newUserRoleManagementContext(t, 1, common.RoleAdminUser, ManageRequest{
			Id:     target.Id,
			Action: "promote_root",
		})

		ManageUser(ctx)

		response := decodeUserRoleManagementResponse(t, recorder)
		require.False(t, response.Success)
		require.NoError(t, db.First(target, target.Id).Error)
		require.Equal(t, common.RoleCommonUser, target.Role)
	})

	t.Run("root demotes another root to administrator", func(t *testing.T) {
		db := setupUserRoleManagementTestDB(t)
		createUserRoleManagementFixture(t, db, 1, common.RoleRootUser)
		target := createUserRoleManagementFixture(t, db, 2, common.RoleRootUser)
		ctx, recorder := newUserRoleManagementContext(t, 1, common.RoleRootUser, ManageRequest{
			Id:     target.Id,
			Action: "demote_root",
		})

		ManageUser(ctx)

		response := decodeUserRoleManagementResponse(t, recorder)
		require.True(t, response.Success)
		require.Equal(t, common.RoleAdminUser, response.Data.Role)
		require.NoError(t, db.First(target, target.Id).Error)
		require.Equal(t, common.RoleAdminUser, target.Role)
	})

	t.Run("root cannot demote itself", func(t *testing.T) {
		db := setupUserRoleManagementTestDB(t)
		actor := createUserRoleManagementFixture(t, db, 1, common.RoleRootUser)
		ctx, recorder := newUserRoleManagementContext(t, actor.Id, common.RoleRootUser, ManageRequest{
			Id:     actor.Id,
			Action: "demote_root",
		})

		ManageUser(ctx)

		response := decodeUserRoleManagementResponse(t, recorder)
		require.False(t, response.Success)
		require.NoError(t, db.First(actor, actor.Id).Error)
		require.Equal(t, common.RoleRootUser, actor.Role)
	})
}
