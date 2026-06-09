package sora

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultRunningStatus(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult([]byte(`{"id":"upstream_task","task_id":"upstream_task","status":"running","progress":5}`))

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, model.TaskStatusInProgress, info.Status)
	assert.Equal(t, "5%", info.Progress)
}

func TestParseTaskResultSuccessAliases(t *testing.T) {
	adaptor := &TaskAdaptor{}
	for _, status := range []string{"completed", "succeeded", "success"} {
		t.Run(status, func(t *testing.T) {
			info, err := adaptor.ParseTaskResult([]byte(`{"id":"upstream_task","status":"` + status + `","progress":100}`))

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, model.TaskStatusSuccess, info.Status)
			assert.Equal(t, "100%", info.Progress)
		})
	}
}

func TestFetchTaskUsesImageGenerationsPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"id":"upstream_img_task","status":"succeeded","progress":100}`)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL, "test-key", map[string]any{
		"task_id":      "upstream_img_task",
		"request_path": "/v1/images/generations",
	}, "")

	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	assert.Equal(t, "/v1/images/generations/upstream_img_task", gotPath)
}

func TestFetchTaskPollsImageEditsViaGenerationsPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"id":"upstream_img_edit_task","status":"succeeded","progress":100}`)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL, "test-key", map[string]any{
		"task_id":      "upstream_img_edit_task",
		"request_path": "/v1/images/edits",
	}, "")

	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	assert.Equal(t, "/v1/images/generations/upstream_img_edit_task", gotPath)
}

func TestFetchTaskUsesSingularVideoGenerationsPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"id":"upstream_video_task","status":"succeeded","progress":100}`)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL, "test-key", map[string]any{
		"task_id":      "upstream_video_task",
		"request_path": "/v1/video/generations",
	}, "")

	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	assert.Equal(t, "/v1/video/generations/upstream_video_task", gotPath)
}

func TestFetchTaskUsesGPT2APIVideoGenerationsForCanvasVideosPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"id":"upstream_video_task","status":"succeeded","progress":100}`)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL+"/gpt2api.com", "test-key", map[string]any{
		"task_id":             "upstream_video_task",
		"request_path":        "/v1/videos",
		"upstream_model_name": "grok-imagine-video",
	}, "")

	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	assert.Equal(t, "/gpt2api.com/v1/video/generations/upstream_video_task", gotPath)
}

func TestGPT2APIVideoMappingHelpers(t *testing.T) {
	body := map[string]any{
		"model":           "grok-imagine-video",
		"prompt":          "test prompt",
		"seconds":         "6",
		"size":            "1280x720",
		"resolution_name": "720p",
		"preset":          "normal",
	}
	mapGPT2APIVideoJSONBody(body)

	assert.Equal(t, 6, body["duration"])
	assert.Equal(t, "16:9", body["ratio"])
	assert.Equal(t, "hd", body["quality"])
	assert.Equal(t, true, body["async"])
	_, hasSeconds := body["seconds"]
	assert.False(t, hasSeconds)
	_, hasSize := body["size"]
	assert.False(t, hasSize)
}

func TestGPT2APIVideoFormValuesPreferJSONPayload(t *testing.T) {
	body := formValuesToMap(map[string][]string{
		"model":           {"grok-imagine-video"},
		"prompt":          {"一个韩国女偶像在沙滩"},
		"seconds":         {"6"},
		"size":            {"720x1280"},
		"resolution_name": {"720p"},
		"preset":          {"normal"},
	})
	mapGPT2APIVideoJSONBody(body)

	assert.Equal(t, "grok-imagine-video", body["model"])
	assert.Equal(t, "一个韩国女偶像在沙滩", body["prompt"])
	assert.Equal(t, 6, body["duration"])
	assert.Equal(t, "9:16", body["ratio"])
	assert.Equal(t, "hd", body["quality"])
	assert.Equal(t, true, body["async"])
	_, hasPreset := body["preset"]
	assert.False(t, hasPreset)
}

