package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
			"result":{"data":[{"height":2560,"url":"https://example.com/nested.png","width":1440}]},
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
