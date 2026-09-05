package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError                   *types.NewAPIError
		relayInfo                     *relaycommon.RelayInfo
		ws                            *websocket.Conn
		liteChannelSelectionExhausted bool
	)

	service.BeginChannelMetricRequest(c)
	if c.Request != nil && strings.HasPrefix(c.Request.URL.Path, "/pg") {
		service.MarkChannelMetricPlaygroundRequest(c)
	}
	// 此 defer 必须早于错误响应 defer 注册。Go 按后进先出执行 defer，
	// 因而最终请求样本可以读取错误响应实际写出的客户端状态码。
	defer func() {
		service.FinishChannelMetricRequest(c, relayInfo, newAPIError)
	}()

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			newAPIError = types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
			helper.WssError(c, ws, newAPIError.ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		writeRelayErrorResponse(c, ws, relayFormat, newAPIError, requestId)
	}()

	filterResult, err := service.ApplySensitiveFilterToRequestBody(c, relayFormat)
	if err != nil {
		newAPIError = relayRequestParseError(err)
		return
	}
	if filterResult.Blocked {
		logger.LogWarn(c, fmt.Sprintf("user sensitive request blocked: %s", service.FormatSensitiveFilterMatches(filterResult.Matches)))
		newAPIError = types.NewError(errors.New("sensitive words detected"), types.ErrorCodeSensitiveWordsDetected)
		return
	}

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		newAPIError = relayRequestParseError(err)
		return
	}

	relayInfo, err = relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}
	c.Set("relay_info", relayInfo)
	common.SetContextKey(c, constant.ContextKeyRelayInfo, relayInfo)
	service.BindChannelMetricRelayInfo(c, relayInfo)

	needCountToken := constant.CountToken
	var meta *types.TokenCountMeta
	if needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil
	channelRetryStates := make(map[int]channelRetryState)
	var pendingChannelFailure *channelFailureSnapshot
	maxRetries := service.RelayMaxRetries(retryParam)

	for attemptIndex := 0; attemptIndex <= maxRetries; attemptIndex++ {
		relayInfo.RetryIndex = attemptIndex
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			if pendingChannelFailure != nil {
				newAPIError = finalizePendingChannelFailure(c, relayInfo, pendingChannelFailure)
				pendingChannelFailure = nil
			} else {
				newAPIError = channelErr
			}
			liteChannelSelectionExhausted = strings.EqualFold(strings.TrimSpace(relayInfo.OriginModelName), "gpt-image-2-lite") && len(c.GetStringSlice("use_channel")) > 0
			break
		}

		if delay := channelRetryDelay(channelRetryStates, channel.Id, time.Now()); delay > 0 {
			logger.LogInfo(c, fmt.Sprintf("429 重试复用渠道 #%d，等待 %s", channel.Id, delay))
			if !waitForRelayRetry(c, delay) {
				return
			}
		}

		priceData, priceErr := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
		if priceErr != nil {
			newAPIError = types.NewError(priceErr, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest), types.ErrOptionWithSkipRetry())
			break
		}
		reservationTarget := priceData.QuotaToPreConsume
		if priceData.FreeModel {
			reservationTarget = 0
			logger.LogInfo(c, fmt.Sprintf("模型 %s 在分组 %s 免费", relayInfo.OriginModelName, relayInfo.UsingGroup))
		}
		if admissionErr := service.ReconcileBillingReservation(c, reservationTarget, relayInfo); admissionErr != nil {
			newAPIError = admissionErr
			break
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)
		relayInfo.ResetAttemptState(time.Now())
		service.BeginChannelMetricAttempt(c, relayInfo, channel.Id, channel.Name, channel.Type)

		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		if newAPIError == nil {
			relayInfo.LastError = nil
			service.FinishChannelMetricAttempt(c, relayInfo, nil, false, "")
			return
		}

		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError
		recordChannelRetryState(channelRetryStates, channel.Id, newAPIError, time.Now())

		retryDecision := shouldRetryWithReason(c, newAPIError, remainingRelayRetries(maxRetries, attemptIndex))
		channelError := *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan())
		processChannelError(
			c,
			relayInfo,
			channelError,
			newAPIError,
			!retryDecision.Retry,
		)
		if retryDecision.Retry {
			pendingChannelFailure = &channelFailureSnapshot{channel: channelError, err: newAPIError}
			excludeChannelFromRetry(c, retryParam, channel, newAPIError)
		} else {
			pendingChannelFailure = nil
		}
		service.FinishChannelMetricAttempt(c, relayInfo, newAPIError, retryDecision.Retry, retryDecision.Reason)
		if retryDecision.Reason == "auth_unavailable_same_group_fallback" {
			retryParam.ForceExcludeUsed = true
			retryParam.SetRetry(0)
			retryParam.ResetRetryNextTry()
		}
		if retryDecision.Reason == "auth_unavailable_fast_group_fallback" {
			retryParam.ForceExcludeUsed = false
			retryParam.SetRetry(0)
			retryParam.ResetRetryNextTry()
		}
		if !retryDecision.Retry {
			logger.LogInfo(c, fmt.Sprintf("不重试：%s", retryDecision.Reason))
			break
		}
		retryParam.IncreaseRetry()
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if newAPIError != nil {
		if shouldMarkLitePoolExhausted(c, relayInfo, newAPIError, liteChannelSelectionExhausted) {
			newAPIError = markLitePoolExhausted(newAPIError)
		}
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

func relayRequestParseError(err error) *types.NewAPIError {
	if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
	}
	return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithStatusCode(http.StatusBadRequest), types.ErrOptionWithSkipRetry())
}

