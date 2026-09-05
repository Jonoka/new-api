package helper

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func modelPriceNotConfiguredError(modelName string, userId int) error {
	if model.IsAdmin(userId) {
		return fmt.Errorf(
			"模型 %s 的价格未配置。请前往「系统设置 → 运营设置」开启自用模式，或在「系统设置 → 分组与模型定价设置」中为该模型配置价格；"+
				"Model %s price not configured. Go to System Settings → Operation Settings to enable self-use mode, or configure the model price in System Settings → Group & Model Pricing.",
			modelName, modelName,
		)
	}
	return fmt.Errorf(
		"模型 %s 的价格尚未由管理员配置，暂时无法使用，请联系站点管理员开启该模型；"+
			"Model %s has not been priced by the administrator yet. Please contact the site administrator to enable this model.",
		modelName, modelName,
	)
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// 客户端未提供最大输出时，为分层计费预扣一个保守的输出上限。
const defaultTieredPreConsumeMaxTokens = 8192

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, "final group: %s", autoGroup)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)
	if info.PricingInitialized && !info.PricingPerCall {
		if info.PricingBillingMode == billing_setting.BillingModeTieredExpr {
			return modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
		}
		return refreshTokenPriceForGroup(c, info, groupRatioInfo)
	}

	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)

	// Check if this model uses tiered_expr billing
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
		priceData, err := modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
		if err == nil {
			freezePricing(info, false, billing_setting.BillingModeTieredExpr, promptTokens, meta.MaxTokens)
		}
		return priceData, err
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio
		quota, err := common.QuotaFromFloatStrict(float64(preConsumedTokens) * ratio)
		if err != nil {
			return types.PriceData{}, err
		}
		preConsumedQuota = quota
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelPriceUnit:       ratio_setting.GetModelPriceUnit(info.OriginModelName),
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
	}
	if usePrice {
		for name, ratio := range meta.BillingRatios {
			priceData.AddOtherRatio(name, ratio)
		}
		quotaToPreConsume := priceData.ApplyOtherRatiosToFloat(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		quota, err := common.QuotaFromFloatStrict(quotaToPreConsume)
		if err != nil {
			return types.PriceData{}, err
		}
		priceData.QuotaToPreConsume = quota
	}

	if common.DebugEnabled {
		logger.LogDebug(c, "model_price_helper result: %s", priceData.ToSetting())
	}
	info.PriceData = priceData
	freezePricing(info, false, billing_setting.BillingModeRatio, promptTokens, meta.MaxTokens)
	return priceData, nil
}

// ModelPriceHelperPerCall 固定单价/倍率任务的 PriceHelper（MJ、Task）。
// 固定单价的具体单位由 PriceData.ModelPriceUnit 决定。
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)
	if info.PricingInitialized && info.PricingPerCall {
		if info.PricingBillingMode == billing_setting.BillingModeTieredExpr {
			priceData, err := modelPriceHelperTiered(c, info, 0, &types.TokenCountMeta{}, groupRatioInfo)
			if err != nil {
				return types.PriceData{}, err
			}
			priceData.Quota = priceData.QuotaToPreConsume
			info.PriceData = priceData
			return priceData, nil
		}
		return refreshPerCallPriceForGroup(info, groupRatioInfo)
	}
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
		priceData, err := modelPriceHelperTiered(c, info, 0, &types.TokenCountMeta{}, groupRatioInfo)
		if err != nil {
			return types.PriceData{}, err
		}
		priceData.Quota = priceData.QuotaToPreConsume
		info.PriceData = priceData
		freezePricing(info, true, billing_setting.BillingModeTieredExpr, 0, 0)
		return priceData, nil
	}

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			var ratioSuccess bool
			var matchName string
			modelRatio, ratioSuccess, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !ratioSuccess && !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
	}

	var quota int
	freeModel := false

	if usePrice {
		var err error
		quota, err = common.QuotaFromFloatStrict(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return types.PriceData{}, err
		}
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		var err error
		quota, err = common.QuotaFromFloatStrict(modelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return types.PriceData{}, err
		}
		modelPrice = -1
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelRatio == 0 {
				quota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:      freeModel,
		ModelPrice:     modelPrice,
		ModelPriceUnit: ratio_setting.GetModelPriceUnit(info.OriginModelName),
		ModelRatio:     modelRatio,
		UsePrice:       usePrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}
	info.PriceData = priceData
	freezePricing(info, true, billing_setting.BillingModeRatio, 0, 0)
	return priceData, nil
}

