package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// ReserveMidjourneyBilling preserves the legacy wallet-only policy while using
// the canonical journal-backed wallet and token reservation transaction.
func ReserveMidjourneyBilling(c *gin.Context, info *relaycommon.RelayInfo, quota int) *types.NewAPIError {
	if info == nil {
		return types.NewError(errors.New("relay info is required"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if info.Billing != nil {
		return types.NewError(errors.New("midjourney billing reservation already exists"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	info.ForcePreConsume = true
	info.EnsureTaskSubmissionIdentity()

	originalPreference := info.UserSetting.BillingPreference
	info.UserSetting.BillingPreference = "wallet_only"
	apiErr := PreConsumeBilling(c, quota, info)
	info.UserSetting.BillingPreference = originalPreference
	if apiErr != nil {
		return apiErr
	}
	session, ok := info.Billing.(*BillingSession)
	if !ok || session.funding == nil || session.funding.Source() != BillingSourceWallet {
		if info.Billing != nil {
			info.Billing.Refund(c)
		}
		return types.NewError(errors.New("midjourney reservation did not select wallet funding"), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	return nil
}

// HandoffMidjourneyBilling first durably links the legacy and internal task rows,
// then transfers the live reservation through the canonical task handoff. A
// linked active submission remains recoverable if the second transaction fails.
func HandoffMidjourneyBilling(c *gin.Context, info *relaycommon.RelayInfo, midjourney *model.Midjourney, task *model.Task, expectedStatus model.TaskStatus, chargedQuota int) error {
	if info == nil || midjourney == nil || task == nil || chargedQuota < 0 {
		return errors.New("invalid midjourney billing handoff")
	}
	if task.UserId != info.UserId || midjourney.UserId != info.UserId {
		return errors.New("midjourney billing handoff user mismatch")
	}
	info.EnsureTaskSubmissionIdentity()
	originalTask := task
	copyTask := *task
	task = &copyTask
	task.Group = info.UsingGroup
	task.ChannelId = info.ChannelId
	task.PrivateData.BillingSource = BillingSourceWallet
	task.PrivateData.SubscriptionId = 0
	task.PrivateData.TokenId = info.TokenId
	if info.IsPlayground || info.SkipTokenQuota {
		task.PrivateData.TokenId = 0
	}
	linkContext := context.Background()
	if c != nil && c.Request != nil {
		linkContext = c.Request.Context()
	}
	if err := model.PersistMidjourneySubmissionLink(linkContext, info.TaskSubmissionID, info.TaskSubmissionLeaseToken, midjourney, task); err != nil {
		return err
	}
	info.TaskSubmissionTaskRowID = task.ID
	if expectedStatus == "" {
		expectedStatus = task.Status
	}
	if err := HandoffTaskBilling(c, info, task, expectedStatus, chargedQuota); err != nil {
		return err
	}
	*originalTask = *task
	taskRowID := task.ID
	midjourney.TaskRowID = &taskRowID
	return nil
}

// FinalizeMidjourneyTaskAccounting freezes the first terminal provider result
// and applies its canonical wallet/token adjustment before public projection.
func FinalizeMidjourneyTaskAccounting(ctx context.Context, task *model.Task, terminal dto.MidjourneyDto, successQuota int, reason string) (*model.TaskTerminalDecisionResult, error) {
	if task == nil {
		return nil, errors.New("midjourney accounting task is required")
	}
	expectedStatus := task.Status
	task.Status = model.TaskStatus(terminal.Status)
	task.Progress = terminal.Progress
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		task.Progress = "100%"
	}
	task.StartTime = millisecondsToSeconds(terminal.StartTime)
	task.FinishTime = millisecondsToSeconds(terminal.FinishTime)
	task.FailReason = terminal.FailReason
	task.PrivateData.ResultURL = terminal.ImageUrl
	data, err := common.Marshal(terminal)
	if err != nil {
		return nil, err
	}
	task.Data = data
	finalQuota := successQuota
	if task.Status == model.TaskStatusFailure {
		finalQuota = 0
	}
	return FinalizeTaskAccounting(ctx, task, expectedStatus, finalQuota, reason)
}

func MidjourneyTaskProjection(task *model.Task) (dto.MidjourneyDto, error) {
	var projection dto.MidjourneyDto
	if task == nil || (task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure) {
		return projection, errors.New("terminal midjourney accounting task is required")
	}
	if len(task.Data) > 0 {
		if err := common.Unmarshal(task.Data, &projection); err != nil {
			return projection, err
		}
	}
	projection.Status = string(task.Status)
	projection.Progress = task.Progress
	projection.StartTime = secondsToMilliseconds(task.StartTime)
	projection.FinishTime = secondsToMilliseconds(task.FinishTime)
	projection.FailReason = task.FailReason
	if projection.ImageUrl == "" {
		projection.ImageUrl = task.PrivateData.ResultURL
	}
	return projection, nil
}

func ApplyMidjourneyTaskProjection(task *model.Midjourney, projection dto.MidjourneyDto) {
	if task == nil {
		return
	}
	task.Progress = projection.Progress
	task.Status = projection.Status
	task.FailReason = projection.FailReason
	if projection.StartTime != 0 {
		task.StartTime = projection.StartTime
	}
	if projection.FinishTime != 0 {
		task.FinishTime = projection.FinishTime
	}
	// A released pre-handoff task has no frozen provider DTO. Preserve the
	// legacy row's submission identity and metadata while projecting its
	// canonical released status.
	if projection.MjId == "" {
		if projection.ImageUrl != "" {
			task.ImageUrl = projection.ImageUrl
		}
		return
	}
	task.PromptEn = projection.PromptEn
	task.State = projection.State
	task.SubmitTime = projection.SubmitTime
	task.ImageUrl = projection.ImageUrl
	task.VideoUrl = projection.VideoUrl
	if projection.Properties != nil {
		properties, _ := common.Marshal(projection.Properties)
		task.Properties = string(properties)
	}
	if projection.Buttons != nil {
		buttons, _ := common.Marshal(projection.Buttons)
		task.Buttons = string(buttons)
	}
	if len(projection.VideoUrls) > 0 {
		videoURLs, _ := common.Marshal(projection.VideoUrls)
		task.VideoUrls = string(videoURLs)
	} else {
		task.VideoUrls = ""
	}
}

func millisecondsToSeconds(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return value / 1000
}

func secondsToMilliseconds(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return value * 1000
}
