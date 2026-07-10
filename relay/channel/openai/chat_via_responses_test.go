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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestConvertResponsesSSEToJSONWithEventLines(t *testing.T) {
	body := []byte(`event: response.created
data: {"type":"response.created","response":{"id":"resp_1","model":"test-model","created_at":1800000000}}
event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"Hi"}
event: response.done
data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":8},"output_tokens":2,"total_tokens":12}}}
data: [DONE]
`)

	converted, err := convertResponsesSSEToJSON(body)
	require.NoError(t, err)

	var resp dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(converted, &resp))
	require.NotNil(t, resp.Usage)
	require.NotNil(t, resp.Usage.InputTokensDetails)
	require.Equal(t, 8, resp.Usage.InputTokensDetails.CachedTokens)
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

func requireResponsesSSEDataByType(t *testing.T, body string, eventType string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		data, ok := normalizeResponsesStreamJSONData(line)
		if !ok {
			continue
		}
		if gjson.Get(data, "type").String() == eventType {
			return data
		}
	}
	require.Failf(t, "missing responses SSE event", "event type %q not found in body:\n%s", eventType, body)
	return ""
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

func TestOaiResponsesHandlerMapsCacheCreationTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":169969,"output_tokens":60,"total_tokens":170029,"input_tokens_details":{"cached_tokens":168704,"cache_creation_tokens":1265}}}`
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 169969, usage.PromptTokens)
	require.Equal(t, 168704, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 1265, usage.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 60, usage.CompletionTokens)
	require.EqualValues(t, 1265, gjson.Get(recorder.Body.String(), "usage.input_tokens_details.cache_creation_tokens").Int())
	require.EqualValues(t, 1265, gjson.Get(recorder.Body.String(), "usage.input_tokens_details.cached_creation_tokens").Int())
	require.EqualValues(t, 1265, gjson.Get(recorder.Body.String(), "usage.cache_creation_tokens").Int())
	require.EqualValues(t, 1265, gjson.Get(recorder.Body.String(), "usage.cache_write_tokens").Int())
}

func TestOaiResponsesHandlerMapsCacheWriteTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_write_tokens":30}}}`
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
	require.EqualValues(t, 30, gjson.Get(recorder.Body.String(), "usage.input_tokens_details.cache_write_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(recorder.Body.String(), "usage.cache_creation_input_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(recorder.Body.String(), "usage.cache_write_input_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(recorder.Body.String(), "usage.cache_write_tokens").Int())
}

func TestOaiResponsesHandlerMapsTopLevelCacheCreationAliases(t *testing.T) {
	for name, body := range map[string]string{
		"cache_creation_input_tokens": `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"cache_creation_input_tokens":30}}`,
		"cache_write_input_tokens":    `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"cache_write_input_tokens":30}}`,
		"cache_write_tokens":          `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"cache_write_tokens":30}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _, info, resp := setupResponsesStreamTest(body)

			usage, err := OaiResponsesHandler(c, info, resp)
			require.Nil(t, err)
			require.NotNil(t, usage)
			require.Equal(t, 100, usage.PromptTokens)
			require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
		})
	}
}

func TestOaiResponsesStreamHandlerMapsCompletedCacheCreationTokens(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":169969,"output_tokens":60,"total_tokens":170029,"input_tokens_details":{"cached_tokens":168704,"cache_creation_tokens":1265}}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 169969, usage.PromptTokens)
	require.Equal(t, 168704, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 1265, usage.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 60, usage.CompletionTokens)
	eventData := requireResponsesSSEDataByType(t, recorder.Body.String(), "response.completed")
	require.EqualValues(t, 1265, gjson.Get(eventData, "response.usage.input_tokens_details.cache_creation_tokens").Int())
	require.EqualValues(t, 1265, gjson.Get(eventData, "response.usage.input_tokens_details.cached_creation_tokens").Int())
	require.EqualValues(t, 1265, gjson.Get(eventData, "response.usage.cache_creation_tokens").Int())
	require.EqualValues(t, 1265, gjson.Get(eventData, "response.usage.cache_write_tokens").Int())
}

func TestOaiResponsesStreamHandlerMapsDoneCacheWriteTokens(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_write_tokens":30}}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
	eventData := requireResponsesSSEDataByType(t, recorder.Body.String(), "response.done")
	require.EqualValues(t, 30, gjson.Get(eventData, "response.usage.input_tokens_details.cache_write_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(eventData, "response.usage.cache_creation_input_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(eventData, "response.usage.cache_write_input_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(eventData, "response.usage.cache_write_tokens").Int())
}

func TestOaiResponsesUsageMapsLegacyCachedCreationTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cached_creation_tokens":30}}}`
	c, _, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesUsagePrefersCacheCreationTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_creation_tokens":30,"cached_creation_tokens":999}}}`
	c, _, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesUsagePrefersCacheWriteTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_write_tokens":20,"cache_creation_tokens":30,"cached_creation_tokens":999}}}`
	c, _, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 20, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesUsageKeepsExplicitZeroCacheCreationTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_creation_tokens":0,"cached_creation_tokens":999}}}`
	c, _, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 999, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesUsageKeepsMissingCacheCreationAsZero(t *testing.T) {
	body := `{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70}}}`
	c, _, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 0, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesUsageInfersGPT56CacheCreationTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"gpt-5.6-sol","created_at":1800000000,"usage":{"input_tokens":188727,"output_tokens":368,"total_tokens":189095,"input_tokens_details":{"cached_tokens":185088}}}`
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 185088, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 3639, usage.PromptTokensDetails.CachedCreationTokens)
	require.EqualValues(t, 3639, gjson.Get(recorder.Body.String(), "usage.input_tokens_details.cache_creation_tokens").Int())
	require.EqualValues(t, 3639, gjson.Get(recorder.Body.String(), "usage.input_tokens_details.cached_creation_tokens").Int())
	require.EqualValues(t, 3639, gjson.Get(recorder.Body.String(), "usage.cache_creation_tokens").Int())
	require.EqualValues(t, 3639, gjson.Get(recorder.Body.String(), "usage.cache_write_tokens").Int())
}

func TestOaiResponsesUsageInfersGPT56CacheCreationWithTopLevelZeroAlias(t *testing.T) {
	body := `{"id":"resp_1","model":"gpt-5.6-sol","created_at":1800000000,"usage":{"input_tokens":188727,"output_tokens":368,"total_tokens":189095,"cache_creation_input_tokens":0,"input_tokens_details":{"cached_tokens":185088}}}`
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 185088, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 3639, usage.PromptTokensDetails.CachedCreationTokens)
	require.EqualValues(t, 3639, gjson.Get(recorder.Body.String(), "usage.cache_creation_input_tokens").Int())
	require.EqualValues(t, 3639, gjson.Get(recorder.Body.String(), "usage.cache_write_tokens").Int())
}

func TestOaiResponsesUsageDoesNotInferCacheCreationForOtherModels(t *testing.T) {
	body := `{"id":"resp_1","model":"gpt-5.5","created_at":1800000000,"usage":{"input_tokens":188727,"output_tokens":368,"total_tokens":189095,"input_tokens_details":{"cached_tokens":185088}}}`
	c, _, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 185088, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 0, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesCompactionHandlerMapsCacheCreationTokens(t *testing.T) {
	body := `{"id":"resp_1","usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_creation_tokens":30}}}`
	c, _, _, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesCompactionHandler(c, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesCompactionHandlerMapsCacheWriteTokens(t *testing.T) {
	body := `{"id":"resp_1","usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_write_tokens":30}}}`
	c, recorder, _, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesCompactionHandler(c, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
	require.EqualValues(t, 30, gjson.Get(recorder.Body.String(), "usage.cache_write_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(recorder.Body.String(), "usage.cache_creation_input_tokens").Int())
}

func TestOaiResponsesCompactionHandlerInfersGPT56CacheCreationTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"gpt-5.6-terra","usage":{"input_tokens":188727,"output_tokens":368,"total_tokens":189095,"input_tokens_details":{"cached_tokens":185088}}}`
	c, recorder, _, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesCompactionHandler(c, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 188727, usage.PromptTokens)
	require.Equal(t, 185088, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 3639, usage.PromptTokensDetails.CachedCreationTokens)
	require.EqualValues(t, 3639, gjson.Get(recorder.Body.String(), "usage.input_tokens_details.cache_creation_tokens").Int())
	require.EqualValues(t, 3639, gjson.Get(recorder.Body.String(), "usage.cache_write_tokens").Int())
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

