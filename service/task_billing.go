package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	tokenName := c.GetString("token_name")
	logContent := buildTaskConsumptionLogContent(info)
	other := buildTaskConsumptionOther(c, info)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     info.UsingGroup,
		Other:     other,
	})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
	model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
}

func buildTaskConsumptionOther(c *gin.Context, info *relaycommon.RelayInfo) map[string]interface{} {
	other := make(map[string]interface{})
	other["is_task"] = true
	if c != nil && c.Request != nil && c.Request.URL != nil {
		other["request_path"] = c.Request.URL.Path
	}
	other["model_price"] = info.PriceData.ModelPrice
	if info.PriceData.UsePrice {
		other["model_price_unit"] = info.PriceData.ModelPriceUnit
	}
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	for key, ratio := range info.PriceData.OtherRatios {
		other[key] = ratio
	}
	for key, value := range info.PriceData.BillingMeta {
		if key == "variant_legacy_ratio_keys" {
			continue
		}
		other["billing_"+key] = value
	}
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	attachQuotaSaturationToOther(other, info.QuotaClamp)
	return other
}

// BuildTaskAccountingLogFacts freezes the public billing attribution used by
// the durable async handoff. It intentionally excludes provider responses and
// all token/channel credentials.
func BuildTaskAccountingLogFacts(c *gin.Context, info *relaycommon.RelayInfo, quota int) model.TaskAccountingLogFacts {
	facts := model.TaskAccountingLogFacts{
		UserID: info.UserId, TokenID: info.TokenId, ChannelID: info.ChannelId,
		ModelName: info.OriginModelName, Group: info.UsingGroup,
		Quota: quota, LogType: model.LogTypeConsume, Content: buildTaskConsumptionLogContent(info),
		CreatedAt: common.GetTimestamp(), Other: buildTaskConsumptionOther(c, info),
	}
	if c != nil {
		facts.Username = c.GetString("username")
		facts.TokenName = c.GetString("token_name")
		facts.RequestID = c.GetString(common.RequestIdKey)
		facts.UpstreamRequestID = c.GetString(common.UpstreamRequestIdKey)
		if model.ShouldRecordUserLogIP(info.UserId) {
			facts.IP = c.ClientIP()
		}
	}
	return facts
}

func buildTaskConsumptionLogContent(info *relaycommon.RelayInfo) string {
	logContent := fmt.Sprintf("操作 %s", info.Action)
	// 固定价格任务按配置的价格单位展示。
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else if info.PriceData.UsePrice {
		unitName := "次"
		if info.PriceData.ModelPriceUnit == "second" {
			unitName = "秒"
			logContent = fmt.Sprintf("%s，按秒计费", logContent)
		} else {
			logContent = fmt.Sprintf("%s，按次计费", logContent)
		}
		variantStatus := info.PriceData.BillingMeta["variant_price_status"]
		if resolution := info.PriceData.BillingMeta["resolution"]; resolution != "" {
			if variantStatus == "disabled" {
				logContent = fmt.Sprintf("%s，请求分辨率 %s（不参与计费）", logContent, resolution)
			} else {
				logContent = fmt.Sprintf("%s，计费分辨率 %s", logContent, resolution)
			}
		}
		if quality := info.PriceData.BillingMeta["quality"]; quality != "" {
			if variantStatus == "disabled" {
				logContent = fmt.Sprintf("%s，请求质量 %s（不参与计费）", logContent, quality)
			} else {
				logContent = fmt.Sprintf("%s，计费质量 %s", logContent, quality)
			}
		}
		if variantStatus == "fallback" {
			logContent = fmt.Sprintf("%s，未匹配规格档位，使用兜底价", logContent)
		} else if variantStatus == "legacy" {
			logContent = fmt.Sprintf("%s，未匹配规格档位，沿用旧倍率计费", logContent)
		}
		logContent = fmt.Sprintf("%s，档位单价 $%.6f / %s", logContent, info.PriceData.ModelPrice, unitName)
		if seconds, ok := info.PriceData.OtherRatios["seconds"]; ok && info.PriceData.ShouldApplyTaskRatio("seconds") {
			logContent = fmt.Sprintf("%s，时长 %s 秒", logContent, strconv.FormatFloat(seconds, 'f', -1, 64))
		}
		if len(info.PriceData.OtherRatios) > 0 {
			var contents []string
			for key, ra := range info.PriceData.OtherRatios {
				if key == "seconds" {
					continue
				}
				if !info.PriceData.ShouldApplyTaskRatio(key) {
					continue
				}
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s，其它倍率：%s", logContent, strings.Join(contents, ", "))
			}
		}
		logContent = fmt.Sprintf("%s，分组倍率 %s", logContent, strconv.FormatFloat(info.PriceData.GroupRatioInfo.GroupRatio, 'f', -1, 64))
		if common.QuotaPerUnit > 0 {
			logContent = fmt.Sprintf("%s，合计 $%.6f", logContent, float64(info.PriceData.Quota)/common.QuotaPerUnit)
		}
	} else if len(info.PriceData.OtherRatios) > 0 {
		var contents []string
		for key, ra := range info.PriceData.OtherRatios {
			if !info.PriceData.ShouldApplyTaskRatio(key) || ra == 1 {
				continue
			}
			contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
		}
		if len(contents) > 0 {
			logContent = fmt.Sprintf("%s，计算参数：%s", logContent, strings.Join(contents, ", "))
		}
	}
	return logContent
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	// 差额结算会额外写入消费日志；明确标记为任务流量，避免历史指标回填时
	// 将这类账单调整误算成一次新的同步转发请求。
	other["is_task"] = true
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = bc.ModelPrice
		if bc.ModelPriceUnit != "" {
			other["model_price_unit"] = bc.ModelPriceUnit
		}
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
		if len(bc.OtherRatios) > 0 {
			for k, v := range bc.OtherRatios {
				other[k] = v
			}
		}
		for k, v := range bc.BillingMeta {
			if k == "variant_legacy_ratio_keys" {
				continue
			}
			other["billing_"+k] = v
		}
	}
	if snap := task.PrivateData.TieredBillingSnapshot; snap != nil {
		other["billing_mode"] = "tiered_expr"
		other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
		if snap.EstimatedTier != "" {
			other["matched_tier"] = snap.EstimatedTier
		}
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	if useTimeMs := taskUseTimeMilliseconds(task); useTimeMs > 0 {
		other["use_time_ms"] = float64(useTimeMs)
	}
	return other
}

