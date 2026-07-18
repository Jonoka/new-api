package xai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const (
	xaiVideoRequestContextKey = "xai_video_request"
	baseVideoModel            = "grok-imagine-video"
	imageVideoModel           = "grok-imagine-video-1.5"
	defaultVideoDuration      = 8
	defaultVideoResolution    = "480p"
	maxVideoDuration          = 15
	maxMultipartImageBytes    = 20 * 1024 * 1024
)

var (
	allowedAspectRatios = map[string]struct{}{
		"1:1": {}, "16:9": {}, "9:16": {}, "4:3": {}, "3:4": {}, "3:2": {}, "2:3": {},
	}
	allowedResolutions = map[string]struct{}{
		"480p": {}, "720p": {}, "1080p": {},
	}
)

// mediaInput 是 xAI Imagine 接口统一使用的图片输入格式。
type mediaInput struct {
	URL    string `json:"url,omitempty"`
	FileID string `json:"file_id,omitempty"`
}

// videoGenerationRequest 是发送给 xAI 的视频生成请求。
// Duration 使用指针，确保“未提供”和显式零值具有不同语义。
type videoGenerationRequest struct {
	Model           string         `json:"model"`
	Prompt          string         `json:"prompt,omitempty"`
	Image           *mediaInput    `json:"image,omitempty"`
	ReferenceImages []mediaInput   `json:"reference_images,omitempty"`
	Duration        *int           `json:"duration,omitempty"`
	AspectRatio     string         `json:"aspect_ratio,omitempty"`
	Resolution      string         `json:"resolution,omitempty"`
	Output          map[string]any `json:"output,omitempty"`
	StorageOptions  map[string]any `json:"storage_options,omitempty"`
	User            string         `json:"user,omitempty"`
}