func TestOaiResponsesToChatStreamHandlerMapsCacheWriteTokens(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_write_tokens":30}}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)
	info.ShouldIncludeUsage = true

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 5, usage.CompletionTokens)
	require.Contains(t, recorder.Body.String(), `"cache_write_tokens":30`)
	require.Contains(t, recorder.Body.String(), `"cache_creation_tokens":30`)
}

func TestOaiResponsesToChatHandlerMapsCacheWriteTokens(t *testing.T) {
	body := `{"id":"resp_1","model":"test-model","created_at":1800000000,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi"}]}],"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":70,"cache_write_tokens":30}}}`
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesToChatHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 5, usage.CompletionTokens)
	require.EqualValues(t, 30, gjson.Get(recorder.Body.String(), "usage.cache_write_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(recorder.Body.String(), "usage.cache_creation_tokens").Int())
}

func TestOaiResponsesToChatStreamHandlerSkipsNestedEventData(t *testing.T) {
	body := strings.Join([]string{
		`data: event: response.output_text.delta`,
		`data: data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: event: response.done`,
		`data: data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":8},"output_tokens":2,"total_tokens":12}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 8, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 12, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), `"content":"Hi"`)
}

func TestOaiResponsesHandlerConvertsEventPrefixedSSEBody(t *testing.T) {
	body := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`event: response.done`,
		`data: {"type":"response.done","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":8},"output_tokens":2,"total_tokens":12}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 8, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 12, usage.TotalTokens)
	var out dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal([]byte(recorder.Body.String()), &out))
	require.NotNil(t, out.Usage)
	require.NotNil(t, out.Usage.InputTokensDetails)
	require.Equal(t, 8, out.Usage.InputTokensDetails.CachedTokens)
	require.Len(t, out.Output, 1)
	require.Equal(t, "Hi", out.Output[0].Content[0].Text)
}

func TestOaiResponsesToChatHandlerBindsContinuationResponseID(t *testing.T) {
	body := `{"id":"resp_bind_1","model":"test-model","created_at":1800000000,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi"}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}`
	c, _, info, resp := setupResponsesStreamTest(body)
	req := &dto.OpenAIResponsesRequest{
		PromptCacheKey: []byte(`"cache-bind-1"`),
	}
	info.Request = req

	usage, err := OaiResponsesToChatHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, "resp_bind_1", service.GetOpenAIResponsesContinuationResponseID(info, req))
}

func TestOaiResponsesToChatStreamHandlerBindsContinuationResponseID(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.done","response":{"id":"resp_bind_stream_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, _, info, resp := setupResponsesStreamTest(body)
	req := &dto.OpenAIResponsesRequest{
		PromptCacheKey: []byte(`"cache-bind-stream-1"`),
	}
	info.Request = req

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, "resp_bind_stream_1", service.GetOpenAIResponsesContinuationResponseID(info, req))
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
