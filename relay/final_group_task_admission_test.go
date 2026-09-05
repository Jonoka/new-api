package relay

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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayTaskSubmitDoesNotSendUpstreamWhenSelectedGroupAdmissionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldSQLite, oldBatch, oldRedis := common.UsingSQLite, common.BatchUpdateEnabled, common.RedisEnabled
	savedPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		model.DB = oldDB
		common.UsingSQLite, common.BatchUpdateEnabled, common.RedisEnabled = oldSQLite, oldBatch, oldRedis
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"final-group-task-model":1}`))

	user := &model.User{Username: fmt.Sprintf("ta%d", time.Now().UnixNano()%1_000_000_000_000_000), Password: "test-password", Quota: 100}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: fmt.Sprintf("task-admission-token-%d", time.Now().UnixNano()), RemainQuota: 1000000}
	require.NoError(t, db.Create(token).Error)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"must-not-be-returned"}`))
	}))
	t.Cleanup(upstream.Close)

	body := `{"model":"final-group-task-model","prompt":"test","seconds":"4","size":"720x1280"}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("platform", string(constant.TaskPlatformImage))
	c.Set("group", "default")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "final-group-task-model")

	info := &relaycommon.RelayInfo{
		UserId: user.Id, UserQuota: user.Quota, TokenId: token.Id, TokenKey: token.Key,
		OriginModelName: "final-group-task-model", UpstreamModelName: "final-group-task-model", UsingGroup: "default", UserGroup: "default",
		RequestId: fmt.Sprintf("task-admission-request-%d", time.Now().UnixNano()), RequestURLPath: "/v1/videos",
		UserSetting:   dto.UserSetting{BillingPreference: "wallet_only"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	result, taskErr := RelayTaskSubmit(c, info)
	require.Nil(t, result)
	require.NotNil(t, taskErr)
	require.Zero(t, upstreamCalls.Load())
	require.Nil(t, info.Billing)
	require.Equal(t, float64(4), info.PriceData.OtherRatios["seconds"])
	require.Equal(t, float64(1), info.PriceData.GroupRatioInfo.GroupRatio)
}
