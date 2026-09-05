package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	canvasImageTaskActionGenerations = "images/generations"
	canvasImageTaskActionEdits       = "images/edits"
	canvasImageTaskRelayPrefix       = "/canvas/v1"
	apiImageTaskRelayPrefix          = "/v1"
)

var updateCanvasImageTask = func(task *model.Task) error { return task.Update() }

type canvasImageTaskRelayRequest struct {
	TaskID      string
	Action      string
	RelayPrefix string
	Body        []byte
	Header      http.Header
	RawQuery    string
	Keys        map[string]any
	Context     context.Context
}

var startCanvasImageTaskRelay = func(relayReq canvasImageTaskRelayRequest) {
	go runCanvasImageTaskRelay(relayReq)
}

func CanvasImageTaskSubmit(c *gin.Context) {
	submitImageTask(
		c,
		canvasImageTaskAction(c),
		constant.TaskPlatformCanvasImage,
		canvasImageTaskRelayPrefix,
	)
}

// ImageTaskSubmit 为令牌 API 提供与 Canvas 相同的异步图片任务能力。
func ImageTaskSubmit(c *gin.Context) {
	action, ok := parseImageTaskAction(c.Query("action"))
	if !ok {
		abortCanvasRequest(c, http.StatusBadRequest, "unsupported image task action")
		return
	}
	submitImageTask(c, action, constant.TaskPlatformImage, apiImageTaskRelayPrefix)
}

func submitImageTask(c *gin.Context, action string, platform constant.TaskPlatform, relayPrefix string) {
	body, err := readCanvasImageTaskBody(c)
	if err != nil {
		abortCanvasRequest(c, http.StatusBadRequest, err.Error())
		return
	}
	group := imageTaskGroup(c, relayPrefix)

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     model.GenerateTaskID(),
		Platform:   platform,
		UserId:     c.GetInt("id"),
		Group:      group,
		Action:     action,
		Status:     model.TaskStatusQueued,
		Progress:   "0%",
		SubmitTime: now,
	}
	if err := task.Insert(); err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to create image task")
		return
	}

	relayReq := canvasImageTaskRelayRequest{
		TaskID:      task.TaskID,
		Action:      action,
		RelayPrefix: relayPrefix,
		Body:        append([]byte(nil), body...),
		Header:      c.Request.Header.Clone(),
		RawQuery:    imageTaskRelayRawQuery(c),
		Keys:        cloneCanvasImageTaskKeys(c.Keys),
	}
	startCanvasImageTaskRelay(relayReq)

	c.JSON(http.StatusAccepted, gin.H{
		"task_id": task.TaskID,
		"status":  mapCanvasImageTaskStatus(task.Status),
	})
}

func CanvasImageTaskFetch(c *gin.Context) {
	fetchImageTask(c, constant.TaskPlatformCanvasImage, canvasImageTaskRelayPrefix)
}

// ImageTaskFetch 查询令牌 API 提交的异步图片任务。
func ImageTaskFetch(c *gin.Context) {
	fetchImageTask(c, constant.TaskPlatformImage, apiImageTaskRelayPrefix)
}

func fetchImageTask(c *gin.Context, platform constant.TaskPlatform, responsePrefix string) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists || task.Platform != platform {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}

	c.JSON(http.StatusOK, buildImageTaskResponse(task, responsePrefix))
}

func CanvasImageTaskContent(c *gin.Context) {
	serveImageTaskContent(c, constant.TaskPlatformCanvasImage)
}

// ImageTaskContent 返回令牌 API 异步图片任务中暂存的图片内容。
func ImageTaskContent(c *gin.Context) {
	serveImageTaskContent(c, constant.TaskPlatformImage)
}

func serveImageTaskContent(c *gin.Context, platform constant.TaskPlatform) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	index, err := strconv.Atoi(strings.TrimSpace(c.Param("index")))
	if taskID == "" || err != nil || index < 0 {
		abortCanvasRequest(c, http.StatusBadRequest, "invalid image content request")
		return
	}

	userID := c.GetInt("id")
	if platform == constant.TaskPlatformCanvasImage {
		var ok bool
		userID, ok = validateCanvasImageTaskContentToken(c, taskID, index)
		if !ok {
			abortCanvasRequest(c, http.StatusUnauthorized, "Unauthorized")
			return
		}
	}

	task, exists, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists || task.Platform != platform {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}
	writeImageTaskContent(c, task, index)
}

