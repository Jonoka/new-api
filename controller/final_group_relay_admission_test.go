package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFinalGroupRelayDoesNotSendSelectedPaidGroupBeforeAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldSQLite, oldBatch, oldRedis := common.UsingSQLite, common.BatchUpdateEnabled, common.RedisEnabled
	oldCountToken := constant.CountToken
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		model.DB = oldDB
		common.UsingSQLite, common.BatchUpdateEnabled, common.RedisEnabled = oldSQLite, oldBatch, oldRedis
		constant.CountToken = oldCountToken
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	model.DB = db
	common.UsingSQLite = true
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	constant.CountToken = false
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"final-group-controller-model":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"paid":1}`))

	user := &model.User{Username: fmt.Sprintf("cr%d", time.Now().UnixNano()%1_000_000_000_000_000), Password: "test-password", Quota: 100}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: fmt.Sprintf("controller-token-%d", time.Now().UnixNano()), RemainQuota: 1000000}
	require.NoError(t, db.Create(token).Error)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"must-not-be-returned"}`))
	}))
	t.Cleanup(upstream.Close)

	body := `{"model":"final-group-controller-model","messages":[{"role":"user","content":"test"}]}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, common.RequestIdKey, fmt.Sprintf("controller-request-%d", time.Now().UnixNano()))
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "paid")
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
	common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "paid")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "final-group-controller-model")
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	c.Set("channel_name", "admission-test")
	c.Set("token_quota", token.RemainQuota)

	Relay(c, types.RelayFormatOpenAI)
	require.Zero(t, upstreamCalls.Load())
	require.Equal(t, http.StatusForbidden, recorder.Code)
	var gotUser model.User
	var gotToken model.Token
	require.NoError(t, db.First(&gotUser, user.Id).Error)
	require.NoError(t, db.First(&gotToken, token.Id).Error)
	require.Equal(t, 100, gotUser.Quota)
	require.Equal(t, 1000000, gotToken.RemainQuota)
}