func TestParseURLEncodedGPT2APIVideoForm(t *testing.T) {
	values, err := parseURLEncodedForm([]byte("model=grok-imagine-video&prompt=test&seconds=6&size=720x1280&resolution_name=720p&preset=normal"))
	require.NoError(t, err)
	body := formValuesToMap(values)
	mapGPT2APIVideoJSONBody(body)

	assert.Equal(t, 6, body["duration"])
	assert.Equal(t, "9:16", body["ratio"])
	assert.Equal(t, "hd", body["quality"])
	_, hasSize := body["size"]
	assert.False(t, hasSize)
}

func TestGPT2APIVideoMultipartInputReferenceBecomesJSONImage(t *testing.T) {
	var formBody bytes.Buffer
	writer := multipart.NewWriter(&formBody)
	require.NoError(t, writer.WriteField("model", "veo3.1"))
	require.NoError(t, writer.WriteField("prompt", "女人看着镜头微笑"))
	require.NoError(t, writer.WriteField("seconds", "6"))
	require.NoError(t, writer.WriteField("size", "720x1280"))
	require.NoError(t, writer.WriteField("resolution_name", "720p"))
	require.NoError(t, writer.WriteField("preset", "normal"))
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="input_reference[]"; filename="reference.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-image-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(formBody.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx, _ := common.NewTestContext(req)
	ctx.Set(common.KeyRequestBody, formBody.Bytes())

	adaptor := &TaskAdaptor{}
	bodyReader, err := adaptor.BuildRequestBody(ctx, &relaycommon.RelayInfo{
		ChannelId:         23,
		ChannelBaseUrl:    "https://gpt2api.com",
		RequestURLPath:    "/v1/videos",
		UpstreamModelName: "veo3.1",
	})
	require.NoError(t, err)
	payload, err := io.ReadAll(bodyReader)
	require.NoError(t, err)

	assert.Equal(t, "application/json", ctx.Request.Header.Get("Content-Type"))
	var got map[string]any
	require.NoError(t, common.Unmarshal(payload, &got))
	assert.Equal(t, "veo3.1", got["model"])
	assert.Equal(t, "女人看着镜头微笑", got["prompt"])
	assert.Equal(t, 6, got["duration"])
	assert.Equal(t, "9:16", got["ratio"])
	assert.Equal(t, "hd", got["quality"])
	assert.Equal(t, true, got["async"])
	image, _ := got["image"].(string)
	assert.True(t, strings.HasPrefix(image, "data:image/png;base64,"))
	_, hasInputReference := got["input_reference"]
	assert.False(t, hasInputReference)
}

func TestChannel25VideoFormMapsCanvasFields(t *testing.T) {
	values, err := parseURLEncodedForm([]byte("model=veo3.1&prompt=test&seconds=6&size=720x1280&resolution_name=720p&preset=normal"))
	require.NoError(t, err)
	body := formValuesToMap(values)
	body["model"] = channel25VideoModelForRequest(&relaycommon.RelayInfo{
		ChannelId:         25,
		ChannelBaseUrl:    "https://img-api.xn--1ys141f4ks.com",
		RequestURLPath:    "/v1/videos",
		UpstreamModelName: "veo3.1",
	}, body)
	mapChannel25VideoJSONBody(body)

	assert.Equal(t, "veo3.1-720p", body["model"])
	assert.Equal(t, "test", body["prompt"])
	assert.Equal(t, "6", body["seconds"])
	assert.Equal(t, "720x1280", body["size"])
	assert.Equal(t, "9:16", body["aspect_ratio"])
	assert.Equal(t, 1, body["type"])
	_, hasPreset := body["preset"]
	assert.False(t, hasPreset)
}

