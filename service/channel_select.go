package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const maxSelfReferentialChannelSkips = 64

type RetryParam struct {
	Ctx                *gin.Context
	TokenGroup         string
	ModelName          string
	Retry              *int
	ExcludedChannelIDs map[int]struct{}
	ForceExcludeUsed   bool
	resetNextTry       bool
}

func excludeSelfReferentialChannel(param *RetryParam, channel *model.Channel, group string) bool {
	if param == nil || channel == nil || !IsSelfReferentialChannel(param.Ctx, channel) {
		return false
	}
	if param.ExcludedChannelIDs == nil {
		param.ExcludedChannelIDs = make(map[int]struct{})
	}
	param.ExcludedChannelIDs[channel.Id] = struct{}{}
	logger.LogError(param.Ctx, fmt.Sprintf("检测到自引用渠道 #%d（分组：%s，上游：%s），已跳过以避免递归请求和 429 放大", channel.Id, group, channel.GetBaseURL()))
	return true
}

func getRandomSatisfiedChannelWithGuards(param *RetryParam, group string, modelName string, retry int) (*model.Channel, error) {
	if param == nil {
		return model.GetRandomSatisfiedChannelWithExclusions(group, modelName, retry, nil)
	}
	priorityRetry := retry
	skippedSelfReference := false
	for i := 0; i < maxSelfReferentialChannelSkips; i++ {
		channel, err := model.GetRandomSatisfiedChannelWithExclusions(group, modelName, priorityRetry, param.ExcludedChannelIDs)
		if err != nil {
			return nil, err
		}
		if channel == nil {
			if skippedSelfReference {
				priorityRetry++
				skippedSelfReference = false
				continue
			}
			return nil, nil
		}
		if !excludeSelfReferentialChannel(param, channel, group) {
			return channel, nil
		}
		skippedSelfReference = true
	}
	return nil, fmt.Errorf("分组 %s 存在过多自引用渠道，已停止本次调度", group)
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func orderedRetryGroups(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	groups := make([]string, 0, len(parts))
	for _, part := range parts {
		group := strings.TrimSpace(part)
		if group == "" {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return groups
}

func retryGroupIndex(ctx *gin.Context) int {
	if value, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroupIndex); exists {
		if index, ok := value.(int); ok && index > 0 {
			return index
		}
	}
	return 0
}

// RelayMaxRetries 返回 relay 外层允许的最大重试次数。
// 单分组保持站点 RetryTimes；显式多分组和启用跨组的 auto
// 每组只发起一次上游请求，因此预算由分组数决定。
func RelayMaxRetries(param *RetryParam) int {
	if param == nil {
		return common.RetryTimes
	}
	groupCount := 0
	if strings.Contains(param.TokenGroup, ",") {
		groupCount = len(orderedRetryGroups(param.TokenGroup))
	} else if param.TokenGroup == "auto" && common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry) {
		userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
		groupCount = len(GetUserAutoGroup(userGroup))
	}
	if groupCount > 0 {
		return groupCount - 1
	}
	return common.RetryTimes
}

func CheckTokenGroupRatioLimit(ctx *gin.Context, userGroup string, usingGroup string) error {
	if ctx == nil || usingGroup == "" {
		return nil
	}
	limits, ok := common.GetContextKeyType[map[string]float64](ctx, constant.ContextKeyTokenGroupRatioLimits)
	if !ok || len(limits) == 0 {
		return nil
	}
	maxRatio, ok := limits[usingGroup]
	if !ok || maxRatio <= 0 {
		return nil
	}
	actualRatio, hasSpecialRatio := ratio_setting.GetGroupGroupRatio(userGroup, usingGroup)
	if !hasSpecialRatio {
		actualRatio = ratio_setting.GetGroupRatio(usingGroup)
	}
	if actualRatio > maxRatio {
		return errors.New("超过令牌倍率保护")
	}
	return nil
}

// CacheGetRandomSatisfiedChannel 从当前重试分组选择渠道。
// 显式多分组和启用跨组重试的 auto 每组最多发起一次上游请求；
// 当前分组失败后，下一次 relay 尝试直接进入后续分组。
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetUserAutoGroup(userGroup)

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// Calculate priorityRetry for current group
			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()
			if crossGroupRetry {
				priorityRetry = 0
			}
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			channel, _ = getRandomSatisfiedChannelWithGuards(param, autoGroup, param.ModelName, priorityRetry)
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				if !crossGroupRetry {
					param.SetRetry(0)
				}
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			if err := CheckTokenGroupRatioLimit(param.Ctx, userGroup, autoGroup); err != nil {
				return nil, selectGroup, err
			}
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			if crossGroupRetry {
				logger.LogDebug(param.Ctx, "Auto group %s will switch immediately on the next retry", autoGroup)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else if strings.Contains(param.TokenGroup, ",") {
		// 使用原始有序分组的索引跟踪进度，避免渠道动态禁用后列表收缩而跳过后续分组。
		candidateGroups := orderedRetryGroups(param.TokenGroup)
		startGroupIndex := retryGroupIndex(param.Ctx)

		if len(candidateGroups) == 0 {
			// 没有任何分组有该模型的渠道
			return nil, selectGroup, nil
		} else if len(candidateGroups) == 1 {
			// 单候选分组：直接路由，不需要分组间 failover
			if startGroupIndex > 0 {
				return nil, candidateGroups[0], nil
			}
			if !model.GroupHasModelChannels(candidateGroups[0], param.ModelName) {
				return nil, candidateGroups[0], nil
			}
			channel, err = getRandomSatisfiedChannelWithGuards(param, candidateGroups[0], param.ModelName, 0)
			selectGroup = candidateGroups[0]
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, candidateGroups[0])
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, 1)
			if err != nil {
				return nil, selectGroup, err
			}
			if err := CheckTokenGroupRatioLimit(param.Ctx, userGroup, candidateGroups[0]); err != nil {
				return nil, selectGroup, err
			}
		} else {
			// 多候选分组：按用户排序依次尝试（复用 auto 的 group-advancement 模式）
			for i := startGroupIndex; i < len(candidateGroups); i++ {
				g := candidateGroups[i]
				priorityRetry := 0
				if !model.GroupHasModelChannels(g, param.ModelName) {
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
					continue
				}
				logger.LogDebug(param.Ctx, "Multi-group selecting group: %s, priorityRetry: %d", g, priorityRetry)

				channel, _ = getRandomSatisfiedChannelWithGuards(param, g, param.ModelName, priorityRetry)
				if channel == nil {
					logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", g, param.ModelName, priorityRetry)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
					continue
				}
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, g)
				selectGroup = g
				if err := CheckTokenGroupRatioLimit(param.Ctx, userGroup, g); err != nil {
					return nil, selectGroup, err
				}
				logger.LogDebug(param.Ctx, "Multi-group selected group: %s", g)

				// 当前分组最多发起一次上游请求，失败后下次直接进入后续分组。
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				break
			}
		}
	} else {
		channel, err = getRandomSatisfiedChannelWithGuards(param, param.TokenGroup, param.ModelName, param.GetRetry())
		if err != nil {
			return nil, param.TokenGroup, err
		}
		if channel != nil {
			if err := CheckTokenGroupRatioLimit(param.Ctx, userGroup, param.TokenGroup); err != nil {
				return nil, param.TokenGroup, err
			}
		}
	}
	return channel, selectGroup, nil
}
