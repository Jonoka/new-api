package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMidjourneyFreeActionsRemainUncharged(t *testing.T) {
	require.False(t, midjourneyConsumesQuota(constant.MjActionInPaint))
	require.False(t, midjourneyConsumesQuota(constant.MjActionCustomZoom))
	require.True(t, midjourneyConsumesQuota(constant.MjActionImagine))
	require.True(t, midjourneyConsumesQuota(constant.MjActionSwapFace))
	require.Zero(t, midjourneyStoredQuota(false, 100))
	require.Equal(t, 100, midjourneyStoredQuota(true, 100))
}

func TestMidjourneyAcceptedResponseCodesRemainCompatible(t *testing.T) {
	for _, code := range []int{1, 21, 22} {
		require.True(t, midjourneySubmitAccepted(http.StatusOK, code, "upstream-id"), "submit code %d", code)
	}
	require.False(t, midjourneySubmitAccepted(http.StatusOK, 23, "upstream-id"))
	require.False(t, midjourneySubmitAccepted(http.StatusBadGateway, 1, "upstream-id"))
	require.False(t, midjourneySubmitAccepted(http.StatusOK, 1, " "))
	require.True(t, midjourneySwapAccepted(http.StatusOK, 1, "upstream-id"))
	require.False(t, midjourneySwapAccepted(http.StatusOK, 21, "upstream-id"))
}

