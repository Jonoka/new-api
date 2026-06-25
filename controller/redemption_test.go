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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRedemptionControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.Redemption{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func newRedemptionControllerContext(t *testing.T, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	payload, err := common.Marshal(body)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/redemption/", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	return ctx, recorder
}

func TestUpdateRedemptionRejectsMaxRedeemCountOverLimit(t *testing.T) {
	db := setupRedemptionControllerTestDB(t)

	redemption := &model.Redemption{
		UserId:         1,
		Key:            "max-redeem-update-limit",
		Status:         common.RedemptionCodeStatusEnabled,
		Name:           "limit",
		Quota:          100,
		CreatedTime:    common.GetTimestamp(),
		MaxRedeemCount: 1,
	}
	require.NoError(t, redemption.Insert())

	ctx, recorder := newRedemptionControllerContext(t, map[string]any{
		"id":               redemption.Id,
		"name":             "limit",
		"quota":            100,
		"expired_time":     0,
		"max_redeem_count": 100001,
	})

	UpdateRedemption(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "兑换次数不能超过 100000")

	var saved model.Redemption
	require.NoError(t, db.First(&saved, "id = ?", redemption.Id).Error)
	assert.Equal(t, 1, saved.MaxRedeemCount)
}