// rawVideoGenerationRequest 同时接收 New API 通用字段和 xAI 原生字段。
// any 字段用于兼容数字/字符串时长、URL 字符串和对象式媒体输入。
type rawVideoGenerationRequest struct {
	Model           string `json:"model"`
	Prompt          string `json:"prompt"`
	Image           any    `json:"image,omitempty"`
	Images          any    `json:"images,omitempty"`
	ReferenceImages any    `json:"reference_images,omitempty"`
	InputReference  any    `json:"input_reference,omitempty"`
	Duration        any    `json:"duration,omitempty"`
	Seconds         any    `json:"seconds,omitempty"`
	AspectRatio     string `json:"aspect_ratio,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
	Size            string `json:"size,omitempty"`
	Width           any    `json:"width,omitempty"`
	Height          any    `json:"height,omitempty"`
	Metadata        any    `json:"metadata,omitempty"`
	Output          any    `json:"output,omitempty"`
	StorageOptions  any    `json:"storage_options,omitempty"`
	User            string `json:"user,omitempty"`
}

type submitResponse struct {
	RequestID string `json:"request_id"`
}

type videoError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type videoFileOutput struct {
	PublicURL string `json:"public_url,omitempty"`
}

type videoResult struct {
	URL               string           `json:"url,omitempty"`
	Duration          *int             `json:"duration,omitempty"`
	RespectModeration *bool            `json:"respect_moderation,omitempty"`
	FileOutput        *videoFileOutput `json:"file_output,omitempty"`
}

type statusResponse struct {
	Status   string       `json:"status"`
	Progress *int         `json:"progress,omitempty"`
	Model    string       `json:"model,omitempty"`
	Video    *videoResult `json:"video,omitempty"`
	Error    *videoError  `json:"error,omitempty"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

var (
	_ channel.TaskAdaptor          = (*TaskAdaptor)(nil)
	_ channel.OpenAIVideoConverter = (*TaskAdaptor)(nil)
)

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	if info == nil {
		return
	}
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("relay info is required"), "invalid_request", http.StatusBadRequest)
	}
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	if info.Action == constant.TaskActionRemix {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("xAI video remix is not supported"),
			"not_implemented",
			http.StatusNotImplemented,
		)
	}

	request, err := parseVideoGenerationRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model is required"), "missing_model", http.StatusBadRequest)
	}
	if request.Image != nil && len(request.ReferenceImages) > 0 {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("image and reference_images cannot be used together"),
			"invalid_request",
			http.StatusBadRequest,
		)
	}
	if strings.TrimSpace(request.Prompt) == "" && request.Image == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if request.Duration != nil && (*request.Duration < 1 || *request.Duration > maxVideoDuration) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("duration must be between 1 and %d", maxVideoDuration),
			"invalid_duration",
			http.StatusBadRequest,
		)
	}
	if request.AspectRatio != "" {
		if _, ok := allowedAspectRatios[request.AspectRatio]; !ok {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("unsupported aspect_ratio: %s", request.AspectRatio),
				"invalid_aspect_ratio",
				http.StatusBadRequest,
			)
		}
	}
	if request.Resolution != "" {
		if _, ok := allowedResolutions[request.Resolution]; !ok {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("unsupported resolution: %s", request.Resolution),
				"invalid_resolution",
				http.StatusBadRequest,
			)
		}
	}
	hasImageInput := request.Image != nil || len(request.ReferenceImages) > 0
	if request.Model == imageVideoModel && !hasImageInput {
		// xAI 1.5 仅支持图生视频；与 Sub2API 保持一致，纯文本请求回落到基础模型并按基础模型计费。
		request.Model = baseVideoModel
		info.OriginModelName = baseVideoModel
	}
	if request.Resolution == "1080p" && (request.Model != imageVideoModel || !hasImageInput) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("1080p is only supported by %s for image-to-video generation", imageVideoModel),
			"invalid_resolution",
			http.StatusBadRequest,
		)
	}

	switch {
	case len(request.ReferenceImages) > 0:
		info.Action = constant.TaskActionReferenceGenerate
	case request.Image != nil:
		info.Action = constant.TaskActionGenerate
	default:
		info.Action = constant.TaskActionTextGenerate
	}
	c.Set(xaiVideoRequestContextKey, request)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	request, err := getVideoGenerationRequest(c)
	if err != nil {
		return nil
	}
	duration := defaultVideoDuration
	if request.Duration != nil {
		duration = *request.Duration
	}
	modelName := request.Model
	if info != nil && info.OriginModelName != "" {
		modelName = info.OriginModelName
	}
	resolution := request.Resolution
	if resolution == "" {
		resolution = defaultVideoResolution
	}
	return map[string]float64{
		"seconds":    float64(duration),
		"resolution": xaiResolutionPriceRatio(modelName, resolution),
	}
}

