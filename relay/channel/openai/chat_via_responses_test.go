package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertResponsesSSEToJSON(t *testing.T) {
	body := []byte(`data: {"type":"response.created","response":{"id":"resp_1","model":"test-model","created_at":1800000000}}
data: {"type":"response.output_text.delta","delta":"Hel"}
data: {"type":"response.output_text.delta","delta":"lo"}
data: {"type":"response.completed","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}
data: [DONE]
`)

	converted, err := convertResponsesSSEToJSON(body)
	require.NoError(t, err)

	var resp dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(converted, &resp))
	require.Equal(t, "resp_1", resp.ID)
	require.Equal(t, "test-model", resp.Model)
	require.NotNil(t, resp.Usage)
	require.Equal(t, 5, resp.Usage.TotalTokens)
	require.Len(t, resp.Output, 1)
	require.Equal(t, "Hello", resp.Output[0].Content[0].Text)
}

func TestConvertResponsesSSEToJSONWithDoneUsage(t *testing.T) {
	body := []byte(`data: {"type":"response.created","response":{"id":"resp_1","model":"test-model","created_at":1800000000}}
data: {"type":"response.output_text.delta","delta":"Hi"}
data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}
data: [DONE]
`)

	converted, err := convertResponsesSSEToJSON(body)
	require.NoError(t, err)

	var resp dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(converted, &resp))
	require.NotNil(t, resp.Usage)
	require.Equal(t, 10, resp.Usage.InputTokens)
	require.Equal(t, 2, resp.Usage.OutputTokens)
	require.Equal(t, 12, resp.Usage.TotalTokens)
	require.Len(t, resp.Output, 1)
	require.Equal(t, "Hi", resp.Output[0].Content[0].Text)
}

func setupResponsesStreamTest(body string) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo, *http.Response) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "test-model",
		},
		RelayFormat: types.RelayFormatOpenAI,
	}

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	return c, recorder, info, resp
}

func TestOaiResponsesStreamHandlerReadsDoneUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, _, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 12, usage.TotalTokens)
}

func TestOaiResponsesToChatStreamHandlerReadsDoneUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 12, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), `"content":"Hi"`)
}

func TestOaiResponsesStreamHandlerDoesNotBillPromptOnlyWithoutUsageOrOutput(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, _, info, resp := setupResponsesStreamTest(body)
	info.SetEstimatePromptTokens(9)

	usage, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.PromptTokens)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Equal(t, 0, usage.TotalTokens)
}

func TestOaiResponsesStreamHandlerFallsBackToEstimatedPromptTokensWithOutput(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, _, info, resp := setupResponsesStreamTest(body)
	info.SetEstimatePromptTokens(9)

	usage, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 9, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}