func shouldMarkLitePoolExhausted(c *gin.Context, relayInfo *relaycommon.RelayInfo, err *types.NewAPIError, channelSelectionExhausted bool) bool {
	if !channelSelectionExhausted || c == nil || relayInfo == nil || err == nil || !strings.EqualFold(strings.TrimSpace(relayInfo.OriginModelName), "gpt-image-2-lite") {
		return false
	}
	if len(c.GetStringSlice("use_channel")) == 0 {
		return false
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeInvalidRequest,
		types.ErrorCodeReadRequestBodyFailed,
		types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeModelPriceError,
		types.ErrorCodeInsufficientUserQuota,
		types.ErrorCodePreConsumeTokenQuotaFailed,
		types.ErrorCodePromptBlocked,
		types.ErrorCodeSensitiveWordsDetected,
		types.ErrorCodeAccessDenied:
		return false
	}
	return err.StatusCode >= http.StatusInternalServerError && err.StatusCode <= 599
}

func markLitePoolExhausted(err *types.NewAPIError) *types.NewAPIError {
	if err == nil {
		return nil
	}
	openAIError := err.ToOpenAIError()
	openAIError.Type = string(types.ErrorCodeLitePoolExhausted)
	openAIError.Code = string(types.ErrorCodeLitePoolExhausted)
	return types.WithOpenAIError(openAIError, err.StatusCode)
}

