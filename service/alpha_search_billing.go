package service

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	alphaSearchToolName        = "web_search_preview"
	alphaSearchCallCount       = 1
	alphaSearchBillingStateKey = "alpha_search_billing_state"
)

type alphaSearchBillingState struct {
	modelName      string
	group          string
	groupRatioInfo types.GroupRatioInfo
	toolPrice      float64
	toolRatios     map[string]float64
	quota          int
	fundingChosen  bool
	settled        bool
}

// AdmitAlphaSearchBilling prices and reserves exactly one configured search
// call for the currently selected group. It intentionally ignores every model
// token, fixed-request and tiered-expression price.
func AdmitAlphaSearchBilling(c *gin.Context, info *relaycommon.RelayInfo, groupRatioInfo types.GroupRatioInfo, toolMultipliers map[string]float64) *types.NewAPIError {
	if c == nil || info == nil || strings.TrimSpace(info.OriginModelName) == "" {
		return types.NewErrorWithStatusCode(errors.New("alpha search billing context is incomplete"), types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	state, err := calculateAlphaSearchBilling(info.OriginModelName, info.UsingGroup, groupRatioInfo, toolMultipliers)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	previous, _ := c.Get(alphaSearchBillingStateKey)
	previousState, _ := previous.(*alphaSearchBillingState)
	if previousState != nil {
		state.fundingChosen = previousState.fundingChosen
	}

	info.ForcePreConsume = true
	info.PriceData = types.PriceData{
		FreeModel:         state.quota == 0,
		OtherRatios:       cloneAlphaSearchRatios(state.toolRatios),
		Quota:             state.quota,
		QuotaToPreConsume: state.quota,
		GroupRatioInfo:    state.groupRatioInfo,
		BillingMeta: map[string]string{
			"alpha_search_tool":         alphaSearchToolName,
			"alpha_search_call_count":   strconv.Itoa(alphaSearchCallCount),
			"alpha_search_price_per_1k": strconv.FormatFloat(state.toolPrice, 'f', -1, 64),
		},
	}
	if info.Billing == nil {
		session, apiErr := newBillingSession(c, info, state.quota, true)
		if apiErr != nil {
			return apiErr
		}
		info.Billing = session
		state.fundingChosen = state.quota > 0
		c.Set(alphaSearchBillingStateKey, state)
		return nil
	}
	if state.quota > 0 && !state.fundingChosen {
		// A zero first attempt has not proven that its provisional funding
		// source can cover a paid retry. Close the zero journal and run normal
		// preference/fallback selection against the paid target.
		info.Billing.Refund(c)
		if info.Billing.NeedsRefund() {
			return types.NewError(errors.New("failed to close zero alpha search reservation"), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		resetAlphaSearchBillingSession(info)
		session, apiErr := newBillingSession(c, info, state.quota, true)
		if apiErr != nil {
			return apiErr
		}
		info.Billing = session
		state.fundingChosen = true
		c.Set(alphaSearchBillingStateKey, state)
		return nil
	}
	if err := info.Billing.Reserve(state.quota); err != nil {
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	if state.quota > 0 {
		state.fundingChosen = true
	}
	c.Set(alphaSearchBillingStateKey, state)
	return nil
}

func resetAlphaSearchBillingSession(info *relaycommon.RelayInfo) {
	info.Billing = nil
	info.FinalPreConsumedQuota = 0
	info.BillingSource = ""
	info.SubscriptionId = 0
	info.SubscriptionPreConsumed = 0
	info.SubscriptionPostDelta = 0
	info.SubscriptionAmountTotal = 0
	info.SubscriptionAmountUsedAfterPreConsume = 0
	info.SubscriptionPlanId = 0
	info.SubscriptionPlanTitle = ""
	info.TaskSubmissionID = ""
	info.TaskSubmissionLeaseToken = ""
	info.TaskSubmissionTaskRowID = 0
}

func calculateAlphaSearchBilling(modelName, group string, groupRatioInfo types.GroupRatioInfo, toolMultipliers map[string]float64) (*alphaSearchBillingState, error) {
	if math.IsNaN(groupRatioInfo.GroupRatio) || math.IsInf(groupRatioInfo.GroupRatio, 0) || groupRatioInfo.GroupRatio < 0 {
		return nil, errors.New("alpha search group ratio is invalid")
	}
	toolPrice := operation_setting.GetToolPriceForModel(alphaSearchToolName, modelName)
	if math.IsNaN(toolPrice) || math.IsInf(toolPrice, 0) || toolPrice < 0 {
		return nil, errors.New("alpha search tool price is invalid")
	}
	ratios := make(map[string]float64)
	quotaValue := decimal.NewFromFloat(toolPrice).
		Div(decimal.NewFromInt(1000)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromFloat(groupRatioInfo.GroupRatio))
	for name, ratio := range toolMultipliers {
		if strings.TrimSpace(name) == "" || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 {
			continue
		}
		ratios[name] = ratio
		quotaValue = quotaValue.Mul(decimal.NewFromFloat(ratio))
	}
	quota, clamp := common.QuotaFromDecimalChecked(quotaValue)
	if clamp != nil {
		return nil, clamp
	}
	return &alphaSearchBillingState{
		modelName: modelName, group: group, groupRatioInfo: groupRatioInfo,
		toolPrice: toolPrice, toolRatios: ratios, quota: quota,
	}, nil
}

func cloneAlphaSearchRatios(source map[string]float64) map[string]float64 {
	if len(source) == 0 {
		return nil
	}
	copyRatios := make(map[string]float64, len(source))
	for name, ratio := range source {
		copyRatios[name] = ratio
	}
	return copyRatios
}

// SettleAlphaSearchBilling commits money, token usage, request/user/channel
// counters and the consume-log outbox before the buffered response is exposed.
func SettleAlphaSearchBilling(c *gin.Context, info *relaycommon.RelayInfo) error {
	if c == nil || info == nil {
		return errors.New("alpha search billing context is incomplete")
	}
	value, ok := c.Get(alphaSearchBillingStateKey)
	if !ok {
		return errors.New("alpha search billing was not admitted")
	}
	state, ok := value.(*alphaSearchBillingState)
	if !ok || state == nil || state.modelName != info.OriginModelName || state.group != info.UsingGroup {
		return errors.New("alpha search billing state does not match the final attempt")
	}
	if state.settled {
		return nil
	}
	if info.Billing == nil {
		return errors.New("alpha search billing reservation is missing")
	}

	facts := buildAlphaSearchLogFacts(c, info, state)
	err := settleSynchronousSubmission(c, info, facts, &ChannelMetricUsage{}, common.GetUUID(), "alpha search", model.CompleteAlphaSearchSubmissionTx, model.ResolveAlphaSearchSettlement)
	if err != nil {
		return err
	}
	state.settled = true
	recordTextPerformanceSample(info, 0)
	return nil
}

func buildAlphaSearchLogFacts(c *gin.Context, info *relaycommon.RelayInfo, state *alphaSearchBillingState) model.TaskAccountingLogFacts {
	other := GenerateTextOtherInfo(c, info, 0, state.groupRatioInfo.GroupRatio, 0, 0, 0, 0, state.groupRatioInfo.GroupSpecialRatio)
	other["alpha_search"] = true
	other["web_search"] = true
	other["web_search_call_count"] = alphaSearchCallCount
	other["web_search_price"] = state.toolPrice
	for name, ratio := range state.toolRatios {
		other[name] = ratio
	}

	CaptureTaskBillingAttribution(c, info)
	facts := model.TaskAccountingLogFacts{
		UserID: info.UserId, TokenID: info.TokenId, ChannelID: info.ChannelId,
		ModelName: info.OriginModelName, Group: info.UsingGroup,
		Quota: state.quota, LogType: model.LogTypeConsume,
		Content: alphaSearchLogContent(state), CreatedAt: common.GetTimestamp(), Other: other,
		UseTimeSeconds: int(time.Now().Unix() - info.StartTime.Unix()),
	}
	if attribution := info.TaskBillingAttribution; attribution != nil {
		facts.Username = attribution.Username
		facts.TokenName = attribution.TokenName
		facts.RequestID = attribution.RequestID
		facts.UpstreamRequestID = attribution.UpstreamRequestID
		facts.IP = attribution.IP
	}
	return facts
}

func alphaSearchLogContent(state *alphaSearchBillingState) string {
	cost := decimal.NewFromFloat(state.toolPrice).
		Div(decimal.NewFromInt(1000)).
		Mul(decimal.NewFromFloat(state.groupRatioInfo.GroupRatio))
	names := make([]string, 0, len(state.toolRatios))
	for name := range state.toolRatios {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cost = cost.Mul(decimal.NewFromFloat(state.toolRatios[name]))
	}
	return fmt.Sprintf("Alpha Search 调用 1 次，调用花费 %s", cost.Mul(decimal.NewFromFloat(common.QuotaPerUnit)).String())
}