func freezePricing(info *relaycommon.RelayInfo, perCall bool, mode string, promptTokens int, maxCompletionTokens int) {
	info.PricingInitialized = true
	info.PricingPerCall = perCall
	info.PricingBillingMode = mode
	info.PricingPromptTokens = promptTokens
	info.PricingMaxCompletionTokens = maxCompletionTokens
}

func refreshTokenPriceForGroup(c *gin.Context, info *relaycommon.RelayInfo, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	priceData := info.PriceData
	priceData.GroupRatioInfo = groupRatioInfo
	priceData.FreeModel = false
	if priceData.UsePrice {
		quotaValue := priceData.ApplyOtherRatiosToFloat(priceData.ModelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		quota, err := common.QuotaFromFloatStrict(quotaValue)
		if err != nil {
			return types.PriceData{}, err
		}
		priceData.QuotaToPreConsume = quota
	} else {
		preConsumedTokens := common.Max(info.PricingPromptTokens, common.PreConsumedQuota)
		if info.PricingMaxCompletionTokens != 0 {
			preConsumedTokens += info.PricingMaxCompletionTokens
		}
		quota, err := common.QuotaFromFloatStrict(float64(preConsumedTokens) * priceData.ModelRatio * groupRatioInfo.GroupRatio)
		if err != nil {
			return types.PriceData{}, err
		}
		priceData.QuotaToPreConsume = quota
	}
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume &&
		(groupRatioInfo.GroupRatio == 0 || (priceData.UsePrice && priceData.ModelPrice == 0) || (!priceData.UsePrice && priceData.ModelRatio == 0)) {
		priceData.QuotaToPreConsume = 0
		priceData.FreeModel = true
	}
	info.PriceData = priceData
	if common.DebugEnabled {
		logger.LogDebug(c, "refresh_token_price_for_group result: %s", priceData.ToSetting())
	}
	return priceData, nil
}

func refreshPerCallPriceForGroup(info *relaycommon.RelayInfo, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	priceData := info.PriceData
	priceData.GroupRatioInfo = groupRatioInfo
	priceData.FreeModel = false
	quotaValue := priceData.ModelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio
	if priceData.UsePrice {
		quotaValue = priceData.ModelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio
	}
	quota, err := common.QuotaFromFloatStrict(quotaValue)
	if err != nil {
		return types.PriceData{}, err
	}
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume &&
		(groupRatioInfo.GroupRatio == 0 || (priceData.UsePrice && priceData.ModelPrice == 0) || (!priceData.UsePrice && priceData.ModelRatio == 0)) {
		quota = 0
		priceData.FreeModel = true
	}
	priceData.Quota = quota
	priceData.QuotaToPreConsume = quota
	info.PriceData = priceData
	return priceData, nil
}

func HasModelBillingConfig(modelName string) bool {
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(modelName); ok {
		return true
	}
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTieredExpr {
		return false
	}
	expr, ok := billing_setting.GetBillingExpr(modelName)
	return ok && strings.TrimSpace(expr) != ""
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	exprStr := ""
	estimatedPromptTokens := promptTokens
	estimatedCompletionTokens := meta.MaxTokens
	if frozen := info.TieredBillingSnapshot; frozen != nil && frozen.BillingMode == billing_setting.BillingModeTieredExpr && frozen.ModelName == info.OriginModelName {
		exprStr = frozen.ExprString
		estimatedPromptTokens = frozen.EstimatedPromptTokens
		estimatedCompletionTokens = frozen.EstimatedCompletionTokens
	} else {
		var ok bool
		exprStr, ok = billing_setting.GetBillingExpr(info.OriginModelName)
		if !ok {
			return types.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", info.OriginModelName)
		}
	}
	if estimatedCompletionTokens == 0 {
		estimatedCompletionTokens = defaultTieredPreConsumeMaxTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}

	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
		P:   float64(estimatedPromptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(estimatedPromptTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", info.OriginModelName, err)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota, err := billingexpr.QuotaRoundStrict(quotaBeforeGroup * groupRatioInfo.GroupRatio)
	if err != nil {
		return types.PriceData{}, err
	}

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		Group:                     info.UsingGroup,
		GroupRatio:                groupRatioInfo.GroupRatio,
		GroupSpecialRatio:         groupRatioInfo.GroupSpecialRatio,
		HasGroupSpecialRatio:      groupRatioInfo.HasSpecialRatio,
		EstimatedPromptTokens:     estimatedPromptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		QuotaToPreConsume: preConsumedQuota,
	}

	logger.LogDebug(c, "model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", info.OriginModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier)

	info.PriceData = priceData
	return priceData, nil
}