func writeRelayErrorResponse(c *gin.Context, ws *websocket.Conn, relayFormat types.RelayFormat, relayErr *types.NewAPIError, requestID string) {
	if relayErr == nil {
		return
	}
	if reason := requestContextErrorReason(c, relayErr); reason != "" {
		logger.LogInfo(c, fmt.Sprintf("relay stopped after request context ended (%s): %s", reason, common.LocalLogPreview(relayErr.Error())))
		return
	}
	logger.LogError(c, fmt.Sprintf("relay error: %s", common.LocalLogPreview(relayErr.Error())))
	if relayFormat != types.RelayFormatOpenAIRealtime && c != nil && c.Writer != nil && c.Writer.Written() {
		logger.LogInfo(c, "relay response already started; skip writing a second error response")
		return
	}

	relayErr.SetMessage(common.MessageWithRequestId(relayErr.Error(), requestID))
	switch relayFormat {
	case types.RelayFormatOpenAIRealtime:
		helper.WssError(c, ws, relayErr.ToOpenAIError())
	case types.RelayFormatClaude:
		c.JSON(relayErr.StatusCode, gin.H{
			"type":  "error",
			"error": relayErr.ToClaudeError(),
		})
	default:
		c.JSON(relayErr.StatusCode, gin.H{
			"error": relayErr.ToOpenAIError(),
		})
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func usedChannelIDSet(c *gin.Context) map[int]struct{} {
	if c == nil {
		return nil
	}
	useChannel := c.GetStringSlice("use_channel")
	used := make(map[int]struct{}, len(useChannel))
	for _, raw := range useChannel {
		id, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil && id > 0 {
			used[id] = struct{}{}
		}
	}
	if len(used) == 0 {
		return nil
	}
	return used
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	if retryParam.GetRetry() > 0 || retryParam.ForceExcludeUsed {
		retryParam.ExcludedChannelIDs = usedChannelIDSet(c)
	} else {
		retryParam.ExcludedChannelIDs = nil
	}
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)

	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	if retryParam.GetRetry() > 0 {
		if rateLimitErr := middleware.CheckModelRequestRateLimitForGroup(c, selectGroup); rateLimitErr != nil {
			return nil, rateLimitErr
		}
	}

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

type retryDecision struct {
	Retry  bool
	Reason string
}

type channelFailureSnapshot struct {
	channel types.ChannelError
	err     *types.NewAPIError
}

func remainingRelayRetries(maxRetries int, attemptIndex int) int {
	remaining := maxRetries - attemptIndex
	if remaining < 0 {
		return 0
	}
	return remaining
}

func finalizePendingChannelFailure(c *gin.Context, relayInfo *relaycommon.RelayInfo, pending *channelFailureSnapshot) *types.NewAPIError {
	if pending == nil {
		return nil
	}
	recordChannelErrorLog(c, relayInfo, pending.channel, pending.err)
	return pending.err
}

func excludeChannelFromRetry(c *gin.Context, param *service.RetryParam, channel *model.Channel, relayErr *types.NewAPIError) {
	if param == nil || channel == nil || relayErr == nil {
		return
	}
	// 只有多 Key 渠道允许同渠道复用，用于同渠道内换 Key/退避。
	// 单 Key 429 需要切换其它候选渠道，避免把一次上游限流放大成重复撞同一渠道。
	controlledReuse := channel.ChannelInfo.IsMultiKey
	crossGroupRetry := strings.Contains(param.TokenGroup, ",") ||
		(param.TokenGroup == "auto" && common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry))
	if controlledReuse && !crossGroupRetry {
		return
	}
	if param.ExcludedChannelIDs == nil {
		param.ExcludedChannelIDs = make(map[int]struct{})
	}
	param.ExcludedChannelIDs[channel.Id] = struct{}{}
}

const (
	repeatedChannelRetryBaseDelay = 500 * time.Millisecond
	repeatedChannelRetryMaxDelay  = 10 * time.Second
)

type channelRetryState struct {
	rateLimitCount int
	cooldownUntil  time.Time
}

func isRateLimitError(openaiErr *types.NewAPIError) bool {
	if openaiErr == nil {
		return false
	}
	return openaiErr.StatusCode == http.StatusTooManyRequests ||
		openaiErr.OriginalStatusCode == http.StatusTooManyRequests
}

func repeatedChannelRetryDelay(openaiErr *types.NewAPIError, retry int, repeatedChannel bool) time.Duration {
	if !repeatedChannel || !isRateLimitError(openaiErr) {
		return 0
	}

	delay := openaiErr.RetryAfter
	if delay <= 0 {
		exponent := retry - 1
		if exponent < 0 {
			exponent = 0
		}
		if exponent > 4 {
			exponent = 4
		}
		delay = repeatedChannelRetryBaseDelay * time.Duration(1<<exponent)
	}
	if delay > repeatedChannelRetryMaxDelay {
		return repeatedChannelRetryMaxDelay
	}
	return delay
}

func recordChannelRetryState(states map[int]channelRetryState, channelID int, openaiErr *types.NewAPIError, now time.Time) {
	if states == nil || channelID <= 0 {
		return
	}
	if !isRateLimitError(openaiErr) {
		delete(states, channelID)
		return
	}

	state := states[channelID]
	state.rateLimitCount++
	delay := repeatedChannelRetryDelay(openaiErr, state.rateLimitCount, true)
	state.cooldownUntil = now.Add(delay)
	states[channelID] = state
}

func channelRetryDelay(states map[int]channelRetryState, channelID int, now time.Time) time.Duration {
	state, ok := states[channelID]
	if !ok {
		return 0
	}
	delay := state.cooldownUntil.Sub(now)
	if delay <= 0 {
		return 0
	}
	return delay
}

func waitForRelayRetry(c *gin.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-c.Request.Context().Done():
		return false
	}
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	return shouldRetryWithReason(c, openaiErr, retryTimes).Retry
}

