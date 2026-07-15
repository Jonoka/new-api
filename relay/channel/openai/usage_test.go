package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIUsageTokenCounts(t *testing.T) {
	tests := []struct {
		name     string
		usage    dto.Usage
		modified bool
		prompt   int
		complete int
		total    int
	}{
		{
			name:     "标准字段保持不变",
			usage:    dto.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30, InputTokens: 10, OutputTokens: 20},
			modified: false,
			prompt:   10,
			complete: 20,
			total:    30,
		},
		{
			name:     "使用 input output 别名补齐",
			usage:    dto.Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
			modified: true,
			prompt:   10,
			complete: 20,
			total:    30,
		},
		{
			name:     "混合格式使用 output tokens 补齐输出",
			usage:    dto.Usage{PromptTokens: 10, OutputTokens: 20, TotalTokens: 30},
			modified: true,
			prompt:   10,
			complete: 20,
			total:    30,
		},
		{
			name:     "使用 total 减 prompt 补齐输出",
			usage:    dto.Usage{PromptTokens: 10, TotalTokens: 30},
			modified: true,
			prompt:   10,
			complete: 20,
			total:    30,
		},
		{
			name:     "使用 total 减 completion 补齐输入",
			usage:    dto.Usage{CompletionTokens: 20, TotalTokens: 30},
			modified: true,
			prompt:   10,
			complete: 20,
			total:    30,
		},
		{
			name:     "标准输出优先且不重复计算",
			usage:    dto.Usage{PromptTokens: 10, CompletionTokens: 20, OutputTokens: 99, TotalTokens: 30},
			modified: false,
			prompt:   10,
			complete: 20,
			total:    30,
		},
		{
			name:     "只有 total 时不推断为全部输出",
			usage:    dto.Usage{TotalTokens: 30},
			modified: false,
			prompt:   0,
			complete: 0,
			total:    30,
		},
		{
			name:     "规范与输入输出之和不一致的 total",
			usage:    dto.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 10},
			modified: true,
			prompt:   10,
			complete: 20,
			total:    30,
		},
		{
			name:     "别名存在时不使用冲突的 total 差值",
			usage:    dto.Usage{PromptTokens: 10, OutputTokens: 20, TotalTokens: 999},
			modified: true,
			prompt:   10,
			complete: 20,
			total:    30,
		},
		{
			name:     "标准字段优先于冲突别名",
			usage:    dto.Usage{PromptTokens: 10, InputTokens: 11, CompletionTokens: 20, OutputTokens: 21, TotalTokens: 40},
			modified: true,
			prompt:   10,
			complete: 20,
			total:    30,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modified := normalizeOpenAIUsageTokenCounts(&test.usage)
			require.Equal(t, test.modified, modified)
			require.Equal(t, test.prompt, test.usage.PromptTokens)
			require.Equal(t, test.complete, test.usage.CompletionTokens)
			require.Equal(t, test.total, test.usage.TotalTokens)
		})
	}
}

func TestHandleLastResponseNormalizesAliasOnlyUsage(t *testing.T) {
	lastStreamData := `{"id":"chatcmpl-test","created":1,"model":"gpt-5.4","choices":[],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}`
	var responseID string
	var created int64
	var fingerprint string
	var model string
	usage := &dto.Usage{}
	containUsage := false

	err := handleLastResponse(
		lastStreamData,
		&responseID,
		&created,
		&fingerprint,
		&model,
		&usage,
		&containUsage,
	)

	require.NoError(t, err)
	require.True(t, containUsage)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 120, usage.TotalTokens)
}

func TestNormalizeAndValidateOpenAIUsageAcceptsAliasOnlyUsage(t *testing.T) {
	usage := &dto.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120}

	require.True(t, normalizeAndValidateOpenAIUsage(usage))
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 120, usage.TotalTokens)
}

func TestFillMissingOpenAIChatUsageUsesTotalBeforeLocalEstimate(t *testing.T) {
	usage := &dto.Usage{TotalTokens: 30}

	modified := fillMissingOpenAIChatUsage(usage, 10, 99)

	require.True(t, modified)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
}

func TestFillMissingOpenAIChatUsageUsesLocalCompletionEstimateLast(t *testing.T) {
	usage := &dto.Usage{PromptTokens: 10}

	modified := fillMissingOpenAIChatUsage(usage, 99, 20)

	require.True(t, modified)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
}

func TestOpenaiHandlerNormalizesUsageAndPreservesUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	responseBody := []byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":788,"output_tokens":589,"total_tokens":1377,"vendor_metric":42}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.4",
		},
	}

	usage, newAPIError := OpenaiHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 788, usage.PromptTokens)
	require.Equal(t, 589, usage.CompletionTokens)
	require.Equal(t, 1377, usage.TotalTokens)
	require.EqualValues(t, 589, gjson.GetBytes(recorder.Body.Bytes(), "usage.completion_tokens").Int())
	require.EqualValues(t, 42, gjson.GetBytes(recorder.Body.Bytes(), "usage.vendor_metric").Int())
}

func TestOpenaiHandlerWithUsageDoesNotDoubleCountAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	responseBody := []byte(`{"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30,"input_tokens":10,"output_tokens":20,"prompt_tokens_details":{"image_tokens":3,"text_tokens":7},"input_tokens_details":{"image_tokens":3,"text_tokens":7}}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
	}

	usage, newAPIError := OpenaiHandlerWithUsage(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
	require.Equal(t, 3, usage.PromptTokensDetails.ImageTokens)
	require.Equal(t, 7, usage.PromptTokensDetails.TextTokens)
}
