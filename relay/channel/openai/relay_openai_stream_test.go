package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupOaiStreamTest(body io.Reader) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo, *http.Response) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		StartTime:   time.Now(),
	}

	resp := &http.Response{Body: io.NopCloser(body)}
	return c, recorder, info, resp
}

func chatCompletionSSE(data string) string {
	return fmt.Sprintf("data: %s\n", data)
}

func TestOaiStreamHandlerForwardsCurrentChunkImmediately(t *testing.T) {
	pr, pw := io.Pipe()
	c, recorder, info, resp := setupOaiStreamTest(pr)

	done := make(chan *types.NewAPIError, 1)
	go func() {
		_, err := OaiStreamHandler(c, info, resp)
		done <- err
	}()

	firstChunk := `{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"first"},"finish_reason":null}]}`
	_, err := pw.Write([]byte(chatCompletionSSE(firstChunk)))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return strings.Contains(recorder.Body.String(), `"content":"first"`)
	}, 500*time.Millisecond, 10*time.Millisecond, "first content chunk should be sent before the next SSE chunk")

	_, err = pw.Write([]byte("data: [DONE]\n"))
	require.NoError(t, err)
	require.NoError(t, pw.Close())
	require.Nil(t, <-done)
}

func TestOaiStreamHandlerDoesNotDuplicateLastChunk(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"last"},"finish_reason":null}]}`),
		"data: [DONE]\n",
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)

	output := recorder.Body.String()
	assert.Equal(t, 1, strings.Count(output, `"content":"last"`))
	assert.Contains(t, output, "[DONE]")
}

func TestOaiStreamHandlerSkipsUsageOnlyChunkWhenNotRequested(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"body"},"finish_reason":null}]}`),
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`),
		"data: [DONE]\n",
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))
	info.ShouldIncludeUsage = false

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, &dto.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}, usage)

	output := recorder.Body.String()
	assert.Contains(t, output, `"content":"body"`)
	assert.NotContains(t, output, `"usage":{"prompt_tokens":3`)
	assert.Contains(t, output, "[DONE]")
}