func writeImageTaskContent(c *gin.Context, task *model.Task, index int) {
	if task.Status != model.TaskStatusSuccess {
		abortCanvasRequest(c, http.StatusBadRequest, "image task is not completed")
		return
	}
	if imageTaskDataExpired(task, time.Now().Unix()) || len(bytes.TrimSpace(task.Data)) == 0 {
		abortCanvasRequest(c, http.StatusGone, "image task data has expired")
		return
	}

	image, mimeType, err := readCanvasImageTaskContent(task, index)
	if err != nil {
		abortCanvasRequest(c, http.StatusNotFound, "image content not found")
		return
	}

	c.Header("Cache-Control", fmt.Sprintf("private, max-age=%d", imageTaskContentMaxAge(task, time.Now().Unix())))
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mimeType, image)
}

func imageTaskRelayRawQuery(c *gin.Context) string {
	query := c.Request.URL.Query()
	query.Del("action")
	return query.Encode()
}

func imageTaskGroup(c *gin.Context, relayPrefix string) string {
	if relayPrefix == canvasImageTaskRelayPrefix {
		return strings.TrimSpace(c.Query("group"))
	}
	return common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
}

func readCanvasImageTaskBody(c *gin.Context) ([]byte, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

func cloneCanvasImageTaskKeys(keys map[string]any) map[string]any {
	next := make(map[string]any, len(keys))
	for key, value := range keys {
		if key == common.KeyBodyStorage || key == common.KeyRequestBody {
			continue
		}
		next[key] = value
	}
	return next
}

func runCanvasImageTaskRelay(relayReq canvasImageTaskRelayRequest) {
	timeout := time.Duration(constant.ImageTaskTimeoutMinutes) * time.Minute
	runCanvasImageTaskRelayWithExecutor(relayReq, timeout, executeCanvasImageRelay)
}

func runCanvasImageTaskRelayWithExecutor(
	relayReq canvasImageTaskRelayRequest,
	timeout time.Duration,
	execute func(canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int, *relaycommon.RelayInfo),
) {
	task, exists, err := model.GetByTaskId(canvasImageTaskUserID(relayReq.Keys), relayReq.TaskID)
	if err != nil || !exists {
		return
	}
	var liveRelayInfo *relaycommon.RelayInfo
	defer func() {
		if recovered := recover(); recovered != nil {
			common.SysError(fmt.Sprintf("canvas image task panic: %v", recovered))
			refundCanvasImageSubmit(liveRelayInfo)
			failCanvasImageTask(task, fmt.Sprintf("image generation failed: %v", recovered), nil)
		}
	}()

	now := time.Now().Unix()
	expectedStatus := task.Status
	task.Status = model.TaskStatusInProgress
	task.StartTime = now
	task.UpdatedAt = now
	task.Progress = "10%"
	won, err := task.UpdateWithStatus(expectedStatus)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to start image task %s: %v", task.TaskID, err))
		return
	}
	if !won {
		return
	}

	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		relayReq.Context = ctx
	} else {
		relayReq.Context = context.Background()
	}

	recorder, channelID, relayInfo := execute(relayReq)
	liveRelayInfo = relayInfo
	if errors.Is(relayReq.Context.Err(), context.DeadlineExceeded) {
		refundCanvasImageSubmit(relayInfo)
		failCanvasImageTask(task, imageTaskTimeoutReason(timeout), nil)
		return
	}
	finishCanvasImageTask(task, channelID, recorder, relayInfo)
}

func imageTaskTimeoutReason(timeout time.Duration) string {
	if timeout >= time.Minute && timeout%time.Minute == 0 {
		return fmt.Sprintf("image generation timed out after %d minutes", int(timeout/time.Minute))
	}
	return "image generation timed out"
}

func executeCanvasImageRelay(relayReq canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int, *relaycommon.RelayInfo) {
	return executeCanvasImageRelayWithHandler(relayReq, nil)
}