func isAuthUnavailableError(openaiErr *types.NewAPIError) bool {
	if openaiErr == nil {
		return false
	}
	if openaiErr.StatusCode != http.StatusServiceUnavailable {
		return false
	}
	msg := strings.ToLower(openaiErr.Error())
	return strings.Contains(msg, "auth_unavailable") && strings.Contains(msg, "no auth available")
}

func fallbackTokenGroup(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if relayInfo, ok := c.Get("relay_info"); ok {
		if info, ok := relayInfo.(*relaycommon.RelayInfo); ok && info != nil && info.TokenGroup != "" {
			return info.TokenGroup
		}
	}
	return common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
}

func shouldFastFallbackGroup(c *gin.Context, openaiErr *types.NewAPIError) bool {
	if c == nil || !isAuthUnavailableError(openaiErr) {
		return false
	}
	tokenGroup := fallbackTokenGroup(c)
	if tokenGroup == "auto" {
		return true
	}
	if strings.Contains(tokenGroup, ",") {
		return true
	}
	return false
}

func markFastFallbackNextGroup(c *gin.Context) {
	if c == nil {
		return
	}
	idx := common.GetContextKeyInt(c, constant.ContextKeyAutoGroupIndex)
	common.SetContextKey(c, constant.ContextKeyAutoGroupIndex, idx+1)
	common.SetContextKey(c, constant.ContextKeyAutoGroupRetryIndex, 0)
}

const authUnavailableSameGroupTriedKey = "auth_unavailable_same_group_tried"

func shouldTrySameGroupAfterAuthUnavailable(c *gin.Context) bool {
	if c == nil || c.GetBool(authUnavailableSameGroupTriedKey) {
		return false
	}
	c.Set(authUnavailableSameGroupTriedKey, true)
	return true
}

func consumeResponsesPreOutputCapacityRetry(c *gin.Context) bool {
	if c == nil || !c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)) {
		return false
	}
	c.Set(string(constant.ContextKeyResponsesPreOutputRetry), false)
	return true
}

