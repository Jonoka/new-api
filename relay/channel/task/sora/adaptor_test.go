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
	assert.Equal(t, "public_task", got["task_id"])
}
