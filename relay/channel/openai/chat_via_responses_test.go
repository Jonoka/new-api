package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
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

func TestOaiResponsesStreamHandlerDropsPreOutputNestedCapacityError(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"test-model"}}`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
		`data: {"type":"response.output_item.added","item":{"id":"rs_1","type":"reasoning","status":"in_progress"}}`,
		`data: {"type":"response.content_part.added","item_id":"msg_1","part":{"type":"output_text"}}`,
		`data: {"type":"response.web_search_call.searching","item_id":"ws_1"}`,
		`data: {"type":"response.failed","response":{"error":{"code":"model_at_capacity","message":"Selected model is at capacity. Please try a different model."}}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.True(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesStreamHandlerDropsPreOutputTopLevelCapacityError(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.queued"}`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":""}`,
		`data: {"type":"error","code":"overloaded","message":"Selected model is at capacity. Please try a different model.","param":"model"}`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.True(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesStreamHandlerDropsPreOutputTopLevelNestedCapacityError(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"test-model"}}`,
		`data: {"type":"error","error":{"type":"server_error","code":"model_at_capacity","message":"Selected model is at capacity. Please try a different model."},"sequence_number":3}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.Equal(t, types.ErrorCode("model_at_capacity"), err.GetErrorCode())
	require.True(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesStreamHandlerFlushesPreambleOnceOnSuccess(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"test-model"}}`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1"}}`,
		`data: {"type":"response.output_item.added","item":{"id":"msg_1","type":"message","status":"in_progress"}}`,
		`data: {"type":"response.content_part.added","item_id":"msg_1","part":{"type":"output_text"}}`,
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"test-model"}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	streamBody := recorder.Body.String()
	for _, eventType := range []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.completed",
	} {
		require.Equal(t, 1, strings.Count(streamBody, "event: "+eventType+"\n"))
	}
	require.Less(t, strings.Index(streamBody, "event: response.created\n"), strings.Index(streamBody, "event: response.in_progress\n"))
	require.Less(t, strings.Index(streamBody, "event: response.in_progress\n"), strings.Index(streamBody, "event: response.output_item.added\n"))
	require.Less(t, strings.Index(streamBody, "event: response.output_item.added\n"), strings.Index(streamBody, "event: response.content_part.added\n"))
	require.Less(t, strings.Index(streamBody, "event: response.content_part.added\n"), strings.Index(streamBody, "event: response.output_text.delta\n"))
	require.False(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
}

func TestOaiResponsesStreamHandlerAccountsForBufferedToolEventOnlyWhenCommitted(t *testing.T) {
	for _, test := range []struct {
		name          string
		terminalEvent string
		wantCalls     int
		wantRetry     bool
	}{
		{
			name:          "successful stream",
			terminalEvent: `{"type":"response.completed","response":{"id":"resp_1"}}`,
			wantCalls:     1,
		},
		{
			name:          "discarded capacity attempt",
			terminalEvent: `{"type":"response.failed","response":{"error":{"code":"model_at_capacity","message":"Selected model is at capacity"}}}`,
			wantRetry:     true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := strings.Join([]string{
				`data: {"type":"response.output_item.done","item":{"id":"ws_1","type":"web_search_call","status":"completed"}}`,
				"data: " + test.terminalEvent,
				"",
			}, "\n")
			c, recorder, info, resp := setupResponsesStreamTest(body)
			tool := &relaycommon.BuildInToolInfo{ToolName: dto.BuildInToolWebSearchPreview}
			info.ResponsesUsageInfo = &relaycommon.ResponsesUsageInfo{
				BuiltInTools: map[string]*relaycommon.BuildInToolInfo{dto.BuildInToolWebSearchPreview: tool},
			}

			usage, err := OaiResponsesStreamHandler(c, info, resp)

			require.Equal(t, test.wantCalls, tool.CallCount)
			require.Equal(t, test.wantRetry, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
			if test.wantRetry {
				require.Nil(t, usage)
				require.Error(t, err)
				require.Empty(t, recorder.Body.String())
			} else {
				require.NotNil(t, usage)
				require.Nil(t, err)
				require.Contains(t, recorder.Body.String(), "event: response.output_item.done\n")
			}
		})
	}
}

