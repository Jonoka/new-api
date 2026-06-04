package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertImageTaskResponseToB64IfRequestedConvertsNestedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("poll-image"))
	}))
	defer server.Close()

	task := &model.Task{PrivateData: model.TaskPrivateData{
		RequestPath:    "/v1/images/generations",
		ResponseFormat: "b64_json",
	}}
	body := []byte(`{"status":"succeeded","result":{"data":[{"height":2560,"url":"` + server.URL + `/img.png","width":1440}]}}`)

	got, err := convertImageTaskResponseToB64IfRequested(context.Background(), task, body)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(got, &payload))
	result := payload["result"].(map[string]any)
	data := result["data"].([]any)
	item := data[0].(map[string]any)
	assert.Equal(t, "cG9sbC1pbWFnZQ==", item["b64_json"])
	_, hasURL := item["url"]
	assert.False(t, hasURL)
}

func TestConvertImageTaskResponseToB64IfRequestedSkipsNonB64Request(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{
		RequestPath: "/v1/images/generations",
	}}
	body := []byte(`{"data":[{"url":"https://example.com/img.png"}]}`)

	got, err := convertImageTaskResponseToB64IfRequested(context.Background(), task, body)
	require.NoError(t, err)
	assert.JSONEq(t, string(body), string(got))
}