// AdjustBillingOnComplete 在 xAI 返回实际视频时长时进行差额结算。
// 上游未返回时长时返回 0，继续保留按请求时长计算的预扣额度。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	return taskcommon.AdjustPerSecondBillingOnComplete(task, taskResult)
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return buildXAIAPIURL(a.baseURL, "/v1/videos/generations")
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	request, err := getVideoGenerationRequest(c)
	if err != nil {
		return nil, err
	}
	request.Model = info.UpstreamModelName
	if request.Model == imageVideoModel && request.Image == nil && len(request.ReferenceImages) == 0 {
		// xAI 1.5 当前用于图生视频；纯文本请求按官方兼容行为回落到基础视频模型。
		request.Model = baseVideoModel
	}
	data, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func xaiResolutionPriceRatio(modelName, resolution string) float64 {
	switch modelName {
	case imageVideoModel:
		switch resolution {
		case "720p":
			return 0.14 / 0.08
		case "1080p":
			return 0.25 / 0.08
		}
	case baseVideoModel:
		if resolution == "720p" {
			return 0.07 / 0.05
		}
	}
	return 1
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var upstream submitResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(
			errors.Wrapf(err, "body: %s", responseBody),
			"unmarshal_response_body_failed",
			http.StatusInternalServerError,
		)
	}
	if strings.TrimSpace(upstream.RequestID) == "" {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("request_id is empty"),
			"invalid_response",
			http.StatusInternalServerError,
		)
	}

	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.Model = info.OriginModelName
	video.CreatedAt = time.Now().Unix()
	if request, requestErr := getVideoGenerationRequest(c); requestErr == nil {
		duration := defaultVideoDuration
		if request.Duration != nil {
			duration = *request.Duration
		}
		video.Seconds = strconv.Itoa(duration)
	}
	c.JSON(http.StatusOK, video)
	return upstream.RequestID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	requestURL, err := buildXAIAPIURL(baseURL, "/v1/videos/"+url.PathEscape(taskID))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var result statusResponse
	if err := common.Unmarshal(respBody, &result); err != nil {
		return nil, errors.Wrap(err, "unmarshal xAI video task result failed")
	}

	taskResult := &relaycommon.TaskInfo{}
	switch strings.ToLower(strings.TrimSpace(result.Status)) {
	case "":
		// 交给统一轮询逻辑识别标准 API 错误（尤其是可重试的 429）。
		return taskResult, nil
	case "pending":
		taskResult.Status = model.TaskStatusInProgress
		if result.Progress != nil {
			taskResult.Progress = fmt.Sprintf("%d%%", clampProgress(*result.Progress))
		}
	case "done":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = taskcommon.ProgressComplete
		if result.Video != nil {
			taskResult.Url = strings.TrimSpace(result.Video.URL)
			if result.Video.Duration != nil && *result.Video.Duration > 0 {
				taskResult.DurationSeconds = *result.Video.Duration
			}
			if taskResult.Url == "" && result.Video.FileOutput != nil {
				taskResult.Url = strings.TrimSpace(result.Video.FileOutput.PublicURL)
			}
		}
		if taskResult.Url == "" {
			taskResult.Status = model.TaskStatusFailure
			if result.Video != nil && result.Video.RespectModeration != nil && !*result.Video.RespectModeration {
				taskResult.Reason = "video was blocked by moderation"
			} else {
				taskResult.Reason = "xAI video completed without an accessible URL"
			}
		}
	case "failed", "expired":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = taskcommon.ProgressComplete
		if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
			taskResult.Reason = result.Error.Message
		} else {
			taskResult.Reason = "xAI video request " + strings.ToLower(strings.TrimSpace(result.Status))
		}
	default:
		return nil, fmt.Errorf("unknown xAI video status: %s", result.Status)
	}
	return taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.TaskID = task.TaskID
	video.Model = firstNonEmpty(task.Properties.OriginModelName, task.Properties.UpstreamModelName, "grok-imagine-video")
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		video.CompletedAt = firstPositive(task.FinishTime, task.UpdatedAt)
	}

	var upstream statusResponse
	if len(task.Data) > 0 {
		_ = common.Unmarshal(task.Data, &upstream)
	}
	if strings.TrimSpace(upstream.Model) != "" {
		video.Model = upstream.Model
	}
	if upstream.Video != nil && upstream.Video.Duration != nil {
		video.Seconds = strconv.Itoa(*upstream.Video.Duration)
	} else if task.PrivateData.BillingContext != nil {
		if seconds, ok := task.PrivateData.BillingContext.OtherRatios["seconds"]; ok && seconds > 0 {
			video.Seconds = strconv.FormatFloat(seconds, 'f', -1, 64)
		}
	}
	if task.Status == model.TaskStatusFailure {
		code := "video_generation_failed"
		message := strings.TrimSpace(task.FailReason)
		if upstream.Error != nil {
			if strings.TrimSpace(upstream.Error.Code) != "" {
				code = upstream.Error.Code
			}
			if message == "" {
				message = upstream.Error.Message
			}
		}
		if message == "" {
			message = "xAI video generation failed"
		}
		video.Error = &dto.OpenAIVideoError{Code: code, Message: message}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func parseVideoGenerationRequest(c *gin.Context) (videoGenerationRequest, error) {
	var raw rawVideoGenerationRequest
	if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
		return videoGenerationRequest{}, err
	}

	metadata, err := mapFromAny(raw.Metadata)
	if err != nil {
		return videoGenerationRequest{}, fmt.Errorf("invalid metadata: %w", err)
	}

	request := videoGenerationRequest{
		Model:          strings.TrimSpace(raw.Model),
		Prompt:         strings.TrimSpace(raw.Prompt),
		AspectRatio:    strings.TrimSpace(raw.AspectRatio),
		Resolution:     strings.ToLower(strings.TrimSpace(raw.Resolution)),
		User:           strings.TrimSpace(raw.User),
		Output:         nil,
		StorageOptions: nil,
	}
	if request.AspectRatio == "" {
		request.AspectRatio = stringFromMap(metadata, "aspect_ratio")
	}
	if request.Resolution == "" {
		request.Resolution = strings.ToLower(stringFromMap(metadata, "resolution"))
	}
	if request.User == "" {
		request.User = stringFromMap(metadata, "user")
	}

	durationValue := firstPresent(raw.Duration, raw.Seconds, metadata["duration"], metadata["seconds"])
	request.Duration, err = optionalInt(durationValue)
	if err != nil {
		return videoGenerationRequest{}, fmt.Errorf("invalid duration: %w", err)
	}

	request.Output, err = mapFromFirstPresent(raw.Output, metadata["output"])
	if err != nil {
		return videoGenerationRequest{}, fmt.Errorf("invalid output: %w", err)
	}
	request.StorageOptions, err = mapFromFirstPresent(raw.StorageOptions, metadata["storage_options"])
	if err != nil {
		return videoGenerationRequest{}, fmt.Errorf("invalid storage_options: %w", err)
	}

	imageValue := firstPresent(raw.Image, metadata["image"])
	imagesValue := firstPresent(raw.Images, metadata["images"])
	referenceValue := firstPresent(raw.ReferenceImages, metadata["reference_images"])
	inputReferenceValue := firstPresent(raw.InputReference, metadata["input_reference"])

	images, err := mediaInputsFromAny(imageValue)
	if err != nil {
		return videoGenerationRequest{}, fmt.Errorf("invalid image: %w", err)
	}
	if len(images) > 1 {
		return videoGenerationRequest{}, fmt.Errorf("image accepts only one item")
	}
	if len(images) == 1 {
		request.Image = &images[0]
	}

	references, err := mediaInputsFromAny(referenceValue)
	if err != nil {
		return videoGenerationRequest{}, fmt.Errorf("invalid reference_images: %w", err)
	}
	request.ReferenceImages = references

	genericImages, err := mediaInputsFromAny(imagesValue)
	if err != nil {
		return videoGenerationRequest{}, fmt.Errorf("invalid images: %w", err)
	}
	if len(genericImages) > 0 {
		if request.Image != nil || len(request.ReferenceImages) > 0 {
			return videoGenerationRequest{}, fmt.Errorf("image, images and reference_images cannot be combined")
		}
		if len(genericImages) == 1 {
			request.Image = &genericImages[0]
		} else {
			request.ReferenceImages = genericImages
		}
	}

	inputReferences, err := mediaInputsFromAny(inputReferenceValue)
	if err != nil {
		return videoGenerationRequest{}, fmt.Errorf("invalid input_reference: %w", err)
	}
	if len(inputReferences) > 0 {
		if request.Image != nil || len(request.ReferenceImages) > 0 || len(inputReferences) != 1 {
			return videoGenerationRequest{}, fmt.Errorf("input_reference cannot be combined with other image inputs")
		}
		request.Image = &inputReferences[0]
	}

	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		if err := addMultipartImages(c, &request); err != nil {
			return videoGenerationRequest{}, err
		}
	}

	size := strings.TrimSpace(raw.Size)
	if size == "" {
		size = stringFromMap(metadata, "size")
	}
	if size == "" {
		widthValue := firstPresent(raw.Width, metadata["width"])
		heightValue := firstPresent(raw.Height, metadata["height"])
		if !isAbsent(widthValue) || !isAbsent(heightValue) {
			width, widthErr := optionalInt(widthValue)
			height, heightErr := optionalInt(heightValue)
			if widthErr != nil || heightErr != nil || width == nil || height == nil || *width <= 0 || *height <= 0 {
				return videoGenerationRequest{}, fmt.Errorf("width and height must be positive integers")
			}
			size = fmt.Sprintf("%dx%d", *width, *height)
		}
	}
	if request.AspectRatio == "" || request.Resolution == "" {
		ratio, resolution := inferVideoFormatFromSize(size)
		if request.AspectRatio == "" {
			request.AspectRatio = ratio
		}
		if request.Resolution == "" {
			request.Resolution = resolution
		}
	}
	return request, nil
}