func shouldRetryWithReason(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) retryDecision {
	if openaiErr == nil {
		return retryDecision{Reason: "nil_error"}
	}
	if consumeResponsesPreOutputCapacityRetry(c) {
		if retryTimes <= 0 {
			return retryDecision{Reason: "retry_exhausted"}
		}
		if _, ok := c.Get("specific_channel_id"); ok {
			return retryDecision{Reason: "specific_channel"}
		}
		return retryDecision{Retry: true, Reason: "responses_capacity_fallback"}
	}
	if shouldFastFallbackGroup(c, openaiErr) {
		if relayInfo, ok := c.Get("relay_info"); ok {
			if info, ok := relayInfo.(*relaycommon.RelayInfo); ok && info != nil && info.ReceivedResponseCount > 0 {
				return retryDecision{Reason: "no_retry_after_stream_started"}
			}
		}
		if _, ok := c.Get("specific_channel_id"); ok {
			return retryDecision{Reason: "specific_channel"}
		}
		if shouldTrySameGroupAfterAuthUnavailable(c) {
			return retryDecision{Retry: true, Reason: "auth_unavailable_same_group_fallback"}
		}
		markFastFallbackNextGroup(c)
		return retryDecision{Retry: true, Reason: "auth_unavailable_fast_group_fallback"}
	}
	if relayInfo, ok := common.GetContextKey(c, constant.ContextKeyRelayInfo); ok {
		if info, ok := relayInfo.(*relaycommon.RelayInfo); ok && info != nil {
			if info.ReceivedResponseCount > 0 {
				return retryDecision{Reason: "no_retry_after_stream_started"}
			}
		}
	}
	if reason := requestContextRetryBlockReason(c); reason != "" {
		return retryDecision{Reason: reason}
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return retryDecision{Reason: "channel_affinity_skip"}
	}
	if types.IsSkipRetryError(openaiErr) {
		return retryDecision{Reason: "skip_retry_error"}
	}
	if retryTimes <= 0 {
		return retryDecision{Reason: "retry_exhausted"}
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return retryDecision{Reason: "specific_channel"}
	}
	if types.IsChannelError(openaiErr) {
		return retryDecision{Retry: true, Reason: "channel_error"}
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return retryDecision{Reason: "success_status_code"}
	}
	if code < 100 || code > 599 {
		return retryDecision{Retry: true, Reason: "invalid_status_code_retry"}
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return retryDecision{Reason: "always_skip_error_code"}
	}
	if openaiErr.GetErrorCode() == types.ErrorCodeDoRequestFailed {
		return retryDecision{Retry: true, Reason: "transport_error_retry"}
	}
	if operation_setting.ShouldRetryByStatusCode(code) {
		return retryDecision{Retry: true, Reason: "status_code_retry"}
	}
	return retryDecision{Reason: "status_code_not_configured"}
}

func requestContextRetryBlockReason(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	switch err := c.Request.Context().Err(); {
	case errors.Is(err, context.Canceled):
		return "request_context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "request_context_deadline_exceeded"
	case err != nil:
		return "request_context_done"
	default:
		return ""
	}
}

func requestContextErrorReason(c *gin.Context, err error) string {
	if err == nil || c == nil || c.Request == nil {
		return ""
	}
	contextErr := c.Request.Context().Err()
	if contextErr == nil || !errors.Is(err, contextErr) {
		return ""
	}
	switch {
	case errors.Is(contextErr, context.Canceled):
		return "request_context_canceled"
	case errors.Is(contextErr, context.DeadlineExceeded):
		return "request_context_deadline_exceeded"
	default:
		return "request_context_done"
	}
}

func appendErrorLogRequestConversion(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || len(relayInfo.RequestConversionChain) == 0 {
		return
	}
	chain := make([]string, 0, len(relayInfo.RequestConversionChain))
	for _, f := range relayInfo.RequestConversionChain {
		switch f {
		case types.RelayFormatOpenAI:
			chain = append(chain, "OpenAI Compatible")
		case types.RelayFormatClaude:
			chain = append(chain, "Claude Messages")
		case types.RelayFormatGemini:
			chain = append(chain, "Google Gemini")
		case types.RelayFormatOpenAIResponses:
			chain = append(chain, "OpenAI Responses")
		default:
			chain = append(chain, string(f))
		}
	}
	if len(chain) > 0 {
		other["request_conversion"] = chain
	}
}

