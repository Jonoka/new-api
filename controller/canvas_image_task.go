package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	canvasImageTaskActionGenerations = "images/generations"
	canvasImageTaskActionEdits       = "images/edits"
)

type canvasImageTaskRelayRequest struct {
	TaskID   string
	Action   string
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
	action := normalizeCanvasImageTaskAction(c.Query("action"))

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     model.GenerateTaskID(),
		Platform:   constant.TaskPlatformCanvasImage,
		UserId:     c.GetInt("id"),
		Group:      strings.TrimSpace(c.Query("group")),
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
		TaskID:   task.TaskID,
		Action:   action,
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

func CanvasImageTaskContent(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	index, err := strconv.Atoi(strings.TrimSpace(c.Param("index")))
	if taskID == "" || err != nil || index < 0 {
		abortCanvasRequest(c, http.StatusBadRequest, "invalid image content request")
		return
	}

	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}
	if task.Status != model.TaskStatusSuccess {
		abortCanvasRequest(c, http.StatusBadRequest, "image task is not completed")
		return
	}

	image, mimeType, err := readCanvasImageTaskContent(task, index)
	if err != nil {
		abortCanvasRequest(c, http.StatusNotFound, "image content not found")
		return
	}

	c.Header("Cache-Control", "private, max-age=86400")
	c.Data(http.StatusOK, mimeType, image)
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

	recorder, channelID := executeCanvasImageRelay(relayReq)
	finishCanvasImageTask(task, channelID, recorder)
}

func executeCanvasImageRelay(relayReq canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int) {
	return executeCanvasImageRelayWithHandler(relayReq, nil)
}

func executeCanvasImageRelayWithHandler(relayReq canvasImageTaskRelayRequest, handler gin.HandlerFunc) (*httptest.ResponseRecorder, int) {
	recorder := httptest.NewRecorder()
	engine := gin.New()
	channelID := 0
	action := normalizeCanvasImageTaskAction(relayReq.Action)
	targetPath := "/canvas/v1/" + action

	engine.Use(func(c *gin.Context) {
		for key, value := range relayReq.Keys {
			c.Set(key, value)
		}
		c.Next()
		channelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
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
	request := httptest.NewRequest(http.MethodPost, targetURL, bytes.NewReader(relayReq.Body))
	request.Header = relayReq.Header.Clone()
	request.ContentLength = int64(len(relayReq.Body))
	engine.ServeHTTP(recorder, request)

	return recorder, channelID
}

func normalizeCanvasImageTaskAction(action string) string {
	switch strings.Trim(strings.TrimSpace(action), "/") {
	case "edits", canvasImageTaskActionEdits:
		return canvasImageTaskActionEdits
	default:
		return canvasImageTaskActionGenerations
	}
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
		response["result"] = buildCanvasImageTaskResult(task)
	}
	if task.Status == model.TaskStatusFailure {
		response["error"] = task.FailReason
	}
	return response
}

func buildCanvasImageTaskResult(task *model.Task) gin.H {
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
		case strings.TrimSpace(item.URL) != "":
			next["url"] = item.URL
		case strings.TrimSpace(item.B64JSON) != "":
			next["url"] = canvasImageTaskContentPath(task.TaskID, index)
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
	return fmt.Sprintf("/canvas/v1/images/tasks/%s/content/%d", url.PathEscape(taskID), index)
}

func readCanvasImageTaskContent(task *model.Task, index int) ([]byte, string, error) {
	var payload struct {
		Data []struct {
			B64JSON string `json:"b64_json,omitempty"`
		} `json:"data"`
	}
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return nil, "", err
	}
	if index < 0 || index >= len(payload.Data) {
		return nil, "", fmt.Errorf("image index out of range")
	}
	return decodeCanvasImageData(payload.Data[index].B64JSON)
}

func decodeCanvasImageData(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	mimeType := "image/png"
	if strings.HasPrefix(value, "data:") {
		parts := strings.SplitN(value, ",", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid image data url")
		}
		header := strings.TrimPrefix(parts[0], "data:")
		if !strings.Contains(header, ";base64") {
			return nil, "", fmt.Errorf("unsupported image data url")
		}
		mimeType = strings.TrimSuffix(header, ";base64")
		if mimeType == "" {
			mimeType = "image/png"
		}
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
	return image, mimeType, nil
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
