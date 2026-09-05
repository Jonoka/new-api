package controller

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var (
	relayImageTaskRelay = Relay
	insertImageTask     = func(task *model.Task) error { return task.Insert() }
	handoffImageTask    = service.HandoffTaskBilling
)

// RelayImageTaskSubmit handles upstreams that expose async image jobs through
// the OpenAI-compatible /v1/images/generations submit path.  The normal image
// relay path writes the submit response and charges the request, but it does
// not persist a row in tasks, so later GET /v1/images/generations/{task_id}
// cannot find the returned public task id.  This task-shaped submit path keeps
// billing, channel selection, and retries in Relay, then persists the returned
// task metadata for polling.
func RelayImageTaskSubmit(c *gin.Context) {
	var accountingErr error
	defer func() {
		if value, ok := c.Get("relay_info"); ok {
			if info, ok := value.(*relaycommon.RelayInfo); ok {
				finishImageMetricFinalization(info, accountingErr, c.Writer.Status())
			}
		}
	}()
	c.Set(relaycommon.ContextKeyDeferTaskBilling, true)
	capture := &imageTaskResponseCapture{ResponseWriter: c.Writer}
	c.Writer = capture
	relayImageTaskRelay(c, types.RelayFormatOpenAIImage)
	c.Writer = capture.ResponseWriter
	if capture.Status() < http.StatusOK || capture.Status() >= http.StatusMultipleChoices {
		capture.flush()
		return
	}

	value, ok := c.Get("relay_info")
	if !ok {
		common.SysLog("skip image task insert: relay info missing after successful response")
		capture.flush()
		return
	}
	relayInfo, ok := value.(*relaycommon.RelayInfo)
	if !ok || relayInfo == nil {
		common.SysLog("skip image task insert: invalid relay info after successful response")
		capture.flush()
		return
	}

	body := capture.Body()
	taskID, upstreamTaskID, taskStatus, progress := parseImageTaskSubmitResponseBody(body)
	if strings.TrimSpace(taskID) == "" {
		common.SysLog(fmt.Sprintf("skip image task insert: no task id in response, status=%d, body=%s", capture.Status(), common.LocalLogPreview(string(body))))
		if relayInfo.ChannelOtherSettings.ImageAsyncMode != dto.ImageAsyncModeTasksEndpoint {
			if err := service.CompleteDeferredImageBilling(c, relayInfo); err != nil {
				accountingErr = err
				common.SysError("complete image billing error: " + err.Error())
				refundImageSubmit(c, relayInfo)
				capture.statusCode = http.StatusInternalServerError
				capture.buf.Reset()
				_, _ = capture.buf.WriteString(`{"error":{"message":"failed to settle image request"}}`)
			}
			capture.flush()
			return
		}
		refundImageSubmit(c, relayInfo)
		accountingErr = types.NewErrorWithStatusCode(fmt.Errorf("upstream returned no image task id"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		capture.statusCode = http.StatusBadGateway
		capture.buf.Reset()
		_, _ = capture.buf.WriteString(`{"error":{"message":"upstream returned no image task id"}}`)
		capture.flush()
		return
	}
	if strings.TrimSpace(upstreamTaskID) == "" {
		upstreamTaskID = taskID
	}

	task := model.InitTask(constant.TaskPlatform(c.GetString("platform")), relayInfo)
	if relayInfo.ChannelOtherSettings.ImageAsyncMode == dto.ImageAsyncModeTasksEndpoint {
		task.Platform = constant.TaskPlatformImage
	} else if relayInfo.ChannelType > 0 {
		task.Platform = constant.TaskPlatform(strconv.Itoa(relayInfo.ChannelType))
	}
	task.TaskID = taskID
	task.PrivateData.UpstreamTaskID = upstreamTaskID
	task.PrivateData.RequestPath = c.Request.URL.Path
	freezeImageTaskPollingProtocol(task, relayInfo)
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
	task.Quota = relayInfo.PriceData.Quota
	if relayInfo.FinalPreConsumedQuota > 0 {
		task.Quota = relayInfo.FinalPreConsumedQuota
	}
	if relayInfo.TaskRelayInfo != nil {
		task.Action = relayInfo.TaskRelayInfo.Action
	}
	if task.Action == "" {
		task.Action = constant.TaskActionTextGenerate
	}
	if taskStatus != "" {
		task.Status = model.TaskStatus(taskStatus)
	}
	if progress != "" {
		task.Progress = progress
	}
	if len(body) > 0 {
		task.Data = body
	}
	terminalStatus := task.Status
	if terminalStatus == model.TaskStatusSuccess || terminalStatus == model.TaskStatusFailure {
		task.Status = model.TaskStatusSubmitted
		if task.Progress == "100%" {
			task.Progress = "0%"
		}
	}
	chargedQuota := relayInfo.FinalPreConsumedQuota
	if relayInfo.Billing == nil {
		chargedQuota = task.Quota
	}
	if relayInfo.TaskBillingActualQuota != nil {
		chargedQuota = *relayInfo.TaskBillingActualQuota
	}
	if handoffErr := handoffImageTask(c, relayInfo, task, "", chargedQuota); handoffErr != nil {
		accountingErr = handoffErr
		common.SysError("insert image task error: " + handoffErr.Error())
		refundImageSubmit(c, relayInfo)
		capture.statusCode = http.StatusInternalServerError
		capture.buf.Reset()
		_, _ = capture.buf.WriteString(`{"error":{"message":"failed to persist image task"}}`)
		capture.flush()
		return
	}
	if terminalStatus == model.TaskStatusSuccess || terminalStatus == model.TaskStatusFailure {
		expectedStatus := task.Status
		task.Status = terminalStatus
		task.Progress = "100%"
		task.FinishTime = time.Now().Unix()
		finalQuota := chargedQuota
		reason := "upstream completed during async image submission"
		if relayInfo.TaskBillingActualQuota != nil {
			finalQuota = *relayInfo.TaskBillingActualQuota
			reason = "image response actual usage"
		}
		if terminalStatus == model.TaskStatusFailure {
			finalQuota = 0
			reason = "upstream failed during async image submission"
		}
		if _, err := service.FinalizeTaskAccounting(c.Request.Context(), task, expectedStatus, finalQuota, reason); err != nil {
			accountingErr = err
			common.SysError("finalize terminal image task error: " + err.Error())
			capture.statusCode = http.StatusInternalServerError
			capture.buf.Reset()
			_, _ = capture.buf.WriteString(`{"error":{"message":"failed to settle image task"}}`)
			capture.flush()
			return
		}
		relayInfo.PriceData.Quota = task.Quota
	}
	capture.flush()
	common.SysLog(fmt.Sprintf("insert image task success: task_id=%s upstream_task_id=%s channel_id=%d status=%s", task.TaskID, task.PrivateData.UpstreamTaskID, task.ChannelId, task.Status))
}

func refundImageSubmit(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo != nil && relayInfo.Billing != nil {
		relayInfo.Billing.Refund(c)
	}
}

func freezeImageTaskPollingProtocol(task *model.Task, relayInfo *relaycommon.RelayInfo) {
	if task == nil || relayInfo == nil || relayInfo.ChannelOtherSettings.ImageAsyncMode != dto.ImageAsyncModeTasksEndpoint {
		return
	}
	endpoint := relayInfo.ChannelOtherSettings.ImageTasksSubmitPath()
	task.PrivateData.PollPath = strings.TrimRight(endpoint, "/") + "/{task_id}"
	task.PrivateData.TaskProtocol = model.TaskProtocolOpenAIImageTasks
}

func parseImageTaskSubmitResponseBody(body []byte) (taskID, upstreamTaskID, status, progress string) {
	if len(body) == 0 {
		return
	}
	var resp struct {
		ID       string `json:"id"`
		TaskID   string `json:"task_id"`
		Status   string `json:"status"`
		Progress any    `json:"progress"`
		Data     []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &resp); err != nil {
		common.SysLog("skip image task insert: parse response failed: " + err.Error() + ", body=" + common.LocalLogPreview(string(body)))
		return
	}
	taskID = strings.TrimSpace(resp.TaskID)
	upstreamTaskID = strings.TrimSpace(resp.ID)
	if taskID == "" {
		taskID = upstreamTaskID
	}
	if upstreamTaskID == "" {
		upstreamTaskID = taskID
	}
	if !strings.HasPrefix(taskID, "task_") && strings.HasPrefix(upstreamTaskID, "task_") {
		taskID, upstreamTaskID = upstreamTaskID, taskID
	}
	status = strings.ToUpper(strings.TrimSpace(resp.Status))
	switch strings.ToLower(status) {
	case "queued", "pending", "not_start", "not-start":
		status = string(model.TaskStatusQueued)
	case "running", "processing", "in_progress", "in-progress":
		status = string(model.TaskStatusInProgress)
	case "succeeded", "success", "completed", "complete":
		status = string(model.TaskStatusSuccess)
	case "failed", "failure", "error":
		status = string(model.TaskStatusFailure)
	}
	progress = normalizeImageTaskProgress(resp.Progress)
	if imageTaskSubmitHasResultData(resp.Data) || status == string(model.TaskStatusSuccess) || status == string(model.TaskStatusFailure) {
		progress = "100%"
		if status == "" {
			status = string(model.TaskStatusSuccess)
		}
	}
	return
}

func imageTaskSubmitHasResultData(data []struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json"`
}) bool {
	for _, item := range data {
		if strings.TrimSpace(item.URL) != "" || strings.TrimSpace(item.B64JSON) != "" {
			return true
		}
	}
	return false
}

func normalizeImageTaskProgress(progress any) string {
	switch v := progress.(type) {
	case string:
		p := strings.TrimSpace(v)
		if p == "" {
			return "0%"
		}
		if strings.HasSuffix(p, "%") {
			return p
		}
		return p + "%"
	case float64:
		return fmt.Sprintf("%.0f%%", v)
	case int:
		return fmt.Sprintf("%d%%", v)
	case int64:
		return fmt.Sprintf("%d%%", v)
	default:
		return "0%"
	}
}

func RelayImageTaskFetch(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "failed to load image task"}})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "task not found"}})
		return
	}
	c.JSON(http.StatusOK, buildRelayImageTaskResponse(task))
}

