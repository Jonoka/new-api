package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSyncResponsesStreamFlagUsesRelayInfo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isStream bool
		initial  *bool
	}{
		{name: "stream overrides nil", isStream: true, initial: nil},
		{name: "stream overrides false", isStream: true, initial: common.GetPointer(false)},
		{name: "non stream overrides nil", isStream: false, initial: nil},
		{name: "non stream overrides true", isStream: false, initial: common.GetPointer(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{IsStream: tc.isStream}
			req := &dto.OpenAIResponsesRequest{Stream: tc.initial}

			syncResponsesStreamFlag(info, req)

			require.NotNil(t, req.Stream)
			require.Equal(t, tc.isStream, *req.Stream)
		})
	}
}

type captureResponsesAdaptor struct {
	requests [][]byte
	respBody string
	responses []*http.Response
}

func (a *captureResponsesAdaptor) Init(info *relaycommon.RelayInfo) {}
func (a *captureResponsesAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return "https://example.com/v1/responses", nil
}
func (a *captureResponsesAdaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	return nil
}
func (a *captureResponsesAdaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return request, nil
}
func (a *captureResponsesAdaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}
func (a *captureResponsesAdaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, nil
}
func (a *captureResponsesAdaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}
func (a *captureResponsesAdaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, nil
}
func (a *captureResponsesAdaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}
func (a *captureResponsesAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	a.requests = append(a.requests, body)
	if len(a.responses) > 0 {
		resp := a.responses[0]
		a.responses = a.responses[1:]
		return resp, nil
	}
	respBody := a.respBody
	if respBody == "" {
		respBody = `{"id":"resp_default","model":"test-model","created_at":1800000000,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi"}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
	}, nil
}
func (a *captureResponsesAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	return nil, nil
}
func (a *captureResponsesAdaptor) GetModelList() []string { return nil }
func (a *captureResponsesAdaptor) GetChannelName() string { return "capture" }
func (a *captureResponsesAdaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return request, nil
}
func (a *captureResponsesAdaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return request, nil
}

var _ channel.Adaptor = (*captureResponsesAdaptor)(nil)

func TestChatCompletionsViaResponsesAttachesPreviousResponseIDAndTrimsInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(nil))
	c.Set(common.RequestIdKey, "req-test-1")

	info := &relaycommon.RelayInfo{
		RequestId: "req-test-1",
		RequestHeaders: map[string]string{
			"X-Claude-Code-Session-Id": "sess-attach-1",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           0,
			ChannelId:         7001,
			UpstreamModelName: "test-model",
		},
		OriginModelName: "test-model",
		RelayFormat:     types.RelayFormatClaude,
	}

	openAIReq := &dto.GeneralOpenAIRequest{
		Model: "test-model",
		Messages: []dto.Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", Content: "prior assistant"},
			func() dto.Message {
				msg := dto.Message{Role: "assistant"}
				msg.SetToolCalls([]dto.ToolCallRequest{
					{
						ID:   "call_1",
						Type: "function",
						Function: dto.FunctionRequest{
							Name:      "toolA",
							Arguments: `{"x":1}`,
						},
					},
				})
				return msg
			}(),
			{Role: "tool", ToolCallId: "call_1", Content: "ok"},
			{Role: "user", Content: "next turn"},
		},
		PromptCacheKey: "sess-attach-1",
	}
	responsesSeed, err := service.ChatCompletionsRequestToResponsesRequest(openAIReq)
	require.NoError(t, err)
	service.BindOpenAIResponsesContinuationResponseID(info, responsesSeed, "resp_prev_attach_1")

	adaptor := &captureResponsesAdaptor{}
	usage, apiErr := chatCompletionsViaResponses(c, info, adaptor, openAIReq)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Len(t, adaptor.requests, 1)
	require.Equal(t, "resp_prev_attach_1", gjson.GetBytes(adaptor.requests[0], "previous_response_id").String())
	require.JSONEq(t, `[
		{"type":"function_call","call_id":"call_1","name":"toolA","arguments":"{\"x\":1}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"},
		{"role":"user","content":"next turn"}
	]`, gjson.GetBytes(adaptor.requests[0], "input").Raw)
}

func TestChatCompletionsViaResponsesRetriesWithoutPreviousResponseIDOnWebSocketV2Error(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(nil))
	c.Set(common.RequestIdKey, "req-test-2")

	info := &relaycommon.RelayInfo{
		RequestId: "req-test-2",
		RequestHeaders: map[string]string{
			"X-Claude-Code-Session-Id": "sess-retry-1",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           0,
			ChannelId:         7002,
			UpstreamModelName: "test-model",
		},
		OriginModelName: "test-model",
		RelayFormat:     types.RelayFormatClaude,
	}

	openAIReq := &dto.GeneralOpenAIRequest{
		Model: "test-model",
		Messages: []dto.Message{
			{Role: "user", Content: "first"},
			{Role: "user", Content: "second"},
		},
		PromptCacheKey: "sess-retry-1",
	}
	responsesSeed, err := service.ChatCompletionsRequestToResponsesRequest(openAIReq)
	require.NoError(t, err)
	service.BindOpenAIResponsesContinuationResponseID(info, responsesSeed, "resp_prev_retry_1")

	adaptor := &captureResponsesAdaptor{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"message":"previous_response_id is only supported on Responses WebSocket v2","type":"invalid_request_error"}}`,
				)),
			},
			{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_retry_ok","model":"test-model","created_at":1800000000,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi again"}]}],"usage":{"input_tokens":11,"output_tokens":3,"total_tokens":14}}`,
				)),
			},
		},
	}

	usage, apiErr := chatCompletionsViaResponses(c, info, adaptor, openAIReq)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Len(t, adaptor.requests, 2)
	require.Equal(t, "resp_prev_retry_1", gjson.GetBytes(adaptor.requests[0], "previous_response_id").String())
	require.False(t, gjson.GetBytes(adaptor.requests[1], "previous_response_id").Exists())
	require.Equal(t, "resp_retry_ok", service.GetOpenAIResponsesContinuationResponseID(info, responsesSeed))
}