func executeCanvasImageRelayWithHandler(relayReq canvasImageTaskRelayRequest, handler gin.HandlerFunc) (*httptest.ResponseRecorder, int, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	engine := gin.New()
	channelID := 0
	var relayInfo *relaycommon.RelayInfo
	action := normalizeCanvasImageTaskAction(relayReq.Action)
	relayPrefix := strings.TrimRight(strings.TrimSpace(relayReq.RelayPrefix), "/")
	if relayPrefix == "" {
		relayPrefix = canvasImageTaskRelayPrefix
	}
	targetPath := relayPrefix + "/" + action

	engine.Use(func(c *gin.Context) {
		for key, value := range relayReq.Keys {
			c.Set(key, value)
		}
		c.Set(relaycommon.ContextKeyDeferTaskBilling, true)
		c.Next()
		channelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		if value, ok := c.Get("relay_info"); ok {
			if info, ok := value.(*relaycommon.RelayInfo); ok {
				relayInfo = info
			}
		}
	})
	engine.Use(middleware.BodyStorageCleanup())
	if handler != nil {
		engine.POST(targetPath, handler)
	} else {
		engine.POST(targetPath,
			middleware.Distribute(),
			middleware.ModelRequestRateLimit(),
			func(c *gin.Context) {
				Relay(c, types.RelayFormatOpenAIImage)
			},
		)
	}
	targetURL := targetPath
	if relayReq.RawQuery != "" {
		targetURL += "?" + relayReq.RawQuery
	}
	requestContext := relayReq.Context
	if requestContext == nil {
		requestContext = context.Background()
	}
	request := httptest.NewRequest(http.MethodPost, targetURL, bytes.NewReader(relayReq.Body)).WithContext(requestContext)
	request.Header = relayReq.Header.Clone()
	request.ContentLength = int64(len(relayReq.Body))
	engine.ServeHTTP(recorder, request)

	return recorder, channelID, relayInfo
}

func canvasImageTaskAction(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return canvasImageTaskActionGenerations
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		return normalizeCanvasImageTaskAction(action)
	}
	return normalizeCanvasImageTaskAction(strings.TrimPrefix(c.Request.URL.Path, "/canvas/v1/"))
}

func normalizeCanvasImageTaskAction(action string) string {
	normalized, ok := parseImageTaskAction(action)
	if ok {
		return normalized
	}
	return canvasImageTaskActionGenerations
}

func parseImageTaskAction(action string) (string, bool) {
	switch strings.Trim(strings.TrimSpace(action), "/") {
	case "", "generations", canvasImageTaskActionGenerations:
		return canvasImageTaskActionGenerations, true
	case "edits", canvasImageTaskActionEdits:
		return canvasImageTaskActionEdits, true
	default:
		return "", false
	}
}

