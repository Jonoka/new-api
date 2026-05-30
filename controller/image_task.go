package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// RelayImageTaskSubmit handles upstreams that expose async image jobs through
// the OpenAI-compatible /v1/images/generations submit path.  The normal image
// relay path writes the submit response and charges the request, but it does
// not persist a row in tasks, so later GET /v1/images/generations/{task_id}
// cannot find the returned public task id.  This task-shaped submit path keeps
// billing and response behavior in ImageHelper, then persists the returned task
// metadata for polling.
func RelayImageTaskSubmit(c *gin.Context) {
	request, err := helper.GetAndValidateRequest(c, types.RelayFormatOpenAIImage)
	if err != nil {
		apiErr := types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAIImage, request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	meta := request.GetTokenCountMeta()
	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		apiErr := types.NewErrorWithStatusCode(err, types.ErrorCodeCountTokenFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
		return
	}
	relayInfo.SetEstimatePromptTokens(tokens)
	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		apiErr := types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
		return
	}
	if !priceData.FreeModel {
		if apiErr := service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo); apiErr != nil {
			c.JSON(apiErr.StatusCode, apiErr.ToOpenAIError())
			return
		}
	}

	capture := &imageTaskResponseCapture{ResponseWriter: c.Writer}
	c.Writer = capture
	apiErr := relayHandler(c, relayInfo)
	if apiErr != nil {
		if relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
		c.JSON(apiErr.StatusCode, apiErr.ToOpenAIError())
		return
	}

	body := capture.Body()
	taskID, upstreamTaskID, taskStatus, progress := parseImageTaskSubmitResponseBody(body)
	if strings.TrimSpace(taskID) == "" {
		common.SysLog(fmt.Sprintf("skip image task insert: no task id in response, status=%d, body=%s", capture.Status(), common.LocalLogPreview(string(body))))
		return
	}
	if strings.TrimSpace(upstreamTaskID) == "" {
		upstreamTaskID = taskID
	}

	task := model.InitTask(constant.TaskPlatform(c.GetString("platform")), relayInfo)
	task.TaskID = taskID
	task.PrivateData.UpstreamTaskID = upstreamTaskID
	task.PrivateData.RequestPath = c.Request.URL.Path
	task.PrivateData.BillingSource = relayInfo.BillingSource
	task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
	task.PrivateData.TokenId = relayInfo.TokenId
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      relayInfo.PriceData.ModelPrice,
		GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
		ModelRatio:      relayInfo.PriceData.ModelRatio,
		OtherRatios:     relayInfo.PriceData.OtherRatios,
		OriginModelName: relayInfo.OriginModelName,
		PerCallBilling:  relayInfo.PriceData.UsePrice || relayInfo.TieredBillingSnapshot != nil,
	}
	task.PrivateData.TieredBillingSnapshot = relayInfo.TieredBillingSnapshot
	task.Quota = relayInfo.PriceData.Quota
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
	if insertErr := task.Insert(); insertErr != nil {
		common.SysError("insert image task error: " + insertErr.Error())
		return
	}
	common.SysLog(fmt.Sprintf("insert image task success: task_id=%s upstream_task_id=%s channel_id=%d status=%s", task.TaskID, task.PrivateData.UpstreamTaskID, task.ChannelId, task.Status))
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
	progress = "0%"
	return
}

type imageTaskResponseCapture struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *imageTaskResponseCapture) Write(data []byte) (int, error) {
	w.buf.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *imageTaskResponseCapture) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return io.WriteString(w.ResponseWriter, s)
}

func (w *imageTaskResponseCapture) Body() []byte {
	return w.buf.Bytes()
}
