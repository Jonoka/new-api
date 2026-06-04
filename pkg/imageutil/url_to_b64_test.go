package imageutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertImageURLResponseToB64SkipsExistingB64(t *testing.T) {
	body := []byte(`{"created":1,"data":[{"b64_json":"already"}]}`)
	got, changed, err := ConvertImageURLResponseToB64WithClient(context.Background(), body, nil, 20<<20)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.JSONEq(t, string(body), string(got))
}

func TestConvertImageURLResponseToB64DownloadsURLAndRemovesIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer server.Close()

	body := []byte(`{"created":1,"data":[{"height":1280,"url":"` + server.URL + `/image.png","width":720}],"usage":{"total_cost":8}}`)
	got, changed, err := ConvertImageURLResponseToB64WithClient(context.Background(), body, server.Client(), 20<<20)
	require.NoError(t, err)
	assert.True(t, changed)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(got, &payload))
	data := payload["data"].([]any)
	item := data[0].(map[string]any)
	assert.Equal(t, "cG5nLWJ5dGVz", item["b64_json"])
	_, hasURL := item["url"]
	assert.False(t, hasURL)
	assert.Equal(t, float64(1280), item["height"])
	assert.Equal(t, float64(720), item["width"])
}

func TestConvertImageURLResponseToB64RejectsTooLargeImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("too-large"))
	}))
	defer server.Close()

	body := []byte(`{"data":[{"url":"` + server.URL + `/image.png"}]}`)
	_, changed, err := ConvertImageURLResponseToB64WithClient(context.Background(), body, server.Client(), int64(len("too")))
	require.Error(t, err)
	assert.False(t, changed)
}

func TestConvertImageURLResponseToB64RejectsNonImageContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("not-image"))
	}))
	defer server.Close()

	body := []byte(`{"data":[{"url":"` + server.URL + `/image.png"}]}`)
	_, changed, err := ConvertImageURLResponseToB64WithClient(context.Background(), body, server.Client(), 20<<20)
	require.Error(t, err)
	assert.False(t, changed)
	assert.True(t, strings.Contains(err.Error(), "non-image"))
}