func finishCanvasImageTask(task *model.Task, channelID int, recorder *httptest.ResponseRecorder, relayInfo *relaycommon.RelayInfo) {
	expectedStatus := task.Status
	body := bytes.TrimSpace(recorder.Body.Bytes())
	if recorder.Code >= http.StatusOK && recorder.Code < http.StatusMultipleChoices && relayInfo != nil && relayInfo.ChannelOtherSettings.ImageAsyncMode == dto.ImageAsyncModeTasksEndpoint {
		_, upstreamTaskID, status, progress := parseImageTaskSubmitResponseBody(body)
		if upstreamTaskID != "" && (status == "" || status == string(model.TaskStatusSubmitted) || status == string(model.TaskStatusQueued) || status == string(model.TaskStatusInProgress)) {
			if status == "" || status == string(model.TaskStatusSubmitted) {
				status = string(model.TaskStatusQueued)
			}
			if progress == "" {
				progress = "0%"
			}
			now := time.Now().Unix()
			task.UpdatedAt = now
			task.Status = model.TaskStatus(status)
			task.Progress = progress
			task.ChannelId = channelID
			task.Platform = constant.TaskPlatformImage
			task.PrivateData.ClientPlatform = string(constant.TaskPlatformCanvasImage)
			task.PrivateData.UpstreamTaskID = upstreamTaskID
			freezeImageTaskPollingProtocol(task, relayInfo)
			applyCanvasImageTaskBillingSnapshot(task, relayInfo)
			task.Data = json.RawMessage(append([]byte(nil), body...))
			chargedQuota := relayInfo.FinalPreConsumedQuota
			if relayInfo.Billing == nil {
				chargedQuota = task.Quota
			}
			if err := service.HandoffTaskBilling(nil, relayInfo, task, expectedStatus, chargedQuota); err != nil {
				common.SysError(fmt.Sprintf("canvas image async handoff update failed: task_id=%s err=%v", task.TaskID, err))
				refundCanvasImageSubmit(relayInfo)
				failCanvasImageTask(task, "failed to persist upstream async image task", body)
			}
			return
		}
		if status == string(model.TaskStatusFailure) {
			applyCanvasImageTaskBillingSnapshot(task, relayInfo)
			task.Data = json.RawMessage(append([]byte(nil), body...))
			task.Status = expectedStatus
			chargedQuota := relayInfo.FinalPreConsumedQuota
			if relayInfo.Billing == nil {
				chargedQuota = task.Quota
			}
			if err := service.HandoffTaskBilling(nil, relayInfo, task, expectedStatus, chargedQuota); err != nil {
				refundCanvasImageSubmit(relayInfo)
				failCanvasImageTask(task, extractCanvasImageRelayError(body), body)
				return
			}
			task.Status = model.TaskStatusFailure
			task.Progress = "100%"
			task.FinishTime = time.Now().Unix()
			task.FailReason = extractCanvasImageRelayError(body)
			if _, err := service.FinalizeTaskAccounting(context.Background(), task, expectedStatus, 0, task.FailReason); err != nil {
				common.SysError(fmt.Sprintf("canvas terminal failure accounting failed: task_id=%s err=%v", task.TaskID, err))
			}
			return
		}
		if status != string(model.TaskStatusSuccess) {
			reason := "upstream returned no image task id"
			refundCanvasImageSubmit(relayInfo)
			failCanvasImageTask(task, reason, body)
			return
		}
	}
	now := time.Now().Unix()
	task.FinishTime = now
	task.UpdatedAt = now
	task.Progress = "100%"
	if channelID > 0 {
		task.ChannelId = channelID
	}
	applyCanvasImageTaskBillingSnapshot(task, relayInfo)

	if recorder.Code >= http.StatusOK && recorder.Code < http.StatusMultipleChoices && len(body) > 0 {
		if relayInfo == nil {
			failCanvasImageTask(task, "image relay billing context is missing", body)
			return
		}
		task.Data = json.RawMessage(append([]byte(nil), body...))
		task.FailReason = ""
		task.Status = expectedStatus
		chargedQuota := relayInfo.FinalPreConsumedQuota
		if relayInfo.Billing == nil {
			chargedQuota = task.Quota
		}
		if err := service.HandoffTaskBilling(nil, relayInfo, task, expectedStatus, chargedQuota); err != nil {
			common.SysError(fmt.Sprintf("failed to hand off image task %s: %v", task.TaskID, err))
			refundCanvasImageSubmit(relayInfo)
			failCanvasImageTask(task, "failed to persist image task billing", body)
			return
		}
		task.Status = model.TaskStatusSuccess
		finalQuota, reason := chargedQuota, "预扣额度保持不变"
		if relayInfo.TaskBillingActualQuota != nil {
			finalQuota = *relayInfo.TaskBillingActualQuota
			reason = "image response actual usage"
		}
		if relayInfo.TaskBillingActualQuota != nil {
			finalQuota = *relayInfo.TaskBillingActualQuota
		}
		if actualQuota, actualReason, ok := calculateCanvasImageTaskQuota(task, body, relayInfo); ok {
			finalQuota, reason = actualQuota, actualReason
		}
		if _, err := service.FinalizeTaskAccounting(context.Background(), task, expectedStatus, finalQuota, reason); err != nil {
			common.SysError(fmt.Sprintf("failed to finish image task %s: %v", task.TaskID, err))
		}
		return
	}

	failCanvasImageTask(task, extractCanvasImageRelayError(body), body)
}

