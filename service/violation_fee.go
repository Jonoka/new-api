package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ViolationFeeCodePrefix     = "violation_fee."
	CSAMViolationMarker        = "Failed check: SAFETY_CHECK_TYPE"
	ContentViolatesUsageMarker = "Content violates usage guidelines"
	violationFeeStateKey       = "violation_fee_billing_state"
)

type violationFeeBillingState struct {
	submissionID string
	leaseToken   string
	operationID  string
}

func IsViolationFeeCode(code types.ErrorCode) bool {
	return strings.HasPrefix(string(code), ViolationFeeCodePrefix)
}

func HasCSAMViolationMarker(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), CSAMViolationMarker) || strings.Contains(err.Error(), ContentViolatesUsageMarker) {
		return true
	}
	msg := err.ToOpenAIError().Message
	return strings.Contains(msg, CSAMViolationMarker) || strings.Contains(err.Error(), ContentViolatesUsageMarker)
}

func WrapAsViolationFeeGrokCSAM(err *types.NewAPIError) *types.NewAPIError {
	if err == nil {
		return nil
	}
	oai := err.ToOpenAIError()
	oai.Type = string(types.ErrorCodeViolationFeeGrokCSAM)
	oai.Code = string(types.ErrorCodeViolationFeeGrokCSAM)
	return types.WithOpenAIError(oai, err.StatusCode, types.ErrOptionWithSkipRetry())
}

// NormalizeViolationFeeError ensures:
// - if the CSAM marker is present, error.code is set to a stable violation-fee code and skip-retry is enabled.
// - if error.code already has the violation-fee prefix, skip-retry is enabled.
//
// It must be called before retry decision logic.
func NormalizeViolationFeeError(err *types.NewAPIError) *types.NewAPIError {
	if err == nil {
		return nil
	}

	if HasCSAMViolationMarker(err) {
		return WrapAsViolationFeeGrokCSAM(err)
	}

	if IsViolationFeeCode(err.GetErrorCode()) {
		oai := err.ToOpenAIError()
		return types.WithOpenAIError(oai, err.StatusCode, types.ErrOptionWithSkipRetry())
	}

	return err
}

func shouldChargeViolationFee(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if err.GetErrorCode() == types.ErrorCodeViolationFeeGrokCSAM {
		return true
	}
	// In case some callers didn't normalize, keep a safety net.
	return HasCSAMViolationMarker(err)
}

func calcViolationFeeQuota(amount, groupRatio float64) int {
	if amount <= 0 {
		return 0
	}
	if groupRatio <= 0 {
		return 0
	}
	quota := decimal.NewFromFloat(amount).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromFloat(groupRatio)).
		Round(0).
		IntPart()
	if quota <= 0 {
		return 0
	}
	return int(quota)
}

// ChargeViolationFeeIfNeeded charges an additional fee after the normal flow finishes (including refund).
// It uses Grok fee settings as the fee policy.
func ChargeViolationFeeIfNeeded(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, apiErr *types.NewAPIError) bool {
	if ctx == nil || relayInfo == nil || apiErr == nil {
		return false
	}
	//if relayInfo.IsPlayground {
	//	return false
	//}
	if !shouldChargeViolationFee(apiErr) {
		return false
	}

	settings := model_setting.GetGrokSettings()
	if settings == nil || !settings.ViolationDeductionEnabled {
		return false
	}

	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	feeQuota := calcViolationFeeQuota(settings.ViolationDeductionAmount, groupRatio)
	if feeQuota <= 0 {
		return false
	}

	useTimeMs := relayInfo.ElapsedMilliseconds()
	oai := apiErr.ToOpenAIError()

	other := map[string]any{
		"violation_fee":        true,
		"violation_fee_code":   string(types.ErrorCodeViolationFeeGrokCSAM),
		"fee_quota":            feeQuota,
		"base_amount":          settings.ViolationDeductionAmount,
		"group_ratio":          groupRatio,
		"status_code":          apiErr.StatusCode,
		"upstream_error_type":  oai.Type,
		"upstream_error_code":  fmt.Sprintf("%v", oai.Code),
		"violation_fee_marker": CSAMViolationMarker,
		"use_time_ms":          float64(useTimeMs),
	}

	facts := BuildTaskAccountingLogFacts(ctx, relayInfo, feeQuota)
	facts.Content = "Violation fee charged"
	facts.UseTimeSeconds = int(useTimeMs / 1000)
	facts.IsStream = relayInfo.IsStream
	facts.Other = other

	state := getViolationFeeBillingState(ctx)
	source := relayInfo.BillingSource
	if source == "" {
		source = BillingSourceWallet
	}
	req := model.GroupReservationRequest{
		Source: source, UserId: relayInfo.UserId, ModelName: relayInfo.OriginModelName,
		SubscriptionId: relayInfo.SubscriptionId, TokenId: relayInfo.TokenId, TokenKey: relayInfo.TokenKey,
		TokenUnlimited: relayInfo.TokenUnlimited, SkipTokenQuota: relayInfo.IsPlayground || relayInfo.SkipTokenQuota,
		ExpectedReserved: 0, TargetReserved: feeQuota, PostConsume: true,
		SubmissionID: state.submissionID, SubmissionLeaseToken: state.leaseToken,
		SubmissionOperationID: state.operationID, SubmissionFinalState: model.TaskSubmissionStateSettled,
	}
	result, err := model.WithReconciledGroupReservation(req, func(tx *gorm.DB, _ *model.GroupReservationResult) error {
		return model.CompleteViolationFeeSubmissionTx(tx, req.SubmissionID, facts)
	})
	replayed := false
	if err != nil {
		resolveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result, err = model.ResolveViolationFeeSettlement(resolveCtx, req)
		cancel()
		replayed = err == nil
	}
	if err != nil || result == nil {
		if err == nil {
			err = fmt.Errorf("violation fee reservation result is missing")
		}
		logger.LogError(ctx, fmt.Sprintf("failed to charge violation fee: %s", err.Error()))
		return false
	}
	if source == BillingSourceSubscription && !replayed {
		relayInfo.SubscriptionPostDelta += int64(feeQuota)
	}

	projectionCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := model.ReconcileTaskSubmissionCache(projectionCtx, req.SubmissionID); err != nil {
		common.SysLog("violation fee cache reconciliation pending: " + err.Error())
	}
	if err := model.DeliverPendingTaskAccountingLogs(projectionCtx, 100); err != nil {
		common.SysLog("violation fee log delivery pending: " + err.Error())
	}
	if !replayed {
		checkAndSendQuotaNotify(relayInfo, feeQuota, 0)
	}

	return true
}

func getViolationFeeBillingState(ctx *gin.Context) *violationFeeBillingState {
	if value, ok := ctx.Get(violationFeeStateKey); ok {
		if state, ok := value.(*violationFeeBillingState); ok && state != nil {
			return state
		}
	}
	state := &violationFeeBillingState{
		submissionID: common.GetUUID(),
		leaseToken:   common.GetUUID(),
		operationID:  common.GetUUID(),
	}
	ctx.Set(violationFeeStateKey, state)
	return state
}