func TestChannel25VideoMultipartReferencesBecomeJSONImages(t *testing.T) {
	var formBody bytes.Buffer
	writer := multipart.NewWriter(&formBody)
	require.NoError(t, writer.WriteField("model", "veo3.1-components"))
	require.NoError(t, writer.WriteField("prompt", "reference test"))
	require.NoError(t, writer.WriteField("seconds", "4"))
	require.NoError(t, writer.WriteField("size", "1280x720"))
	require.NoError(t, writer.WriteField("resolution_name", "720p"))
	require.NoError(t, writer.WriteField("reference_mode", "components"))
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="input_reference[]"; filename="reference.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-image-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(formBody.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx, _ := common.NewTestContext(req)
	ctx.Set(common.KeyRequestBody, formBody.Bytes())

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelId:         25,
		ChannelBaseUrl:    "https://img-api.xn--1ys141f4ks.com",
		RequestURLPath:    "/v1/videos",
		UpstreamModelName: "veo3.1-components",
	}
	bodyReader, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(bodyReader)
	require.NoError(t, err)

	assert.Equal(t, "application/json", ctx.Request.Header.Get("Content-Type"))
	var got map[string]any
	require.NoError(t, common.Unmarshal(payload, &got))
	assert.Equal(t, "veo3.1-components-720p", got["model"])
	assert.Equal(t, "16:9", got["aspect_ratio"])
	assert.Equal(t, 3, got["type"])
	_, hasImage := got["image"]
	assert.False(t, hasImage)
	images := got["images"].([]any)
	assert.Len(t, images, 1)
	_, hasInputReference := got["input_reference"]
	assert.False(t, hasInputReference)
}

