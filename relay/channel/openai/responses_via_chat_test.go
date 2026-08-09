package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesViaChatAdaptorConvertsRequestAndURL(t *testing.T) {
	info := newOpenAIResponsesViaChatInfo(false)
	info.ChannelBaseUrl = "https://chat-only.example"
	info.SupportStreamOptions = true
	stream := true
	request := dto.OpenAIResponsesRequest{
		Model:                "gpt-test",
		Input:                mustOpenAIResponsesRaw(t, "hello"),
		Stream:               &stream,
		PromptCacheKey:       mustOpenAIResponsesRaw(t, "cache-key"),
		PromptCacheRetention: mustOpenAIResponsesRaw(t, "24h"),
		Reasoning:            &dto.Reasoning{Effort: "high"},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	chatRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Equal(t, "hello", chatRequest.Messages[0].StringContent())
	assert.Equal(t, "cache-key", chatRequest.PromptCacheKey)
	assert.JSONEq(t, `"24h"`, string(chatRequest.PromptCacheRetention))
	assert.Equal(t, "high", chatRequest.ReasoningEffort)
	require.NotNil(t, chatRequest.StreamOptions)
	assert.True(t, chatRequest.StreamOptions.IncludeUsage)
	assert.Equal(t, types.RelayFormatOpenAI, info.FinalRequestRelayFormat)

	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://chat-only.example/v1/chat/completions", url)
}

func TestOaiChatToResponsesHandlerReturnsResponsesJSONAndCacheUsage(t *testing.T) {
	c, recorder := newOpenAIResponsesViaChatContext(t)
	info := newOpenAIResponsesViaChatInfo(false)
	body := []byte(`{
		"id":"chatcmpl_upstream","object":"chat.completion","created":1710000000,"model":"gpt-test",
		"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":4,"cache_write_tokens":3}}
	}`)

	usage, relayErr := OaiChatToResponsesHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.GetCacheCreationTokens())

	var response dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "response", response.Object)
	assert.Equal(t, "hello", response.Output[0].Content[0].Text)
	require.NotNil(t, response.Usage)
	assert.Equal(t, 10, response.Usage.InputTokens)
	assert.Equal(t, 3, response.Usage.GetCacheCreationTokens())
	assert.NotContains(t, recorder.Body.String(), `"choices"`)
}

func TestOaiChatToResponsesStreamHandlerReturnsOrderedResponsesEvents(t *testing.T) {
	c, recorder := newOpenAIResponsesViaChatContext(t)
	info := newOpenAIResponsesViaChatInfo(true)
	body := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	usage, relayErr := OaiChatToResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	})
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.TotalTokens)

	output := recorder.Body.String()
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	requireOrderedOpenAIResponsesEvents(t, output,
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.completed",
	)
	assert.Contains(t, output, `"input_tokens":2`)
	assert.Contains(t, output, `"output_tokens":3`)
}

func newOpenAIResponsesViaChatContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-via-chat-test")
	return c, recorder
}

func newOpenAIResponsesViaChatInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		IsStream:        stream,
		RelayMode:       relayconstant.RelayModeResponses,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RequestURLPath:  "/v1/responses",
		OriginModelName: "gpt-test",
		DisablePing:     true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-test",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ResponsesToChatEnabled: true,
			},
		},
	}
}

func mustOpenAIResponsesRaw(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}

func requireOrderedOpenAIResponsesEvents(t *testing.T, body string, events ...string) {
	t.Helper()
	offset := 0
	for _, event := range events {
		index := strings.Index(body[offset:], event)
		require.NotEqualf(t, -1, index, "missing %q after byte %d", event, offset)
		offset += index + len(event)
	}
}