func refundCanvasImageSubmit(relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil || relayInfo.Billing == nil {
		return
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo.Billing.Refund(c)
}

func applyCanvasImageTaskBillingSnapshot(task *model.Task, relayInfo *relaycommon.RelayInfo) {
	if task == nil || relayInfo == nil {
		return
	}
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
	if relayInfo.PriceData.Quota > 0 {
		task.Quota = relayInfo.PriceData.Quota
	}
	if relayInfo.FinalPreConsumedQuota > 0 {
		task.Quota = relayInfo.FinalPreConsumedQuota
	}
	if relayInfo.OriginModelName != "" {
		task.Properties.OriginModelName = relayInfo.OriginModelName
	}
	if relayInfo.UpstreamModelName != "" {
		task.Properties.UpstreamModelName = relayInfo.UpstreamModelName
	}
}

func calculateCanvasImageTaskQuota(task *model.Task, body []byte, relayInfo *relaycommon.RelayInfo) (int, string, bool) {
	if task == nil || task.PrivateData.TieredBillingSnapshot == nil || len(bytes.TrimSpace(body)) == 0 {
		return 0, "", false
	}
	var originalInput *billingexpr.RequestInput
	if relayInfo != nil {
		originalInput = relayInfo.BillingRequestInput
	}
	actualInput, actual, ok := buildCanvasImageActualBillingInput(task.PrivateData.TieredBillingSnapshot, body, originalInput)
	if !ok {
		return 0, "", false
	}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(task.PrivateData.TieredBillingSnapshot, billingexpr.TokenParams{
		P:   float64(task.PrivateData.TieredBillingSnapshot.EstimatedPromptTokens),
		C:   float64(task.PrivateData.TieredBillingSnapshot.EstimatedCompletionTokens),
		Len: float64(task.PrivateData.TieredBillingSnapshot.EstimatedPromptTokens),
	}, actualInput)
	if err != nil || result.ActualQuotaAfterGroup < 0 {
		common.SysError(fmt.Sprintf("canvas image task actual tiered billing failed: task_id=%s err=%v", task.TaskID, err))
		return 0, "", false
	}
	reason := fmt.Sprintf("image实际结果重算：requested_tier=%s, actual_tier=%s, actual_size=%s, actual_quality=%s, n=%d",
		task.PrivateData.TieredBillingSnapshot.EstimatedTier, result.MatchedTier, actual.Size, actual.Quality, actual.N)
	return result.ActualQuotaAfterGroup, reason, true
}

type canvasImageActualBillingMeta struct {
	Size    string
	Quality string
	N       int
}

func buildCanvasImageActualBillingInput(snap *billingexpr.BillingSnapshot, body []byte, originalInput *billingexpr.RequestInput) (billingexpr.RequestInput, canvasImageActualBillingMeta, bool) {
	if snap == nil {
		return billingexpr.RequestInput{}, canvasImageActualBillingMeta{}, false
	}
	var payload struct {
		Size    string `json:"size"`
		Quality string `json:"quality"`
		N       int    `json:"n"`
		Data    []any  `json:"data"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
		return billingexpr.RequestInput{}, canvasImageActualBillingMeta{}, false
	}
	actual := canvasImageActualBillingMeta{
		Size:    strings.TrimSpace(payload.Size),
		Quality: strings.TrimSpace(payload.Quality),
		N:       payload.N,
	}
	if actual.N <= 0 {
		if len(payload.Data) > 0 {
			actual.N = len(payload.Data)
		} else {
			actual.N = 1
		}
	}
	if actual.Size == "" && actual.Quality == "" {
		return billingexpr.RequestInput{}, canvasImageActualBillingMeta{}, false
	}
	requestBody := map[string]any{}
	var headers map[string]string
	if originalInput != nil {
		headers = originalInput.Headers
		if len(bytes.TrimSpace(originalInput.Body)) > 0 {
			_ = common.Unmarshal(originalInput.Body, &requestBody)
		}
	}
	requestBody["n"] = actual.N
	if actual.Size != "" {
		requestBody["size"] = actual.Size
	}
	if actual.Quality != "" {
		requestBody["quality"] = actual.Quality
	}
	bodyBytes, err := common.Marshal(requestBody)
	if err != nil {
		return billingexpr.RequestInput{}, canvasImageActualBillingMeta{}, false
	}
	return billingexpr.RequestInput{Headers: headers, Body: bodyBytes}, actual, true
}

func failCanvasImageTask(task *model.Task, reason string, body []byte) {
	expectedStatus := task.Status
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FinishTime = time.Now().Unix()
	task.UpdatedAt = task.FinishTime
	task.FailReason = reason
	if len(body) > 0 {
		task.Data = json.RawMessage(append([]byte(nil), body...))
	}
	if accounting, err := model.GetTaskAccounting(task.ID); err == nil && accounting.DecisionID == "" {
		if _, finalizeErr := service.FinalizeTaskAccounting(context.Background(), task, expectedStatus, 0, reason); finalizeErr != nil {
			common.SysError(fmt.Sprintf("failed to account image task failure %s: %v", task.TaskID, finalizeErr))
		}
		return
	}
	_, _ = task.UpdateWithStatus(expectedStatus)
}

func extractCanvasImageRelayError(body []byte) string {
	if len(bytes.TrimSpace(body)) == 0 {
		return "image generation failed"
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := common.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "image generation failed"
	}
	return common.LocalLogPreview(text)
}

func buildCanvasImageTaskResponse(task *model.Task) gin.H {
	return buildImageTaskResponse(task, canvasImageTaskRelayPrefix)
}

func buildAPIImageTaskResponse(task *model.Task) gin.H {
	return buildImageTaskResponse(task, apiImageTaskRelayPrefix)
}

func buildImageTaskResponse(task *model.Task, responsePrefix string) gin.H {
	response := gin.H{
		"task_id":  task.TaskID,
		"status":   mapCanvasImageTaskStatus(task.Status),
		"progress": task.Progress,
	}
	if expiresAt, ok := imageTaskDataExpiresAt(task); ok {
		response["expires_at"] = expiresAt
	}
	expired := imageTaskDataExpired(task, time.Now().Unix())
	if task.Status == model.TaskStatusSuccess && !expired && len(bytes.TrimSpace(task.Data)) > 0 {
		response["result"] = buildImageTaskResult(task, responsePrefix)
	}
	if task.Status == model.TaskStatusSuccess && (expired || len(bytes.TrimSpace(task.Data)) == 0) {
		response["result_expired"] = true
	}
	if task.Status == model.TaskStatusFailure {
		if structured := canvasImageTaskFailurePayload(task.Data); structured != nil {
			response["error"] = structured
		} else {
			response["error"] = task.FailReason
		}
		response["msg"] = task.FailReason
	}
	return response
}

func canvasImageTaskFailurePayload(data []byte) gin.H {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if err := common.Unmarshal(data, &payload); err != nil || strings.TrimSpace(payload.Error.Message) == "" {
		return nil
	}
	result := gin.H{"message": payload.Error.Message}
	if strings.TrimSpace(payload.Error.Type) != "" {
		result["type"] = payload.Error.Type
	}
	if payload.Error.Code != nil {
		result["code"] = payload.Error.Code
	}
	return result
}

func imageTaskDataExpiresAt(task *model.Task) (int64, bool) {
	retentionHours := common.GetImageTaskDataRetentionHours()
	if retentionHours <= 0 || task.FinishTime <= 0 {
		return 0, false
	}
	return task.FinishTime + int64(retentionHours)*int64(time.Hour/time.Second), true
}

func imageTaskDataExpired(task *model.Task, nowUnix int64) bool {
	expiresAt, ok := imageTaskDataExpiresAt(task)
	return ok && nowUnix >= expiresAt
}

func imageTaskContentMaxAge(task *model.Task, nowUnix int64) int64 {
	expiresAt, ok := imageTaskDataExpiresAt(task)
	if !ok {
		return int64((24 * time.Hour) / time.Second)
	}
	remaining := expiresAt - nowUnix
	if remaining < 0 {
		return 0
	}
	return remaining
}

func buildCanvasImageTaskResult(task *model.Task) gin.H {
	return buildImageTaskResult(task, canvasImageTaskRelayPrefix)
}

func buildImageTaskResult(task *model.Task, responsePrefix string) gin.H {
	var payload struct {
		Created any `json:"created,omitempty"`
		Data    []struct {
			URL           string `json:"url,omitempty"`
			B64JSON       string `json:"b64_json,omitempty"`
			RevisedPrompt string `json:"revised_prompt,omitempty"`
		} `json:"data"`
	}
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return gin.H{"data": []gin.H{}}
	}

	items := make([]gin.H, 0, len(payload.Data))
	for index, item := range payload.Data {
		next := gin.H{}
		itemURL := strings.TrimSpace(item.URL)
		switch {
		case strings.TrimSpace(item.B64JSON) != "" || isCanvasImageDataURL(itemURL):
			if responsePrefix == canvasImageTaskRelayPrefix {
				next["url"] = signedCanvasImageTaskContentPath(task, index)
			} else {
				next["url"] = imageTaskContentPath(responsePrefix, task.TaskID, index)
			}
		case itemURL != "":
			next["url"] = itemURL
		default:
			continue
		}
		if strings.TrimSpace(item.RevisedPrompt) != "" {
			next["revised_prompt"] = item.RevisedPrompt
		}
		items = append(items, next)
	}

	result := gin.H{"data": items}
	if payload.Created != nil {
		result["created"] = payload.Created
	}
	return result
}

func canvasImageTaskContentPath(taskID string, index int) string {
	return imageTaskContentPath(canvasImageTaskRelayPrefix, taskID, index)
}

func imageTaskContentPath(responsePrefix string, taskID string, index int) string {
	responsePrefix = strings.TrimRight(strings.TrimSpace(responsePrefix), "/")
	if responsePrefix == "" {
		responsePrefix = canvasImageTaskRelayPrefix
	}
	return fmt.Sprintf("%s/images/tasks/%s/content/%d", responsePrefix, url.PathEscape(taskID), index)
}

func readCanvasImageTaskContent(task *model.Task, index int) ([]byte, string, error) {
	var payload struct {
		Data []struct {
			URL     string `json:"url,omitempty"`
			B64JSON string `json:"b64_json,omitempty"`
		} `json:"data"`
	}
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return nil, "", err
	}
	if index < 0 || index >= len(payload.Data) {
		return nil, "", fmt.Errorf("image index out of range")
	}
	value := payload.Data[index].B64JSON
	if strings.TrimSpace(value) == "" && isCanvasImageDataURL(payload.Data[index].URL) {
		value = payload.Data[index].URL
	}
	return decodeCanvasImageData(value)
}

func isCanvasImageDataURL(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "data:image/")
}

func decodeCanvasImageData(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	declaredMIMEType := ""
	if strings.HasPrefix(value, "data:") {
		parts := strings.SplitN(value, ",", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid image data url")
		}
		header := strings.TrimPrefix(parts[0], "data:")
		if !strings.Contains(header, ";base64") {
			return nil, "", fmt.Errorf("unsupported image data url")
		}
		declaredMIMEType = strings.TrimSpace(strings.TrimSuffix(header, ";base64"))
		value = parts[1]
	}
	if value == "" {
		return nil, "", fmt.Errorf("empty image data")
	}
	image, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		image, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, "", err
	}

	detectedContentType := http.DetectContentType(image)
	detectedMIMEType, detectErr := normalizeCanvasImageMIMEType(detectedContentType)
	if declaredMIMEType == "" {
		if detectErr != nil {
			return nil, "", detectErr
		}
		return image, detectedMIMEType, nil
	}

	declaredMIMEType, err = normalizeCanvasImageMIMEType(declaredMIMEType)
	if err != nil {
		return nil, "", err
	}
	if detectErr == nil {
		if declaredMIMEType != detectedMIMEType {
			return nil, "", fmt.Errorf("image MIME type does not match content")
		}
		return image, detectedMIMEType, nil
	}
	// net/http 暂时无法识别部分现代图片格式（例如 AVIF）。只有在内容
	// 未被识别为其他类型时，才接受白名单内的声明类型。
	if detectedContentType == "application/octet-stream" {
		return image, declaredMIMEType, nil
	}
	return nil, "", detectErr
}

func normalizeCanvasImageMIMEType(mimeType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return "image/png", nil
	case "image/jpeg", "image/jpg":
		return "image/jpeg", nil
	case "image/webp":
		return "image/webp", nil
	case "image/gif":
		return "image/gif", nil
	case "image/avif":
		return "image/avif", nil
	default:
		return "", fmt.Errorf("unsupported image MIME type")
	}
}

func mapCanvasImageTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusInProgress:
		return "processing"
	case model.TaskStatusNotStart, model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func canvasImageTaskUserID(keys map[string]any) int {
	value, ok := keys[string(constant.ContextKeyUserId)]
	if !ok {
		return 0
	}
	id, _ := value.(int)
	return id
}
