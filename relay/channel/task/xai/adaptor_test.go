package xai

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func TestBuildRequestBodySupportsNativeVideoParameters(t *testing.T) {
	c, _ := newJSONContext(t, `{
		"model":"grok-imagine-video-1.5",
		"prompt":"火箭升空",
		"duration":"10",
		"aspect_ratio":"16:9",
		"resolution":"720p"
	}`)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video-1.5"}}
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	if info.Action != constant.TaskActionTextGenerate {
		t.Fatalf("action = %q, want %q", info.Action, constant.TaskActionTextGenerate)
	}
	if info.OriginModelName != baseVideoModel {
		t.Fatalf("billing model = %q, want %q", info.OriginModelName, baseVideoModel)
	}
	if ratio := adaptor.EstimateBilling(c, info)["resolution"]; ratio != 1.4 {
		t.Fatalf("720p fallback resolution ratio = %v, want 1.4", ratio)
	}

	body := mustBuildRequestBody(t, adaptor, c, info)
	if body.Model != "grok-imagine-video" {
		t.Fatalf("model = %q, want 1.5 text-to-video fallback", body.Model)
	}
	if body.Duration == nil || *body.Duration != 10 {
		t.Fatalf("duration = %v, want 10", body.Duration)
	}
	if body.AspectRatio != "16:9" || body.Resolution != "720p" {
		t.Fatalf("format = %q/%q, want 16:9/720p", body.AspectRatio, body.Resolution)
	}
}

func TestBuildRequestBodyNormalizesImageInputAndSize(t *testing.T) {
	c, _ := newJSONContext(t, `{
		"model":"grok-imagine-video",
		"image":{"image_url":"https://example.com/start.png"},
		"duration":8,
		"size":"1280x720"
	}`)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video"}}
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	if info.Action != constant.TaskActionGenerate {
		t.Fatalf("action = %q, want %q", info.Action, constant.TaskActionGenerate)
	}

	body := mustBuildRequestBody(t, adaptor, c, info)
	if body.Image == nil || body.Image.URL != "https://example.com/start.png" {
		t.Fatalf("image = %#v", body.Image)
	}
	if body.AspectRatio != "16:9" || body.Resolution != "720p" {
		t.Fatalf("format = %q/%q, want 16:9/720p", body.AspectRatio, body.Resolution)
	}
}

func TestBuildRequestBodyMapsWidthAndHeight(t *testing.T) {
	c, _ := newJSONContext(t, `{
		"model":"grok-imagine-video",
		"prompt":"竖屏视频",
		"width":720,
		"height":1280
	}`)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video"}}
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	body := mustBuildRequestBody(t, adaptor, c, info)
	if body.AspectRatio != "9:16" || body.Resolution != "720p" {
		t.Fatalf("format = %q/%q, want 9:16/720p", body.AspectRatio, body.Resolution)
	}
}

func TestExplicitResolutionWinsOverConflictingSizeForBilling(t *testing.T) {
	c, _ := newJSONContext(t, `{
		"model":"grok-imagine-video",
		"prompt":"竖屏视频",
		"resolution":"480p",
		"size":"720x1280",
		"duration":10
	}`)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video"}}
	adaptor := &TaskAdaptor{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}

	dimensions := adaptor.EstimateTaskBillingSpec(c, info).Dimensions
	if got := dimensions["resolution"]; got != "480p" {
		t.Fatalf("billing resolution = %q, want 480p", got)
	}
	if got := adaptor.EstimateBilling(c, info)["resolution"]; got != 1 {
		t.Fatalf("legacy resolution ratio = %v, want 1", got)
	}
	body := mustBuildRequestBody(t, adaptor, c, info)
	if body.Resolution != "480p" || body.AspectRatio != "9:16" {
		t.Fatalf("upstream format = %q/%q, want 9:16/480p", body.AspectRatio, body.Resolution)
	}
}

func TestBuildRequestBodyMapsMultipleImagesToReferences(t *testing.T) {
	c, _ := newJSONContext(t, `{
		"model":"grok-imagine-video",
		"prompt":"保持人物一致",
		"images":["https://example.com/a.png","https://example.com/b.png"]
	}`)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video"}}
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	if info.Action != constant.TaskActionReferenceGenerate {
		t.Fatalf("action = %q, want %q", info.Action, constant.TaskActionReferenceGenerate)
	}

	body := mustBuildRequestBody(t, adaptor, c, info)
	if len(body.ReferenceImages) != 2 {
		t.Fatalf("reference image count = %d, want 2", len(body.ReferenceImages))
	}
}