func TestRelayMidjourneySubmitSendsOnlyAdmittedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldSQLite, oldBatch, oldRedis := common.UsingSQLite, common.BatchUpdateEnabled, common.RedisEnabled
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	savedGroupOverrides := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite, common.BatchUpdateEnabled, common.RedisEnabled = oldSQLite, oldBatch, oldRedis
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(savedGroupOverrides))
	})
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared&_pragma=busy_timeout(5000)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Token{}, &model.Channel{}, &model.Midjourney{}, &model.Task{},
		&model.TaskSubmission{}, &model.TaskAccounting{}, &model.TaskAccountingEvent{},
		&model.Log{}, &model.TaskAccountingLogReceipt{},
	))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"mj_imagine":1,"swap_face":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"paid":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))

	quota := common.QuotaFromFloat(common.QuotaPerUnit)
	user := &model.User{Username: fmt.Sprintf("mjh%d", time.Now().UnixNano()%1_000_000_000_000_000), Password: "test", Quota: 2 * quota}
	user.AffCode = user.Username
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: fmt.Sprintf("mj-handler-%d", time.Now().UnixNano()), Name: "mj", RemainQuota: 10 * quota}
	require.NoError(t, db.Create(token).Error)
	channel := &model.Channel{Name: "midjourney-handler"}
	require.NoError(t, db.Create(channel).Error)

	var upstreamCalls atomic.Int32
	var rejectMode atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if rejectMode.Load() {
			_, _ = w.Write([]byte(`{"code":23,"description":"queue full","result":"rejected"}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"code":1,"description":"submitted","result":"mj-%d"}`, call)
	}))
	t.Cleanup(upstream.Close)
	service.InitHttpClient()

	invoke := func(i int) *dto.MidjourneyResponse {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", strings.NewReader(`{"prompt":"test"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		common.SetContextKey(c, common.RequestIdKey, fmt.Sprintf("mj-handler-request-%d", i))
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "paid")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "subscription_only"})
		common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
		common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "paid")
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "mj_imagine")
		common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
		common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
		c.Set("base_url", upstream.URL)
		c.Set("channel_id", channel.Id)
		c.Set("token_name", token.Name)
		c.Set("token_quota", token.RemainQuota)

		info := &relaycommon.RelayInfo{
			UserId: user.Id, UserQuota: user.Quota, TokenId: token.Id, TokenKey: token.Key,
			UsingGroup: "paid", UserGroup: "default", OriginModelName: "mj_imagine",
			RequestId: fmt.Sprintf("mj-handler-request-%d", i), StartTime: time.Now(),
			RelayMode:     relayconstant.RelayModeMidjourneyImagine,
			UserSetting:   dto.UserSetting{BillingPreference: "subscription_only"},
			TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		}
		return RelayMidjourneySubmit(c, info)
	}
	results := make([]*dto.MidjourneyResponse, 3)
	var contenders sync.WaitGroup
	for i := 0; i < len(results); i++ {
		contenders.Add(1)
		go func(i int) {
			defer contenders.Done()
			results[i] = invoke(i)
		}(i)
	}
	contenders.Wait()
	require.EqualValues(t, 2, upstreamCalls.Load())
	rejected := 0
	for _, result := range results {
		if result != nil {
			rejected++
		}
	}
	require.Equal(t, 1, rejected)
	var gotUser model.User
	var gotToken model.Token
	require.NoError(t, db.First(&gotUser, user.Id).Error)
	require.NoError(t, db.First(&gotToken, token.Id).Error)
	require.Zero(t, gotUser.Quota)
	require.Equal(t, 8*quota, gotToken.RemainQuota)
	require.Equal(t, 2*quota, gotToken.UsedQuota)

	var acceptedTasks []model.Task
	require.NoError(t, db.Where("platform = ?", constant.TaskPlatformMidjourney).Find(&acceptedTasks).Error)
	require.Len(t, acceptedTasks, 2)
	for i := range acceptedTasks {
		terminal := dto.MidjourneyDto{MjId: acceptedTasks[i].GetUpstreamTaskID(), Status: string(model.TaskStatusFailure), Progress: "100%", FailReason: "test cleanup"}
		_, err := service.FinalizeMidjourneyTaskAccounting(context.Background(), &acceptedTasks[i], terminal, quota, terminal.FailReason)
		require.NoError(t, err)
	}
	rejectMode.Store(true)
	require.Nil(t, invoke(99))
	require.EqualValues(t, 3, upstreamCalls.Load())
	require.NoError(t, db.First(&gotUser, user.Id).Error)
	require.NoError(t, db.First(&gotToken, token.Id).Error)
	require.Equal(t, 2*quota, gotUser.Quota)
	require.Equal(t, 10*quota, gotToken.RemainQuota)
	require.Zero(t, gotToken.UsedQuota)
	var rejectedTask model.Midjourney
	require.NoError(t, db.Where("code = ?", 23).First(&rejectedTask).Error)
	require.Nil(t, rejectedTask.TaskRowID)
	require.Zero(t, rejectedTask.Quota, "a released provider rejection must not be eligible for a later legacy refund")

	// The handler must stop before provider I/O when the authoritative finite
	// token balance rejects an otherwise funded request.
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"remain_quota": quota / 2, "used_quota": 0,
	}).Error)
	require.NotNil(t, invoke(100))
	require.EqualValues(t, 3, upstreamCalls.Load())
	require.NoError(t, db.First(&gotUser, user.Id).Error)
	require.Equal(t, 2*quota, gotUser.Quota)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"remain_quota": 10 * quota, "used_quota": 0,
	}).Error)

	// Swap uses the same authoritative admission boundary and cannot send when
	// the wallet row no longer has the required amount.
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 0).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/insight-face/swap", strings.NewReader(`{"sourceBase64":"source","targetBase64":"target"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	c.Set("base_url", upstream.URL)
	c.Set("channel_id", channel.Id)
	swapInfo := &relaycommon.RelayInfo{
		UserId: user.Id, UserQuota: 2 * quota, TokenId: token.Id, TokenKey: token.Key,
		UsingGroup: "paid", UserGroup: "default", OriginModelName: "swap_face",
		RequestId: "mj-swap-insufficient", StartTime: time.Now(),
		RelayMode: relayconstant.RelayModeSwapFace, UserSetting: dto.UserSetting{BillingPreference: "subscription_only"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	require.NotNil(t, RelaySwapFace(c, swapInfo))
	require.EqualValues(t, 3, upstreamCalls.Load())
}
