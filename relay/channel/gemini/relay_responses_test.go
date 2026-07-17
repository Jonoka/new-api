package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiResponsesHandlerReturnsResponsesJSON(t *testing.T) {
	c, recorder := newGeminiResponsesContext(t)
	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{{
			Content: dto.GeminiChatContent{Role: "model", Parts: []dto.GeminiPart{{Text: "hello"}}},
		}},
		UsageMetadata: dto.GeminiUsageMetadata{PromptTokenCount: 2, CandidatesTokenCount: 3, TotalTokenCount: 5},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	usage, relayErr := GeminiResponsesHandler(c, newGeminiResponsesInfo(false), &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.TotalTokens)

	var response dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "response", response.Object)
	assert.Equal(t, "hello", response.Output[0].Content[0].Text)
	assert.Equal(t, 2, response.Usage.InputTokens)
	assert.NotContains(t, recorder.Body.String(), `"candidates"`)
}

func TestGeminiResponsesStreamHandlerReturnsResponsesSSE(t *testing.T) {
	c, recorder := newGeminiResponsesContext(t)
	stop := "STOP"
	first, err := common.Marshal(dto.GeminiChatResponse{
		Candidates:    []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Role: "model", Parts: []dto.GeminiPart{{Text: "hello"}}}}},
		UsageMetadata: dto.GeminiUsageMetadata{PromptTokenCount: 2, CandidatesTokenCount: 3, TotalTokenCount: 5},
	})
	require.NoError(t, err)
	last, err := common.Marshal(dto.GeminiChatResponse{
		Candidates:    []dto.GeminiChatCandidate{{FinishReason: &stop, Content: dto.GeminiChatContent{Role: "model"}}},
		UsageMetadata: dto.GeminiUsageMetadata{PromptTokenCount: 2, CandidatesTokenCount: 3, TotalTokenCount: 5},
	})
	require.NoError(t, err)
	body := strings.Join([]string{"data: " + string(first), "", "data: " + string(last), "", "data: [DONE]", ""}, "\n")

	oldTimeout := rootconstant.StreamingTimeout
	rootconstant.StreamingTimeout = 30
	t.Cleanup(func() { rootconstant.StreamingTimeout = oldTimeout })
	usage, relayErr := GeminiResponsesStreamHandler(c, newGeminiResponsesInfo(true), &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	})
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.TotalTokens)

	output := recorder.Body.String()
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	requireOrderedGeminiResponsesEvents(t, output,
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.completed",
	)
	assert.Contains(t, output, `"input_tokens":2`)
	assert.Contains(t, output, `"output_tokens":3`)
}

func newGeminiResponsesContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "gemini-responses-test")
	return c, recorder
}

func newGeminiResponsesInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		IsStream:        stream,
		RelayMode:       relayconstant.RelayModeResponses,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RequestURLPath:  "/v1/responses",
		DisablePing:     true,
		OriginModelName: "gemini-test",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-test",
		},
	}
}

func requireOrderedGeminiResponsesEvents(t *testing.T, body string, events ...string) {
	t.Helper()
	offset := 0
	for _, event := range events {
		index := strings.Index(body[offset:], event)
		require.NotEqualf(t, -1, index, "missing %q after byte %d", event, offset)
		offset += index + len(event)
	}
}
