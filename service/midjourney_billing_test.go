package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func midjourneyBillingFixture(t *testing.T, walletQuota int, tokenQuota int) (*model.User, *model.Token, *model.Channel, *relaycommon.RelayInfo) {
	t.Helper()
	user, token := seedGroupBillingWallet(t, walletQuota, tokenQuota)
	channel := &model.Channel{Name: fmt.Sprintf("midjourney-billing-%d", time.Now().UnixNano())}
	require.NoError(t, model.DB.Create(channel).Error)
	info := groupBillingInfo(user, token)
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: channel.Id}
	info.UsingGroup = "paid"
	info.OriginModelName = "mj_imagine"
	info.UserSetting.BillingPreference = "subscription_only"
	info.PriceData = types.PriceData{UsePrice: true, ModelPrice: 1, Quota: 100, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
	return user, token, channel, info
}

func TestReserveMidjourneyBillingExactAdmissionAndProviderRejectionRelease(t *testing.T) {
	user, token, _, baseInfo := midjourneyBillingFixture(t, 200, 1000)
	c := groupBillingContext(t)
	admitted := 0
	infos := make([]*relaycommon.RelayInfo, 0, 3)
	for i := 0; i < 3; i++ {
		info := *baseInfo
		info.RequestId = fmt.Sprintf("midjourney-admission-%d-%d", time.Now().UnixNano(), i)
		info.Billing = nil
		info.TaskSubmissionID = ""
		info.TaskSubmissionLeaseToken = ""
		if apiErr := ReserveMidjourneyBilling(c, &info, 100); apiErr == nil {
			admitted++
			infos = append(infos, &info)
		} else {
			require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
		}
	}
	require.Equal(t, 2, admitted, "only admitted requests may increment the mock provider send count")
	require.Equal(t, "subscription_only", baseInfo.UserSetting.BillingPreference)
	quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
	require.Zero(t, quota)
	require.Equal(t, 800, remain)
	require.Equal(t, 200, used)

	// A provider rejection releases the same durable owner once.
	infos[0].Billing.Refund(c)
	infos[0].Billing.Refund(c)
	quota, remain, used = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 100, quota)
	require.Equal(t, 900, remain)
	require.Equal(t, 100, used)
	infos[1].Billing.Refund(c)
}

func TestReserveMidjourneyBillingRejectsTokenWithoutWalletDebit(t *testing.T) {
	user, token, _, info := midjourneyBillingFixture(t, 1000, 50)
	c := groupBillingContext(t)
	apiErr := ReserveMidjourneyBilling(c, info, 100)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodePreConsumeTokenQuotaFailed, apiErr.GetErrorCode())
	quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 1000, quota)
	require.Equal(t, 50, remain)
	require.Zero(t, used)
}

func TestMidjourneyHandoffTerminalRefundAndRestartProjection(t *testing.T) {
	user, token, _, info := midjourneyBillingFixture(t, 1000, 1000)
	c := groupBillingContext(t)
	require.Nil(t, ReserveMidjourneyBilling(c, info, 300))

	mj := &model.Midjourney{
		UserId: user.Id, Action: constant.MjActionImagine, MjId: "mj-provider-id",
		Prompt: "test", SubmitTime: time.Now().UnixMilli(), Status: "", Progress: "0%",
		ChannelId: info.ChannelId, Quota: 300,
	}
	task := model.InitTask(constant.TaskPlatformMidjourney, info)
	task.Status = model.TaskStatusSubmitted
	task.Action = constant.MjActionImagine
	task.PrivateData.UpstreamTaskID = mj.MjId
	task.PrivateData.BillingSource = BillingSourceWallet
	task.PrivateData.TokenId = token.Id
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: info.OriginModelName, PerCallBilling: true, GroupRatio: 1}
	require.NoError(t, HandoffMidjourneyBilling(c, info, mj, task, "", 300))
	require.Positive(t, mj.Id)
	require.NotNil(t, mj.TaskRowID)
	require.Equal(t, task.ID, *mj.TaskRowID)
	quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 700, quota)
	require.Equal(t, 700, remain)
	require.Equal(t, 300, used)

	terminal := dto.MidjourneyDto{
		MjId: mj.MjId, Status: string(model.TaskStatusFailure), Progress: "100%",
		FailReason: "provider rejected render", FinishTime: time.Now().UnixMilli(),
	}
	result, err := FinalizeMidjourneyTaskAccounting(context.Background(), task, terminal, 300, terminal.FailReason)
	require.NoError(t, err)
	require.True(t, result.Won)
	result, err = FinalizeMidjourneyTaskAccounting(context.Background(), task, terminal, 300, terminal.FailReason)
	require.NoError(t, err)
	require.False(t, result.Won)
	quota, remain, used = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 1000, quota)
	require.Equal(t, 1000, remain)
	require.Zero(t, used)

	// Simulate restart after canonical accounting and before public projection.
	reloadedTask, err := model.GetTaskByRowID(task.ID)
	require.NoError(t, err)
	projection, err := MidjourneyTaskProjection(reloadedTask)
	require.NoError(t, err)
	ApplyMidjourneyTaskProjection(mj, projection)
	won, err := mj.UpdateWithStatus("")
	require.NoError(t, err)
	require.True(t, won)
	reloadedMJ := model.GetMjByuId(mj.Id)
	require.NotNil(t, reloadedMJ)
	require.Equal(t, string(model.TaskStatusFailure), reloadedMJ.Status)
	require.Equal(t, "100%", reloadedMJ.Progress)
	require.Equal(t, terminal.FailReason, reloadedMJ.FailReason)
}
