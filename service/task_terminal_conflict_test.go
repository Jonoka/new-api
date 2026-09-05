package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestAsyncTaskTerminalResultRetriesChangedNonterminalStatus(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	task, _ := handoffWalletTask(t, 500)
	task.Status = model.TaskStatusSuccess
	result, err := FinalizeTaskAccounting(context.Background(), task, model.TaskStatusQueued, 150, "accepted provider result")
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.EqualValues(t, model.TaskStatusSuccess, task.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 9850, user.Quota)
	require.Equal(t, 150, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	canonical, err := model.GetTaskByRowID(task.ID)
	require.NoError(t, err)
	require.EqualValues(t, model.TaskStatusSuccess, canonical.Status)
	require.Equal(t, 150, canonical.Quota)
}

func TestTaskBillingFailureDoesNotPenalizeSuccessfulUpstream(t *testing.T) {
	err := types.NewErrorWithStatusCode(errors.New("database write failed"), types.ErrorCodeUpdateDataError, http.StatusInternalServerError)
	outcome, owner, stage, eligible := classifyChannelMetricAttempt(groupBillingContext(t), &relaycommon.RelayInfo{}, err, true)
	require.Equal(t, channelmetrics.OutcomeLocalError, outcome)
	require.Equal(t, channelmetrics.FailureOwnerGateway, owner)
	require.Equal(t, channelmetrics.ErrorStageSettlement, stage)
	require.False(t, eligible)
}