func TestBuildRequestBodyConvertsMultipartImageToDataURL(t *testing.T) {
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	if err := writer.WriteField("model", "grok-imagine-video"); err != nil {
		t.Fatalf("write model: %v", err)
	}
	if err := writer.WriteField("duration", "5"); err != nil {
		t.Fatalf("write duration: %v", err)
	}
	part, err := writer.CreateFormFile("input_reference", "frame.png")
	if err != nil {
		t.Fatalf("create image part: %v", err)
	}
	if _, err := part.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(requestBody.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video"}}
	adaptor := &TaskAdaptor{}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	body := mustBuildRequestBody(t, adaptor, c, info)
	if body.Image == nil || !strings.HasPrefix(body.Image.URL, "data:image/png;base64,") {
		t.Fatalf("multipart image = %#v", body.Image)
	}
}

func TestValidateRequestRejectsExplicitZeroDuration(t *testing.T) {
	c, _ := newJSONContext(t, `{"model":"grok-imagine-video","prompt":"测试","duration":0}`)
	info := &relaycommon.RelayInfo{}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	if taskErr == nil {
		t.Fatal("ValidateRequestAndSetAction() error = nil, want invalid duration")
	}
	if taskErr.Code != "invalid_duration" {
		t.Fatalf("error code = %q, want invalid_duration", taskErr.Code)
	}
}

func TestEstimateBillingUsesOfficialDefaultDuration(t *testing.T) {
	c, _ := newJSONContext(t, `{"model":"grok-imagine-video","prompt":"测试"}`)
	info := &relaycommon.RelayInfo{}
	adaptor := &TaskAdaptor{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}

	ratios := adaptor.EstimateBilling(c, info)
	if ratios["seconds"] != defaultVideoDuration {
		t.Fatalf("seconds ratio = %v, want %d", ratios["seconds"], defaultVideoDuration)
	}
	if ratios["resolution"] != 1 {
		t.Fatalf("default resolution ratio = %v, want 1", ratios["resolution"])
	}
}

func TestEstimateBillingUsesOfficialResolutionPrices(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantRatio  float64
		wantAction string
	}{
		{
			name:       "base 720p",
			body:       `{"model":"grok-imagine-video","prompt":"测试","resolution":"720p"}`,
			wantRatio:  1.4,
			wantAction: constant.TaskActionTextGenerate,
		},
		{
			name:       "1.5 720p",
			body:       `{"model":"grok-imagine-video-1.5","image":"https://example.com/a.png","resolution":"720p"}`,
			wantRatio:  1.75,
			wantAction: constant.TaskActionGenerate,
		},
		{
			name:       "1.5 1080p",
			body:       `{"model":"grok-imagine-video-1.5","image":"https://example.com/a.png","resolution":"1080p"}`,
			wantRatio:  3.125,
			wantAction: constant.TaskActionGenerate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newJSONContext(t, tt.body)
			info := &relaycommon.RelayInfo{}
			adaptor := &TaskAdaptor{}
			if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
				t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
			}
			if info.Action != tt.wantAction {
				t.Fatalf("action = %q, want %q", info.Action, tt.wantAction)
			}
			if got := adaptor.EstimateBilling(c, info)["resolution"]; got != tt.wantRatio {
				t.Fatalf("resolution ratio = %v, want %v", got, tt.wantRatio)
			}
		})
	}
}

func TestValidateRequestRejectsUnsupported1080pMode(t *testing.T) {
	c, _ := newJSONContext(t, `{"model":"grok-imagine-video","prompt":"测试","resolution":"1080p"}`)
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})
	if taskErr == nil {
		t.Fatal("ValidateRequestAndSetAction() error = nil, want invalid resolution")
	}
	if taskErr.Code != "invalid_resolution" {
		t.Fatalf("error code = %q, want invalid_resolution", taskErr.Code)
	}
}

func TestDoResponseHidesUpstreamRequestID(t *testing.T) {
	c, recorder := newJSONContext(t, `{"model":"grok-imagine-video","prompt":"测试","duration":6}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	adaptor := &TaskAdaptor{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{"request_id":"upstream-secret"}`))}

	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse() error = %v", taskErr)
	}
	if taskID != "upstream-secret" || !strings.Contains(string(taskData), "upstream-secret") {
		t.Fatalf("upstream result = %q, %s", taskID, taskData)
	}
	var response dto.OpenAIVideo
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal public response: %v", err)
	}
	if response.ID != "task_public" || response.TaskID != "task_public" {
		t.Fatalf("public IDs = %q/%q", response.ID, response.TaskID)
	}
	if strings.Contains(recorder.Body.String(), "upstream-secret") {
		t.Fatalf("public response leaked upstream request id: %s", recorder.Body.String())
	}
}

func TestBuildRequestURLAndHeader(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl: "https://api.x.ai/v1/",
		ApiKey:         "xai-key",
	}})

	requestURL, err := adaptor.BuildRequestURL(nil)
	if err != nil {
		t.Fatalf("BuildRequestURL() error = %v", err)
	}
	if requestURL != "https://api.x.ai/v1/videos/generations" {
		t.Fatalf("request URL = %q", requestURL)
	}
	req := httptest.NewRequest(http.MethodPost, requestURL, nil)
	if err := adaptor.BuildRequestHeader(nil, req, nil); err != nil {
		t.Fatalf("BuildRequestHeader() error = %v", err)
	}
	if req.Header.Get("Authorization") != "Bearer xai-key" {
		t.Fatalf("authorization = %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("Content-Type") != "application/json" || req.Header.Get("Accept") != "application/json" {
		t.Fatalf("content negotiation headers = %v", req.Header)
	}
}