func processChannelError(c *gin.Context, relayInfo *relaycommon.RelayInfo, channelError types.ChannelError, err *types.NewAPIError, recordErrorLog bool) {
	requestContextEnded := requestContextErrorReason(c, err) != ""
	message := fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.Error()))
	if requestContextEnded {
		logger.LogInfo(c, message)
	} else {
		logger.LogError(c, message)
	}
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if !requestContextEnded && service.ShouldDisableChannel(err) && channelError.AutoBan {
		disableReason := err.ErrorWithStatusCode()
		gopool.Go(func() {
			service.DisableChannel(channelError, disableReason)
		})
	}

	if recordErrorLog {
		recordChannelErrorLog(c, relayInfo, channelError, err)
	}
}

func recordChannelErrorLog(c *gin.Context, relayInfo *relaycommon.RelayInfo, channelError types.ChannelError, err *types.NewAPIError) {
	if requestContextErrorReason(c, err) != "" || !constant.ErrorLogEnabled || !types.IsRecordErrorLog(err) {
		return
	}
	userId := c.GetInt("id")
	tokenName := c.GetString("token_name")
	modelName := c.GetString("original_model")
	tokenId := c.GetInt("token_id")
	userGroup := c.GetString("group")
	if relayInfo != nil && relayInfo.UsingGroup != "" {
		userGroup = relayInfo.UsingGroup
	}
	channelId := channelError.ChannelId
	other := make(map[string]interface{})
	if c.Request != nil && c.Request.URL != nil {
		other["request_path"] = c.Request.URL.Path
	}
	other["error_type"] = err.GetErrorType()
	other["error_code"] = err.GetErrorCode()
	other["status_code"] = err.StatusCode
	other["channel_id"] = channelId
	other["channel_name"] = channelError.ChannelName
	other["channel_type"] = channelError.ChannelType
	appendErrorLogRequestConversion(relayInfo, other)
	adminInfo := make(map[string]interface{})
	useChannel := c.GetStringSlice("use_channel")
	adminInfo["use_channel"] = useChannel
	adminInfo["attempt_count"] = len(useChannel)
	if channelError.IsMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
	}
	service.AppendChannelAffinityAdminInfo(c, adminInfo)
	other["admin_info"] = adminInfo
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	elapsed := time.Since(startTime)
	if elapsed < 0 {
		elapsed = 0
	}
	other["use_time_ms"] = float64(elapsed.Milliseconds())
	useTimeSeconds := int(elapsed.Seconds())
	model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	var relayInfo *relaycommon.RelayInfo
	var metricErr *types.NewAPIError
	service.BeginChannelMetricRequest(c)
	service.MarkChannelMetricTaskRequest(c)
	defer func() {
		service.FinishChannelMetricRequest(c, relayInfo, metricErr)
	}()

	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		metricErr = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	common.SetContextKey(c, constant.ContextKeyRelayInfo, relayInfo)
	service.BindChannelMetricRelayInfo(c, relayInfo)

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		metricErr = taskErrorToChannelMetricError(taskErr)
		respondTaskError(c, taskErr)
		return
	}
	// Task responses remain buffered until their accounting ownership is durable.
	capture := &imageTaskResponseCapture{ResponseWriter: c.Writer}
	c.Writer = capture
	defer func() {
		c.Writer = capture.ResponseWriter
		capture.flush()
	}()

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	var pendingTaskFailure *channelFailureSnapshot
	var pendingTaskError *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}
	maxRetries := service.RelayMaxRetries(retryParam)
	if relayInfo.LockedChannel != nil {
		// remix / continuation 必须绑定原任务渠道，不使用跨组预算。
		maxRetries = common.RetryTimes
	}

	for attemptIndex := 0; attemptIndex <= maxRetries; attemptIndex++ {
		relayInfo.RetryIndex = attemptIndex
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
					if pendingTaskFailure != nil {
						finalizePendingChannelFailure(c, relayInfo, pendingTaskFailure)
						taskErr = pendingTaskError
						pendingTaskFailure = nil
						pendingTaskError = nil
					} else {
						taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					}
					break
				}
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				if pendingTaskFailure != nil {
					finalizePendingChannelFailure(c, relayInfo, pendingTaskFailure)
					taskErr = pendingTaskError
					pendingTaskFailure = nil
					pendingTaskError = nil
				} else {
					taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				}
				break
			}
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)
		relayInfo.ResetAttemptState(time.Now())
		service.BeginChannelMetricAttempt(c, relayInfo, channel.Id, channel.Name, channel.Type)

		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			break
		}
		capture.buf.Reset()
		capture.statusCode = 0
		capture.wroteHeader = false

		shouldRetry := shouldRetryTaskRelay(c, channel.Id, taskErr, remainingRelayRetries(maxRetries, attemptIndex))
		if !taskErr.LocalError {
			channelError := *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
				common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan())
			channelAPIError := types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode)
			processChannelError(c,
				relayInfo,
				channelError,
				channelAPIError,
				!shouldRetry)
			if shouldRetry {
				pendingTaskFailure = &channelFailureSnapshot{channel: channelError, err: channelAPIError}
				pendingTaskError = taskErr
				excludeChannelFromRetry(c, retryParam, channel, channelAPIError)
			} else {
				pendingTaskFailure = nil
				pendingTaskError = nil
			}
		}

		metricErr = taskErrorToChannelMetricError(taskErr)
		service.FinishChannelMetricAttempt(c, relayInfo, metricErr, shouldRetry, taskErr.Code)
		if !shouldRetry {
			break
		}
		retryParam.IncreaseRetry()
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// Persist the task, reservation adjustment, counters and log event together.
	if taskErr == nil {
		task := model.InitTask(result.Platform, relayInfo)
		if result.PublicTaskID != "" {
			task.TaskID = result.PublicTaskID
		}
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.RequestPath = relayInfo.RequestURLPath
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.TokenId = relayInfo.TokenId
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:      relayInfo.PriceData.ModelPrice,
			ModelPriceUnit:  relayInfo.PriceData.ModelPriceUnit,
			GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:      relayInfo.PriceData.ModelRatio,
			OtherRatios:     relayInfo.PriceData.OtherRatios,
			BillingMeta:     relayInfo.PriceData.BillingMeta,
			OriginModelName: relayInfo.OriginModelName,
			PerCallBilling:  common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice || relayInfo.TieredBillingSnapshot != nil,
		}
		task.PrivateData.TieredBillingSnapshot = relayInfo.TieredBillingSnapshot
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if handoffErr := service.HandoffTaskBilling(c, relayInfo, task, "", result.Quota); handoffErr != nil {
			common.SysError("persist task billing error: " + handoffErr.Error())
			taskErr = service.TaskErrorWrapperLocal(handoffErr, "persist_task_billing_failed", http.StatusInternalServerError)
			capture.buf.Reset()
			capture.statusCode = 0
			capture.wroteHeader = false
			service.AttachChannelMetricUsageAfterSettlement(c, service.ChannelMetricUsage{}, result.Quota, handoffErr)
		} else {
			service.AttachChannelMetricUsageAfterSettlement(c, service.ChannelMetricUsage{}, result.Quota, nil)
			service.FinishChannelMetricAttempt(c, relayInfo, nil, false, "")
			metricErr = nil
		}
	}

	if taskErr != nil {
		metricErr = taskErrorToChannelMetricError(taskErr)
		respondTaskError(c, taskErr)
	}
}

func taskErrorToChannelMetricError(taskErr *dto.TaskError) *types.NewAPIError {
	if taskErr == nil {
		return nil
	}
	err := taskErr.Error
	if err == nil {
		err = errors.New(taskErr.Message)
	}
	errorCode := types.ErrorCodeBadResponseStatusCode
	if taskErr.LocalError {
		errorCode = types.ErrorCodeConvertRequestFailed
	}
	return types.NewOpenAIError(err, errorCode, taskErr.StatusCode)
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if requestContextRetryBlockReason(c) != "" {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
