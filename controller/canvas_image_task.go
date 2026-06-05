package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const canvasImageTaskAction = "images/generations"

type canvasImageTaskRelayRequest struct {
	TaskID   string
	Body     []byte
	Header   http.Header
	RawQuery string
	Keys     map[string]any
}

func CanvasImageTaskSubmit(c *gin.Context) {
	body, err := readCanvasImageTaskBody(c)
	if err != nil {
		abortCanvasRequest(c, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     model.GenerateTaskID(),
		Platform:   constant.TaskPlatformCanvasImage,
		UserId:     c.GetInt("id"),
		Group:      strings.TrimSpace(c.Query("group")),
		Action:     canvasImageTaskAction,
		Status:     model.TaskStatusQueued,
		Progress:   "0%",
		SubmitTime: now,
	}
	if err := task.Insert(); err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to create image task")
		return
	}

	relayReq := canvasImageTaskRelayRequest{
		TaskID:   task.TaskID,
		Body:     append([]byte(nil), body...),
		Header:   c.Request.Header.Clone(),
		RawQuery: c.Request.URL.RawQuery,
		Keys:     cloneCanvasImageTaskKeys(c.Keys),
	}
	go runCanvasImageTaskRelay(relayReq)

	c.JSON(http.StatusAccepted, gin.H{
		"task_id": task.TaskID,
		"status":  mapCanvasImageTaskStatus(task.Status),
	})
}

func CanvasImageTaskFetch(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}

	c.JSON(http.StatusOK, buildCanvasImageTaskResponse(task))
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
	task, exists, err := model.GetByTaskId(canvasImageTaskUserID(relayReq.Keys), relayReq.TaskID)
	if err != nil || !exists {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			common.SysError(fmt.Sprintf("canvas image task panic: %v", recovered))
			failCanvasImageTask(task, fmt.Sprintf("image generation failed: %v", recovered), nil)
		}
	}()

	now := time.Now().Unix()
	task.Status = model.TaskStatusInProgress
	task.StartTime = now
	task.UpdatedAt = now
	task.Progress = "10%"
	_ = task.Update()

	recorder, channelID := executeCanvasImageGenerationRelay(relayReq)
	finishCanvasImageTask(task, channelID, recorder)
}

func executeCanvasImageGenerationRelay(relayReq canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int) {
	recorder := httptest.NewRecorder()
	engine := gin.New()
	channelID := 0

	engine.Use(func(c *gin.Context) {
		for key, value := range relayReq.Keys {
			c.Set(key, value)
		}
		c.Next()
		channelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	})
	engine.Use(middleware.BodyStorageCleanup())
	engine.POST("/canvas/v1/images/generations",
		middleware.Distribute(),
		middleware.ModelRequestRateLimit(),
		func(c *gin.Context) {
			Relay(c, types.RelayFormatOpenAIImage)
		},
	)

	targetURL := "/canvas/v1/images/generations"
	if relayReq.RawQuery != "" {
		targetURL += "?" + relayReq.RawQuery
	}
	request := httptest.NewRequest(http.MethodPost, targetURL, bytes.NewReader(relayReq.Body))
	request.Header = relayReq.Header.Clone()
	request.ContentLength = int64(len(relayReq.Body))
	engine.ServeHTTP(recorder, request)

	return recorder, channelID
}

func finishCanvasImageTask(task *model.Task, channelID int, recorder *httptest.ResponseRecorder) {
	now := time.Now().Unix()
	task.FinishTime = now
	task.UpdatedAt = now
	task.Progress = "100%"
	if channelID > 0 {
		task.ChannelId = channelID
	}

	body := bytes.TrimSpace(recorder.Body.Bytes())
	if recorder.Code >= http.StatusOK && recorder.Code < http.StatusMultipleChoices && len(body) > 0 {
		task.Status = model.TaskStatusSuccess
		task.Data = json.RawMessage(append([]byte(nil), body...))
		task.FailReason = ""
		_ = task.Update()
		return
	}

	failCanvasImageTask(task, extractCanvasImageRelayError(body), body)
}

func failCanvasImageTask(task *model.Task, reason string, body []byte) {
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FinishTime = time.Now().Unix()
	task.UpdatedAt = task.FinishTime
	task.FailReason = reason
	if len(body) > 0 {
		task.Data = json.RawMessage(append([]byte(nil), body...))
	}
	_ = task.Update()
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
	response := gin.H{
		"task_id":  task.TaskID,
		"status":   mapCanvasImageTaskStatus(task.Status),
		"progress": task.Progress,
	}
	if task.Status == model.TaskStatusSuccess && len(bytes.TrimSpace(task.Data)) > 0 {
		response["result"] = task.Data
	}
	if task.Status == model.TaskStatusFailure {
		response["error"] = task.FailReason
	}
	return response
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