func getVideoGenerationRequest(c *gin.Context) (videoGenerationRequest, error) {
	value, ok := c.Get(xaiVideoRequestContextKey)
	if !ok {
		return videoGenerationRequest{}, fmt.Errorf("xAI video request not found in context")
	}
	request, ok := value.(videoGenerationRequest)
	if !ok {
		return videoGenerationRequest{}, fmt.Errorf("invalid xAI video request type")
	}
	return request, nil
}

func addMultipartImages(c *gin.Context, request *videoGenerationRequest) error {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return fmt.Errorf("parse multipart form failed: %w", err)
	}
	defer form.RemoveAll()

	var singleFiles []mediaInput
	for _, field := range []string{"input_reference", "image"} {
		for _, fileHeader := range form.File[field] {
			input, readErr := mediaInputFromUpload(fileHeader)
			if readErr != nil {
				return readErr
			}
			singleFiles = append(singleFiles, input)
		}
	}
	if len(singleFiles) > 1 {
		return fmt.Errorf("only one input image is allowed")
	}
	if len(singleFiles) == 1 {
		if request.Image != nil || len(request.ReferenceImages) > 0 {
			return fmt.Errorf("uploaded image cannot be combined with other image inputs")
		}
		request.Image = &singleFiles[0]
	}

	var genericFiles []mediaInput
	for _, fileHeader := range form.File["images"] {
		input, readErr := mediaInputFromUpload(fileHeader)
		if readErr != nil {
			return readErr
		}
		genericFiles = append(genericFiles, input)
	}
	var referenceFiles []mediaInput
	for _, fileHeader := range form.File["reference_images"] {
		input, readErr := mediaInputFromUpload(fileHeader)
		if readErr != nil {
			return readErr
		}
		referenceFiles = append(referenceFiles, input)
	}
	if len(genericFiles)+len(referenceFiles) > 0 {
		if request.Image != nil || len(request.ReferenceImages) > 0 {
			return fmt.Errorf("uploaded images cannot be combined with other image inputs")
		}
		if len(referenceFiles) > 0 {
			request.ReferenceImages = append(genericFiles, referenceFiles...)
		} else if len(genericFiles) == 1 {
			request.Image = &genericFiles[0]
		} else {
			request.ReferenceImages = genericFiles
		}
	}
	return nil
}