func TestOaiResponsesStreamHandlerDoesNotMarkPostOutputCapacityError(t *testing.T) {
	for _, test := range []struct {
		name  string
		event string
	}{
		{name: "text", event: `{"type":"response.output_text.delta","delta":"Hi"}`},
		{name: "reasoning", event: `{"type":"response.reasoning_summary_text.delta","delta":"Thinking"}`},
		{name: "tool arguments", event: `{"type":"response.function_call_arguments.delta","delta":"{\"city\":\"Paris\"}"}`},
		{name: "raw tool arguments", event: `{"type":"response.function_call_arguments.done","arguments":{"city":"Paris"}}`},
		{name: "image", event: `{"type":"response.image_generation_call.partial_image","partial_image_b64":"aW1hZ2U="}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := strings.Join([]string{
				`data: {"type":"response.created","response":{"id":"resp_1"}}`,
				"data: " + test.event,
				`data: {"type":"response.failed","response":{"error":{"code":"model_at_capacity","message":"Selected model is at capacity"}}}`,
				"",
			}, "\n")
			c, recorder, info, resp := setupResponsesStreamTest(body)

			usage, err := OaiResponsesStreamHandler(c, info, resp)

			require.Nil(t, err)
			require.NotNil(t, usage)
			require.Contains(t, recorder.Body.String(), "event: response.failed\n")
			require.False(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
		})
	}
}

func TestOaiResponsesStreamHandlerDoesNotRetryCapacityErrorWithOutputPayload(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
	}{
		{name: "text", output: `{"type":"message","content":[{"type":"output_text","text":"partial output"}]}`},
		{name: "refusal", output: `{"type":"message","content":[{"type":"refusal","refusal":"cannot comply"}]}`},
		{name: "custom tool input", output: `{"type":"custom_tool_call","input":"run diagnostics"}`},
		{name: "computer action", output: `{"type":"computer_call","action":{"type":"click","x":10,"y":20}}`},
		{name: "image result object", output: `{"type":"image_generation_call","result":{"b64_json":"aW1hZ2U="}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := strings.Join([]string{
				`data: {"type":"response.failed","response":{"error":{"code":"model_at_capacity","message":"Selected model is at capacity"},"output":[` + test.output + `]}}`,
				"",
			}, "\n")
			c, recorder, info, resp := setupResponsesStreamTest(body)

			usage, err := OaiResponsesStreamHandler(c, info, resp)

			require.Nil(t, err)
			require.NotNil(t, usage)
			require.Contains(t, recorder.Body.String(), "event: response.failed\n")
			require.False(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
		})
	}
}

func TestOaiResponsesStreamHandlerRetriesCapacityErrorWithEmptyOutputPayload(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		`data: {"type":"response.failed","response":{"error":{"code":"model_at_capacity","message":"Selected model is at capacity"},"output":[]}}`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.True(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesStreamHandlerDoesNotRetryTopLevelCapacityErrorWithOutputPayload(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"error","code":"model_at_capacity","message":"Selected model is at capacity","output":[{"type":"message","content":[{"type":"output_text","text":"partial output"}]}]}`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Contains(t, recorder.Body.String(), "event: error\n")
	require.False(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
}

func TestOaiResponsesStreamHandlerPreservesPreOutputNonCapacityError(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		`data: {"type":"response.failed","response":{"error":{"code":"invalid_request","message":"Invalid input"}}}`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Contains(t, recorder.Body.String(), "event: response.created\n")
	require.Contains(t, recorder.Body.String(), "event: response.failed\n")
	require.False(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
}

func TestOaiResponsesStreamHandlerCommitsWhenPreOutputEventLimitIsExceeded(t *testing.T) {
	lines := make([]string, 0, responsesPreOutputMaxEvents+3)
	for i := 0; i <= responsesPreOutputMaxEvents; i++ {
		lines = append(lines, `data: {"type":"response.created","response":{"id":"resp_1"}}`)
	}
	lines = append(lines,
		`data: {"type":"response.failed","response":{"error":{"code":"model_at_capacity","message":"Selected model is at capacity"}}}`,
		"",
	)
	c, recorder, info, resp := setupResponsesStreamTest(strings.Join(lines, "\n"))

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, responsesPreOutputMaxEvents+1, strings.Count(recorder.Body.String(), "event: response.created\n"))
	require.Contains(t, recorder.Body.String(), "event: response.failed\n")
	require.False(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
}

func TestOaiResponsesStreamHandlerCommitsWhenPreOutputByteLimitIsExceeded(t *testing.T) {
	largeMetadata := strings.Repeat("x", responsesPreOutputMaxBytes)
	body := strings.Join([]string{
		`data: {"type":"response.created","metadata":"` + largeMetadata + `"}`,
		`data: {"type":"response.failed","response":{"error":{"code":"model_at_capacity","message":"Selected model is at capacity"}}}`,
		"",
	}, "\n")
	c, recorder, info, resp := setupResponsesStreamTest(body)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Contains(t, recorder.Body.String(), "event: response.created\n")
	require.Contains(t, recorder.Body.String(), "event: response.failed\n")
	require.False(t, c.GetBool(string(constant.ContextKeyResponsesPreOutputRetry)))
}