func TestChannel25VideoType2WithTwoReferencesUsesImagesOnly(t *testing.T) {
	var formBody bytes.Buffer
	writer := multipart.NewWriter(&formBody)
	require.NoError(t, writer.WriteField("model", "veo3.1"))
	require.NoError(t, writer.WriteField("prompt", "type 2 image test"))
	require.NoError(t, writer.WriteField("seconds", "6"))
	require.NoError(t, writer.WriteField("size", "720x1280"))
	require.NoError(t, writer.WriteField("resolution_name", "720p"))
	require.NoError(t, writer.WriteField("type", "2"))
	require.NoError(t, writer.WriteField("reference_mode", "image"))
	for _, filename := range []string{"a.png", "b.png"} {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="input_reference[]"; filename="%s"`, filename))
		header.Set("Content-Type", "image/png")
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write([]byte("fake-image-bytes"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(formBody.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx, _ := common.NewTestContext(req)
	ctx.Set(common.KeyRequestBody, formBody.Bytes())

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelId:         25,
		ChannelBaseUrl:    "https://img-api.xn--1ys141f4ks.com",
		RequestURLPath:    "/v1/videos",
		UpstreamModelName: "veo3.1",
	}
	bodyReader, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	payload, err := io.ReadAll(bodyReader)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, common.Unmarshal(payload, &got))
	assert.Equal(t, "veo3.1-720p", got["model"])
	assert.Equal(t, "9:16", got["aspect_ratio"])
	assert.Equal(t, "2", got["type"])
	_, hasImage := got["image"]
	assert.False(t, hasImage)
	images := got["images"].([]any)
	assert.Len(t, images, 2)
}

func TestChannel25VideoPrepareBillingRequestInputUsesMappedJSON(t *testing.T) {
	var formBody bytes.Buffer
	writer := multipart.NewWriter(&formBody)
	require.NoError(t, writer.WriteField("model", "veo3.1-components"))
	require.NoError(t, writer.WriteField("prompt", "billing test"))
	require.NoError(t, writer.WriteField("seconds", "6"))
	require.NoError(t, writer.WriteField("size", "1280x720"))
	require.NoError(t, writer.WriteField("resolution_name", "720p"))
	require.NoError(t, writer.WriteField("reference_mode", "components"))
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(formBody.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx, _ := common.NewTestContext(req)
	ctx.Set(common.KeyRequestBody, formBody.Bytes())

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelId:         25,
		ChannelBaseUrl:    "https://img-api.xn--1ys141f4ks.com",
		RequestURLPath:    "/v1/videos",
		UpstreamModelName: "veo3.1-components",
	}
	require.NoError(t, adaptor.PrepareBillingRequestInput(ctx, info))
	require.NotNil(t, info.BillingRequestInput)
	assert.Equal(t, "application/json", info.BillingRequestInput.Headers["Content-Type"])
	var got map[string]any
	require.NoError(t, common.Unmarshal(info.BillingRequestInput.Body, &got))
	assert.Equal(t, "veo3.1-components-720p", got["model"])
	assert.Equal(t, "6", got["seconds"])
	assert.Equal(t, "16:9", got["aspect_ratio"])
	assert.Equal(t, 3, got["type"])
}

func TestConvertToOpenAIVideoNormalizesVideoStatusAndStripsUsage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body, err := adaptor.ConvertToOpenAIVideo(&model.Task{
		TaskID: "public_video_task",
		PrivateData: model.TaskPrivateData{
			RequestPath: "/v1/videos",
		},
		Data: []byte(`{
			"id":"upstream_video_task",
			"status":"succeeded",
			"usage":{"total_cost":50,"total_points":0.5},
			"result":{"data":[{"url":"https://example.com/video.mp4"}],"usage":{"total_cost":50,"total_points":0.5}}
		}`),
	})

	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, common.Unmarshal(body, &got))
	assert.Equal(t, "completed", got["status"])
	assert.Equal(t, "public_video_task", got["id"])
	assert.Equal(t, "public_video_task", got["task_id"])
	_, hasTopUsage := got["usage"]
	assert.False(t, hasTopUsage)
	result := got["result"].(map[string]any)
	_, hasResultUsage := result["usage"]
	assert.False(t, hasResultUsage)
}

func TestParseTaskResultImageResponseWithArtifact(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult([]byte(`{"created":1780226859,"data":[{"url":"https://example.com/generated.png"}],"task_id":"upstream_img_task"}`))

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, model.TaskStatusSuccess, info.Status)
	assert.Equal(t, "100%", info.Progress)
	assert.Equal(t, "https://example.com/generated.png", info.Url)
}

func TestParseTaskResultImageResponseWithNestedResultArtifact(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult([]byte(`{
		"status":"succeeded",
		"data":[{"url":"https://example.com/top-level.png"}],
		"result":{"data":[{"url":"https://example.com/nested.png"}]},
		"task_id":"upstream_img_task"
	}`))

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, model.TaskStatusSuccess, info.Status)
	assert.Equal(t, "100%", info.Progress)
	assert.Equal(t, "https://example.com/top-level.png", info.Url)
}

func TestParseTaskResultImageResponseWithOnlyNestedResultArtifact(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult([]byte(`{
		"status":"succeeded",
		"result":{"data":[{"url":"https://example.com/nested.png"}]},
		"task_id":"upstream_img_task"
	}`))

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, model.TaskStatusSuccess, info.Status)
	assert.Equal(t, "100%", info.Progress)
	assert.Equal(t, "https://example.com/nested.png", info.Url)
}

func TestParseTaskResultImageResponseWithOnlyNestedResultB64Artifact(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult([]byte(`{
		"status":"succeeded",
		"result":{"data":[{"b64_json":"aGVsbG8="}]},
		"task_id":"upstream_img_task"
	}`))

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, model.TaskStatusSuccess, info.Status)
	assert.Equal(t, "100%", info.Progress)
	assert.Equal(t, "data:image/png;base64,aGVsbG8=", info.Url)
}

func TestParseTaskResultImageQueuedTask(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult([]byte(`{"object":"image.generation.task","status":"queued","progress":0,"task_id":"upstream_img_task"}`))

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, model.TaskStatusQueued, info.Status)
	assert.Equal(t, "0%", info.Progress)
}

func TestConvertToOpenAIVideoConvertsURLToB64WhenRequested(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer imageServer.Close()

	adaptor := &TaskAdaptor{}
	body, err := adaptor.ConvertToOpenAIVideo(&model.Task{
		TaskID: "public_task",
		PrivateData: model.TaskPrivateData{
			RequestPath:    "/v1/images/generations",
			ResponseFormat: "b64_json",
		},
		Data: []byte(`{"created":1,"data":[{"url":"` + imageServer.URL + `/img.png"}],"task_id":"upstream_task"}`),
	})

	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, common.Unmarshal(body, &got))
	data := got["data"].([]any)
	item := data[0].(map[string]any)
	assert.Equal(t, "aW1hZ2UtYnl0ZXM=", item["b64_json"])
	_, hasURL := item["url"]
	assert.False(t, hasURL)
	result := got["result"].(map[string]any)
	resultData := result["data"].([]any)
	resultItem := resultData[0].(map[string]any)
	assert.Equal(t, "aW1hZ2UtYnl0ZXM=", resultItem["b64_json"])
	_, resultHasURL := resultItem["url"]
	assert.False(t, resultHasURL)
	assert.Equal(t, "public_task", got["task_id"])
}

func TestConvertToOpenAIVideoAddsTopLevelDataFromNestedImageResult(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body, err := adaptor.ConvertToOpenAIVideo(&model.Task{
		TaskID: "public_task",
		PrivateData: model.TaskPrivateData{
			RequestPath: "/v1/images/generations",
		},
		Data: []byte(`{
			"created":1780590448,
			"error":null,
			"id":"upstream_task",
			"object":"image.generation.task",
			"status":"succeeded",
			"usage":{"total_cost":15,"total_points":0.15},
			"result":{"data":[{"height":2560,"url":"https://example.com/nested.png","width":1440}],"usage":{"total_cost":15,"total_points":0.15}},
			"task_id":"upstream_task"
		}`),
	})

	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, common.Unmarshal(body, &got))
	assert.Equal(t, "succeeded", got["status"])
	data := got["data"].([]any)
	item := data[0].(map[string]any)
	assert.Equal(t, "https://example.com/nested.png", item["url"])
	result := got["result"].(map[string]any)
	resultData := result["data"].([]any)
	resultItem := resultData[0].(map[string]any)
	assert.Equal(t, "https://example.com/nested.png", resultItem["url"])
	_, hasTopUsage := got["usage"]
	assert.False(t, hasTopUsage)
	_, hasResultUsage := result["usage"]
	assert.False(t, hasResultUsage)
	assert.Equal(t, "public_task", got["task_id"])
}

func TestConvertToOpenAIVideoAddsTopLevelDataFromNestedImageEditResult(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body, err := adaptor.ConvertToOpenAIVideo(&model.Task{
		TaskID: "public_edit_task",
		PrivateData: model.TaskPrivateData{
			RequestPath: "/v1/images/edits",
		},
		Data: []byte(`{
			"created":1780590448,
			"error":null,
			"id":"upstream_edit_task",
			"object":"image.generation.task",
			"status":"succeeded",
			"result":{"data":[{"height":1024,"url":"https://example.com/edit.png","width":1024}]},
			"task_id":"upstream_edit_task"
		}`),
	})

	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, common.Unmarshal(body, &got))
	data := got["data"].([]any)
	item := data[0].(map[string]any)
	assert.Equal(t, "https://example.com/edit.png", item["url"])
	result := got["result"].(map[string]any)
	resultData := result["data"].([]any)
	resultItem := resultData[0].(map[string]any)
	assert.Equal(t, "https://example.com/edit.png", resultItem["url"])
	assert.Equal(t, "public_edit_task", got["task_id"])
}