func mediaInputFromUpload(fileHeader *multipart.FileHeader) (mediaInput, error) {
	if fileHeader == nil {
		return mediaInput{}, fmt.Errorf("image file is missing")
	}
	if fileHeader.Size > maxMultipartImageBytes {
		return mediaInput{}, fmt.Errorf("image file exceeds %d MiB", maxMultipartImageBytes/(1024*1024))
	}
	file, err := fileHeader.Open()
	if err != nil {
		return mediaInput{}, fmt.Errorf("open image file failed: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxMultipartImageBytes+1))
	if err != nil {
		return mediaInput{}, fmt.Errorf("read image file failed: %w", err)
	}
	if len(data) > maxMultipartImageBytes {
		return mediaInput{}, fmt.Errorf("image file exceeds %d MiB", maxMultipartImageBytes/(1024*1024))
	}
	mimeType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return mediaInput{}, fmt.Errorf("uploaded file must be an image")
	}
	return mediaInput{
		URL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data),
	}, nil
}

func mediaInputsFromAny(value any) ([]mediaInput, error) {
	if isAbsent(value) {
		return nil, nil
	}
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, nil
		}
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var decoded any
			if err := common.UnmarshalJsonStr(trimmed, &decoded); err == nil {
				return mediaInputsFromAny(decoded)
			}
		}
		return []mediaInput{{URL: trimmed}}, nil
	case []any:
		result := make([]mediaInput, 0, len(typed))
		for _, item := range typed {
			inputs, err := mediaInputsFromAny(item)
			if err != nil {
				return nil, err
			}
			result = append(result, inputs...)
		}
		return result, nil
	case []string:
		result := make([]mediaInput, 0, len(typed))
		for _, item := range typed {
			inputs, err := mediaInputsFromAny(item)
			if err != nil {
				return nil, err
			}
			result = append(result, inputs...)
		}
		return result, nil
	case map[string]any:
		input := mediaInput{
			URL:    firstNonEmpty(stringFromMap(typed, "url"), stringFromMap(typed, "image_url")),
			FileID: stringFromMap(typed, "file_id"),
		}
		if input.URL == "" && input.FileID == "" {
			return nil, fmt.Errorf("media input requires url or file_id")
		}
		if input.URL != "" && input.FileID != "" {
			return nil, fmt.Errorf("url and file_id are mutually exclusive")
		}
		return []mediaInput{input}, nil
	default:
		return nil, fmt.Errorf("unsupported media input type %T", value)
	}
}

