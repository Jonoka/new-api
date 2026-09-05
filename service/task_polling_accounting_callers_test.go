package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

type sunoFailurePollAdaptor struct {
	body []byte
}

func (*sunoFailurePollAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *sunoFailurePollAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(a.body)),
	}, nil
}

func (*sunoFailurePollAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}

func (*sunoFailurePollAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func installSunoFailurePollAdaptor(t *testing.T, response dto.TaskResponse[[]dto.SunoDataResponse]) {
	t.Helper()
	body, err := common.Marshal(response)
	require.NoError(t, err)
	oldFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatformSuno, platform)
		return &sunoFailurePollAdaptor{body: body}
	}
	t.Cleanup(func() { GetTaskAdaptorFunc = oldFactory })
}

func prepareSunoAccountingCallerTask(t *testing.T, chargedQuota int) *model.Task {
	t.Helper()
	task, _ := handoffWalletTask(t, chargedQuota)
	baseURL := "http://suno.invalid"
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", task.ChannelId).Update("base_url", baseURL).Error)
	task.Platform = constant.TaskPlatformSuno
	task.PrivateData.UpstreamTaskID = "suno-upstream-" + common.GetUUID()
	won, err := task.UpdateWithStatus(task.Status)
	require.NoError(t, err)
	require.True(t, won)
	return task
}

func TestAsyncTaskSunoFailurePollUsesDurableAccounting(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	const chargedQuota = 650
	task := prepareSunoAccountingCallerTask(t, chargedQuota)
	providerReason := "provider diagnostic must remain private"
	installSunoFailurePollAdaptor(t, dto.TaskResponse[[]dto.SunoDataResponse]{
		Code: dto.TaskSuccessCode,
		Data: []dto.SunoDataResponse{{
			TaskID: task.GetUpstreamTaskID(), Status: string(model.TaskStatusFailure),
			FailReason: providerReason, FinishTime: common.GetTimestamp(),
		}},
	})

	err := UpdateSunoTasks(context.Background(), map[int][]string{
		task.ChannelId: {task.GetUpstreamTaskID()},
	}, map[string]*model.Task{
		taskPollingKey(task.ChannelId, task.GetUpstreamTaskID()): task,
	})
	require.NoError(t, err)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.EqualValues(t, model.TaskStatusFailure, persisted.Status)
	require.Equal(t, "100%", persisted.Progress)
	require.Zero(t, persisted.Quota)
	require.Equal(t, providerReason, persisted.FailReason)

	var accounting model.TaskAccounting
	require.NoError(t, model.DB.Where("task_row_id = ?", task.ID).First(&accounting).Error)
	require.EqualValues(t, model.TaskStatusFailure, accounting.DecisionStatus)
	require.Zero(t, accounting.DecisionQuota)
	require.True(t, accounting.MoneyApplied)
	require.NotEmpty(t, accounting.DecisionID)

	var user model.User
	var token model.Token
	var channel model.Channel
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.NoError(t, model.DB.First(&token, task.PrivateData.TokenId).Error)
	require.NoError(t, model.DB.First(&channel, task.ChannelId).Error)
	require.Equal(t, 10000, user.Quota)
	require.Zero(t, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Equal(t, 10000, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	require.Zero(t, channel.UsedQuota)

	var events []model.TaskAccountingEvent
	require.NoError(t, model.DB.Where("task_row_id = ?", task.ID).Find(&events).Error)
	require.Len(t, events, 2)
	eventKinds := make(map[string]bool, len(events))
	for _, event := range events {
		eventKinds[event.Kind] = event.Delivered
	}
	require.Equal(t, map[string]bool{"initial": true, "adjustment": true}, eventKinds)

	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ?", task.UserId).Find(&logs).Error)
	require.Len(t, logs, 2)
	logsByType := make(map[int]model.Log, len(logs))
	for _, logEntry := range logs {
		logsByType[logEntry.Type] = logEntry
	}
	require.Equal(t, chargedQuota, logsByType[model.LogTypeConsume].Quota)
	require.Equal(t, chargedQuota, logsByType[model.LogTypeRefund].Quota)
	require.NotContains(t, logsByType[model.LogTypeRefund].Other, providerReason)
}

func TestAsyncTaskSunoPollIgnoresUnknownResponseTaskID(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	const chargedQuota = 475
	task := prepareSunoAccountingCallerTask(t, chargedQuota)
	installSunoFailurePollAdaptor(t, dto.TaskResponse[[]dto.SunoDataResponse]{
		Code: dto.TaskSuccessCode,
		Data: []dto.SunoDataResponse{{
			TaskID: "unknown-suno-task", Status: string(model.TaskStatusFailure), FailReason: "wrong owner",
		}},
	})

	require.NotPanics(t, func() {
		err := UpdateSunoTasks(context.Background(), map[int][]string{
			task.ChannelId: {task.GetUpstreamTaskID()},
		}, map[string]*model.Task{
			taskPollingKey(task.ChannelId, task.GetUpstreamTaskID()): task,
		})
		require.NoError(t, err)
	})

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.EqualValues(t, model.TaskStatusInProgress, persisted.Status)
	require.Equal(t, chargedQuota, persisted.Quota)
	var user model.User
	var token model.Token
	var channel model.Channel
	var accounting model.TaskAccounting
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.NoError(t, model.DB.First(&token, task.PrivateData.TokenId).Error)
	require.NoError(t, model.DB.First(&channel, task.ChannelId).Error)
	require.NoError(t, model.DB.Where("task_row_id = ?", task.ID).First(&accounting).Error)
	require.Equal(t, 10000-chargedQuota, user.Quota)
	require.Equal(t, chargedQuota, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Equal(t, 10000-chargedQuota, token.RemainQuota)
	require.Equal(t, chargedQuota, token.UsedQuota)
	require.EqualValues(t, chargedQuota, channel.UsedQuota)
	require.Equal(t, chargedQuota, accounting.ChargedQuota)
	require.Empty(t, accounting.DecisionID)
	require.False(t, accounting.MoneyApplied)
}
