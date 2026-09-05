package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAsyncTaskImageWrapperSettlesOrdinarySynchronousResponse(t *testing.T) {
	db := setupFinalGroupRelayDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskAccounting{}, &model.TaskAccountingEvent{}, &model.TaskAccountingLogReceipt{}))
	user, token := seedFinalGroupRelayFunding(t, db, 90, 2000)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"dGVzdA=="}]}`))
	}))
	t.Cleanup(upstream.Close)
	channel := seedFinalGroupRelayChannel(t, db, "synchronous-image", "fg-same", upstream.URL, 1)
	c, recorder := finalGroupRelayContext(t, user, token, channel, "fg-same", "fg-same", "synchronous-image-accounting")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"fg-relay-model","prompt":"test","n":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	RelayImageTaskSubmit(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "b64_json")
	info := c.MustGet("relay_info").(*relaycommon.RelayInfo)
	require.False(t, info.DeferTaskBilling)
	service.CompleteDeferredImageBilling(c, info)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	require.NoError(t, db.First(channel, channel.Id).Error)
	require.Equal(t, 2000-finalGroupBaseQuota, user.Quota)
	require.Equal(t, 2000-finalGroupBaseQuota, token.RemainQuota)
	require.Equal(t, finalGroupBaseQuota, token.UsedQuota)
	require.Equal(t, finalGroupBaseQuota, user.UsedQuota)
	require.EqualValues(t, finalGroupBaseQuota, channel.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	var logs []model.Log
	require.NoError(t, db.Where("request_id = ?", "synchronous-image-accounting").Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, finalGroupBaseQuota, logs[0].Quota)
	var tasks int64
	require.NoError(t, db.Model(&model.Task{}).Count(&tasks).Error)
	require.Zero(t, tasks)
}

func TestAsyncTaskCanvasCompletionPreservesChargeAndAttribution(t *testing.T) {
	db := setupFinalGroupRelayDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskAccounting{}, &model.TaskAccountingEvent{}, &model.TaskAccountingLogReceipt{}))
	user, token := seedFinalGroupRelayFunding(t, db, 91, 2000)
	setting, err := common.Marshal(dto.UserSetting{BillingPreference: "wallet_only", RecordIpLog: true})
	require.NoError(t, err)
	require.NoError(t, db.Model(user).Update("setting", string(setting)).Error)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"dGVzdA=="}]}`))
	}))
	t.Cleanup(upstream.Close)
	channel := seedFinalGroupRelayChannel(t, db, "canvas-image", "fg-same", upstream.URL, 1)
	c, recorder := finalGroupRelayContext(t, user, token, channel, "fg-same", "fg-same", "canvas-image-accounting")
	c.Set("id", user.Id)
	c.Set("token_name", "canvas-token")
	c.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/generations", strings.NewReader(`{"model":"fg-relay-model","prompt":"test","n":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = "198.51.100.17:4040"
	previousStart := startCanvasImageTaskRelay
	t.Cleanup(func() { startCanvasImageTaskRelay = previousStart })
	startCanvasImageTaskRelay = func(request canvasImageTaskRelayRequest) {
		runCanvasImageTaskRelayWithExecutor(request, time.Minute, func(request canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int, *relaycommon.RelayInfo) {
			return executeCanvasImageRelayWithHandler(request, func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIImage) })
		})
	}
	CanvasImageTaskSubmit(c)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	var task model.Task
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&task).Error)
	require.EqualValues(t, model.TaskStatusSuccess, task.Status)
	require.Equal(t, finalGroupBaseQuota, task.Quota)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	require.Equal(t, 2000-finalGroupBaseQuota, user.Quota)
	require.Equal(t, finalGroupBaseQuota, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Equal(t, 2000, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	var logs []model.Log
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, user.Username, logs[0].Username)
	require.Equal(t, "canvas-token", logs[0].TokenName)
	require.Equal(t, "canvas-image-accounting", logs[0].RequestId)
	require.Equal(t, "198.51.100.17", logs[0].Ip)
	require.Equal(t, finalGroupBaseQuota, logs[0].Quota)
}