func buildRelayImageTaskResponse(task *model.Task) gin.H {
	status := strings.ToLower(strings.TrimSpace(string(task.Status)))
	switch status {
	case "success":
		status = "succeeded"
	case "failure":
		status = "failed"
	case "submitted", "queued", "not_start", "not-start":
		status = "queued"
	case "in_progress", "in-progress", "processing":
		status = "processing"
	case "":
		status = "queued"
	}
	response := gin.H{
		"id":       task.TaskID,
		"task_id":  task.TaskID,
		"status":   status,
		"progress": task.Progress,
	}
	if task.Status == model.TaskStatusFailure {
		response["error"] = gin.H{"message": task.FailReason}
		response["msg"] = task.FailReason
	}
	if (task.Platform == constant.TaskPlatformCanvasImage || task.PrivateData.ClientPlatform == string(constant.TaskPlatformCanvasImage)) && task.Status == model.TaskStatusSuccess && len(bytes.TrimSpace(task.Data)) > 0 {
		for key, value := range buildRelayCanvasImageTaskResult(task) {
			response[key] = value
		}
		response["id"] = task.TaskID
		response["task_id"] = task.TaskID
		response["status"] = status
		return response
	}
	if task.Status == model.TaskStatusSuccess && len(bytes.TrimSpace(task.Data)) > 0 {
		var payload gin.H
		if err := common.Unmarshal(task.Data, &payload); err == nil {
			for key, value := range payload {
				response[key] = value
			}
			response["id"] = task.TaskID
			response["task_id"] = task.TaskID
			response["status"] = status
		}
	}
	return response
}

