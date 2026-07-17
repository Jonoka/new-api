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
	canvasImageTaskRelayPrefix       = "/canvas/v1"
	apiImageTaskRelayPrefix          = "/v1"
)

type canvasImageTaskRelayRequest struct {
	TaskID      string
	Action      string
	RelayPrefix string
	Body        []byte
	Header      http.Header
	RawQuery    string
	Keys        map[string]any
}

func CanvasImageTaskSubmit(c *gin.Context) {
	submitImageTask(
		c,
		normalizeCanvasImageTaskAction(c.Query("action")),
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
	go runCanvasImageTaskRelay(relayReq)

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

	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
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
	relayPrefix := strings.TrimRight(strings.TrimSpace(relayReq.RelayPrefix), "/")
	if relayPrefix == "" {
		relayPrefix = canvasImageTaskRelayPrefix
	}
	targetPath := relayPrefix + "/" + action

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
		response["error"] = task.FailReason
	}
	return response
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
			next["url"] = imageTaskContentPath(responsePrefix, task.TaskID, index)
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