func taskUseTimeMilliseconds(task *model.Task) int64 {
	if task == nil {
		return 0
	}
	start := task.StartTime
	if start <= 0 {
		start = task.SubmitTime
	}
	if start <= 0 {
		start = task.CreatedAt
	}
	end := task.FinishTime
	if end <= 0 {
		end = common.GetTimestamp()
	}
	if start <= 0 || end <= start {
		return 0
	}
	return (end - start) * 1000
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

// FinalizeTaskAccounting records the first terminal result and then applies its
// frozen amount. A failed apply leaves the unpublished decision for recovery.
func FinalizeTaskAccounting(ctx context.Context, task *model.Task, expectedStatus model.TaskStatus, finalQuota int, reason string) (*model.TaskTerminalDecisionResult, error) {
	if task == nil || task.ID <= 0 {
		return nil, fmt.Errorf("persisted task is required")
	}
	decision, err := model.AcceptTaskTerminalDecision(ctx, model.TaskTerminalDecision{
		TaskRowID: task.ID, ExpectedStatus: expectedStatus, Status: task.Status,
		Progress: task.Progress, StartTime: task.StartTime, FinishTime: task.FinishTime,
		FailReason: task.FailReason, ResultURL: task.PrivateData.ResultURL,
		Data: append([]byte(nil), task.Data...), FinalQuota: finalQuota, Reason: reason,
	})
	if err != nil {
		return nil, err
	}
	if !decision.Won && decision.Accounting.DecisionID == "" {
		return decision, nil
	}
	applied, err := model.ApplyTaskAccountingDecision(ctx, task.ID)
	if err != nil {
		return decision, err
	}
	*task = applied.Task
	if err := model.ReconcileTaskAccountingCache(ctx, task.ID); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("task accounting cache reconciliation pending for task %s: %v", task.TaskID, err))
	}
	if err := model.DeliverPendingTaskAccountingLogs(ctx, 100); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("task accounting log delivery pending for task %s: %v", task.TaskID, err))
	}
	return applied, nil
}

func RefundTaskQuota(ctx context.Context, task *model.Task, expectedStatus model.TaskStatus, reason string) (*model.TaskTerminalDecisionResult, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	if task.FinishTime == 0 {
		task.FinishTime = common.GetTimestamp()
	}
	task.FailReason = reason
	return FinalizeTaskAccounting(ctx, task, expectedStatus, 0, reason)
}

// CalculateTaskQuotaByTokens uses the frozen submit-time billing context. It
// never reloads mutable model/group ratios during a later polling cycle.
func CalculateTaskQuotaByTokens(task *model.Task, totalTokens int) (int, string, bool) {
	if task == nil || totalTokens <= 0 || task.PrivateData.BillingContext == nil {
		return 0, "", false
	}
	bc := task.PrivateData.BillingContext
	if bc.PerCallBilling || bc.ModelRatio <= 0 || bc.GroupRatio < 0 {
		return 0, "", false
	}
	otherMultiplier := 1.0
	for _, ratio := range bc.OtherRatios {
		if ratio > 0 && ratio != 1 {
			otherMultiplier *= ratio
		}
	}
	actualQuota := common.QuotaFromFloat(float64(totalTokens) * bc.ModelRatio * bc.GroupRatio * otherMultiplier)
	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, bc.ModelRatio, bc.GroupRatio, otherMultiplier)
	return actualQuota, reason, true
}

func RecalculateTaskQuota(ctx context.Context, task *model.Task, expectedStatus model.TaskStatus, actualQuota int, reason string) (*model.TaskTerminalDecisionResult, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	if actualQuota < 0 {
		return nil, fmt.Errorf("actual task quota cannot be negative")
	}
	return FinalizeTaskAccounting(ctx, task, expectedStatus, actualQuota, reason)
}

func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, expectedStatus model.TaskStatus, totalTokens int) (*model.TaskTerminalDecisionResult, error) {
	actualQuota, reason, ok := CalculateTaskQuotaByTokens(task, totalTokens)
	if !ok {
		return FinalizeTaskAccounting(ctx, task, expectedStatus, task.Quota, "预扣额度保持不变")
	}
	return RecalculateTaskQuota(ctx, task, expectedStatus, actualQuota, reason)
}