func TestParseTaskResult(t *testing.T) {
	falseValue := false
	tests := []struct {
		name       string
		body       string
		wantStatus model.TaskStatus
		wantURL    string
		wantReason string
	}{
		{
			name:       "pending",
			body:       `{"status":"pending","progress":42}`,
			wantStatus: model.TaskStatusInProgress,
		},
		{
			name:       "done",
			body:       `{"status":"done","progress":100,"video":{"url":"https://example.com/video.mp4","duration":8,"respect_moderation":true}}`,
			wantStatus: model.TaskStatusSuccess,
			wantURL:    "https://example.com/video.mp4",
		},
		{
			name:       "failed",
			body:       `{"status":"failed","error":{"code":"service_unavailable","message":"busy"}}`,
			wantStatus: model.TaskStatusFailure,
			wantReason: "busy",
		},
		{
			name:       "expired",
			body:       `{"status":"expired"}`,
			wantStatus: model.TaskStatusFailure,
			wantReason: "xAI video request expired",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(test.body))
			if err != nil {
				t.Fatalf("ParseTaskResult() error = %v", err)
			}
			if model.TaskStatus(result.Status) != test.wantStatus {
				t.Fatalf("status = %q, want %q", result.Status, test.wantStatus)
			}
			if result.Url != test.wantURL || result.Reason != test.wantReason {
				t.Fatalf("url/reason = %q/%q, want %q/%q", result.Url, result.Reason, test.wantURL, test.wantReason)
			}
			if test.name == "done" && result.DurationSeconds != 8 {
				t.Fatalf("duration seconds = %d, want 8", result.DurationSeconds)
			}
		})
	}

	blockedBody, err := common.Marshal(statusResponse{
		Status: "done",
		Video:  &videoResult{RespectModeration: &falseValue},
	})
	if err != nil {
		t.Fatalf("marshal blocked response: %v", err)
	}
	blocked, err := (&TaskAdaptor{}).ParseTaskResult(blockedBody)
	if err != nil {
		t.Fatalf("ParseTaskResult(blocked) error = %v", err)
	}
	if model.TaskStatus(blocked.Status) != model.TaskStatusFailure {
		t.Fatalf("blocked status = %q, want failure", blocked.Status)
	}
}

func TestAdjustBillingOnCompleteUsesActualVideoDuration(t *testing.T) {
	task := &model.Task{
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice:     0.08,
				ModelPriceUnit: types.ModelPriceUnitSecond,
				GroupRatio:     1.2,
				OtherRatios: map[string]float64{
					"seconds":    8,
					"resolution": 1.75,
				},
			},
		},
	}
	taskResult := &relaycommon.TaskInfo{DurationSeconds: 10}

	got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, taskResult)
	want, _ := common.QuotaFromFloatChecked(0.08 * common.QuotaPerUnit * 1.2 * 10 * 1.75)
	if got != want {
		t.Fatalf("AdjustBillingOnComplete() = %d, want %d", got, want)
	}
	if gotSeconds := task.PrivateData.BillingContext.OtherRatios["seconds"]; gotSeconds != 10 {
		t.Fatalf("settled seconds = %v, want 10", gotSeconds)
	}
}

func TestFetchTaskUsesEscapedIDAndBearerAuth(t *testing.T) {
	service.InitHttpClient()
	var gotPath string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"pending"}`))
	}))
	defer server.Close()

	resp, err := (&TaskAdaptor{}).FetchTask(server.URL+"/v1", "xai-key", map[string]any{
		"task_id": "request/with/slash",
	}, "")
	if err != nil {
		t.Fatalf("FetchTask() error = %v", err)
	}
	_ = resp.Body.Close()
	if gotPath != "/v1/videos/request%2Fwith%2Fslash" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer xai-key" {
		t.Fatalf("authorization = %q", gotAuth)
	}
}

func TestConvertToOpenAIVideoIncludesResultMetadata(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		CreatedAt:  100,
		FinishTime: 200,
		Properties: model.Properties{OriginModelName: "grok-imagine-video"},
		Data:       []byte(`{"status":"done","model":"grok-imagine-video-1.5","video":{"duration":12}}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("ConvertToOpenAIVideo() error = %v", err)
	}
	var video dto.OpenAIVideo
	if err := common.Unmarshal(data, &video); err != nil {
		t.Fatalf("unmarshal video: %v", err)
	}
	if video.ID != task.TaskID || video.Status != dto.VideoStatusCompleted {
		t.Fatalf("id/status = %q/%q", video.ID, video.Status)
	}
	if video.Model != "grok-imagine-video-1.5" || video.Seconds != "12" {
		t.Fatalf("model/seconds = %q/%q", video.Model, video.Seconds)
	}
	if video.CompletedAt != 200 {
		t.Fatalf("completed_at = %d, want 200", video.CompletedAt)
	}
}

func newJSONContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	return c, recorder
}

func mustBuildRequestBody(t *testing.T, adaptor *TaskAdaptor, c *gin.Context, info *relaycommon.RelayInfo) videoGenerationRequest {
	t.Helper()
	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var request videoGenerationRequest
	if err := common.Unmarshal(data, &request); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	return request
}