func buildRelayCanvasImageTaskResult(task *model.Task) gin.H {
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
		switch {
		case strings.HasPrefix(strings.TrimSpace(item.URL), "data:"):
			next["url"] = signedCanvasImageTaskContentPath(task, index)
		case strings.TrimSpace(item.URL) != "":
			next["url"] = item.URL
		case strings.TrimSpace(item.B64JSON) != "":
			next["url"] = signedCanvasImageTaskContentPath(task, index)
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

func signedCanvasImageTaskContentPath(task *model.Task, index int) string {
	if task == nil {
		return canvasImageTaskContentPath("", index)
	}
	expires := time.Now().Add(30 * time.Minute).Unix()
	path := canvasImageTaskContentPath(task.TaskID, index)
	return fmt.Sprintf("%s?user_id=%d&expires=%d&token=%s", path, task.UserId, expires, signCanvasImageTaskContentToken(task.TaskID, task.UserId, index, expires))
}

func validateCanvasImageTaskContentToken(c *gin.Context, taskID string, index int) (int, bool) {
	expires, err := strconv.ParseInt(strings.TrimSpace(c.Query("expires")), 10, 64)
	if err != nil || expires < time.Now().Unix() {
		return 0, false
	}
	userID, err := strconv.Atoi(strings.TrimSpace(c.Query("user_id")))
	if err != nil || userID <= 0 {
		return 0, false
	}
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		return 0, false
	}
	expected := signCanvasImageTaskContentToken(taskID, userID, index, expires)
	return userID, hmac.Equal([]byte(token), []byte(expected))
}

func signCanvasImageTaskContentToken(taskID string, userID int, index int, expires int64) string {
	payload := fmt.Sprintf("%s:%d:%d:%d", taskID, userID, index, expires)
	mac := hmac.New(sha256.New, []byte(common.SessionSecret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

type imageTaskResponseCapture struct {
	gin.ResponseWriter
	buf         bytes.Buffer
	statusCode  int
	wroteHeader bool
}

func (w *imageTaskResponseCapture) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = code
	w.wroteHeader = true
}

func (w *imageTaskResponseCapture) Status() int {
	if w.statusCode == 0 {
		return http.StatusOK
	}
	return w.statusCode
}

func (w *imageTaskResponseCapture) flush() {
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(w.Status())
	if w.buf.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.buf.Bytes())
	}
}

// Flush intentionally buffers upstream output. Some relay copy helpers flush
// after writing; committing the real writer here would make persistence errors
// impossible to report to the client.
func (w *imageTaskResponseCapture) Flush() {}

func (w *imageTaskResponseCapture) WriteHeaderNow() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
}

func (w *imageTaskResponseCapture) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.buf.Write(data)
}

func (w *imageTaskResponseCapture) WriteString(s string) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.buf.WriteString(s)
}

func (w *imageTaskResponseCapture) Body() []byte {
	return w.buf.Bytes()
}