func optionalInt(value any) (*int, error) {
	if isAbsent(value) {
		return nil, nil
	}
	var parsed int64
	switch typed := value.(type) {
	case int:
		parsed = int64(typed)
	case int32:
		parsed = int64(typed)
	case int64:
		parsed = typed
	case uint:
		if uint64(typed) > uint64(math.MaxInt) {
			return nil, fmt.Errorf("value is too large")
		}
		parsed = int64(typed)
	case uint64:
		if typed > uint64(math.MaxInt) {
			return nil, fmt.Errorf("value is too large")
		}
		parsed = int64(typed)
	case float64:
		if math.Trunc(typed) != typed {
			return nil, fmt.Errorf("value must be an integer")
		}
		parsed = int64(typed)
	case json.Number:
		value, err := strconv.ParseInt(string(typed), 10, 64)
		if err != nil {
			return nil, err
		}
		parsed = value
	case string:
		value, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return nil, err
		}
		parsed = value
	default:
		return nil, fmt.Errorf("unsupported numeric type %T", value)
	}
	if parsed > int64(math.MaxInt) || parsed < int64(math.MinInt) {
		return nil, fmt.Errorf("value is out of range")
	}
	result := int(parsed)
	return &result, nil
}

func mapFromFirstPresent(values ...any) (map[string]any, error) {
	return mapFromAny(firstPresent(values...))
}

func mapFromAny(value any) (map[string]any, error) {
	if isAbsent(value) {
		return nil, nil
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = item
		}
		return result, nil
	case string:
		var result map[string]any
		if err := common.UnmarshalJsonStr(strings.TrimSpace(typed), &result); err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected object, got %T", value)
	}
}

func firstPresent(values ...any) any {
	for _, value := range values {
		if !isAbsent(value) {
			return value
		}
	}
	return nil
}

func isAbsent(value any) bool {
	if value == nil {
		return true
	}
	if stringValue, ok := value.(string); ok {
		return strings.TrimSpace(stringValue) == ""
	}
	return false
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func inferVideoFormatFromSize(size string) (string, string) {
	size = strings.ToLower(strings.TrimSpace(size))
	if size == "" {
		return "", ""
	}
	if _, ok := allowedAspectRatios[size]; ok {
		return size, ""
	}
	if _, ok := allowedResolutions[size]; ok {
		return "", size
	}
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return "", ""
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return "", ""
	}
	divisor := greatestCommonDivisor(width, height)
	ratio := fmt.Sprintf("%d:%d", width/divisor, height/divisor)
	if _, ok := allowedAspectRatios[ratio]; !ok {
		ratio = ""
	}
	shortSide := min(width, height)
	resolution := ""
	switch {
	case shortSide >= 1080:
		resolution = "1080p"
	case shortSide >= 720:
		resolution = "720p"
	case shortSide >= 480:
		resolution = "480p"
	}
	return ratio, resolution
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a <= 0 {
		return 1
	}
	return a
}

func buildXAIAPIURL(baseURL, endpoint string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("xAI base URL is empty")
	}
	if strings.HasSuffix(strings.ToLower(baseURL), "/v1") {
		endpoint = strings.TrimPrefix(endpoint, "/v1")
	}
	return baseURL + endpoint, nil
}

func clampProgress(progress int) int {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
