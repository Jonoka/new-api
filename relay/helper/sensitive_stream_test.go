package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringDataSendsErrorEventAndStopsAfterSensitiveBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldEnabled := setting.CheckSensitiveEnabled
	oldRules := setting.SensitiveRules
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.SensitiveRules = []setting.SensitiveRule{
		{
			ID:       "response-block",
			Name:     "Response Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"secret"},
		},
	}
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRuleChannelIds = []int{1}
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	err := StringData(c, `{"choices":[{"delta":{"content":"secret"}}]}`)
	require.NoError(t, err)
	Done(c)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, "sensitive_words_detected")
	assert.True(t, c.GetBool("sensitive_response_stream_blocked"))
	assert.False(t, strings.Contains(body, "[DONE]"))
}

func TestStringDataBlocksSensitiveKeywordAcrossStreamChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldEnabled := setting.CheckSensitiveEnabled
	oldRules := setting.SensitiveRules
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.SensitiveRules = []setting.SensitiveRule{
		{
			ID:       "response-block",
			Name:     "Response Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"啦啦队"},
		},
	}
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRuleChannelIds = []int{1}
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"啦"}}]}`))
	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"啦"}}]}`))
	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"队"}}]}`))
	Done(c)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, "sensitive_words_detected")
	assert.True(t, c.GetBool("sensitive_response_stream_blocked"))
	assert.NotContains(t, body, `"content":"啦"`)
	assert.NotContains(t, body, `"content":"队"`)
	assert.False(t, strings.Contains(body, "[DONE]"))
}

func TestStringDataFlushesDelayedSafeStreamChunksOnDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldEnabled := setting.CheckSensitiveEnabled
	oldRules := setting.SensitiveRules
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.SensitiveRules = []setting.SensitiveRule{
		{
			ID:       "response-block",
			Name:     "Response Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"啦啦队"},
		},
	}
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRuleChannelIds = []int{1}
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"安全"}}]}`))
	Done(c)

	body := recorder.Body.String()
	assert.Contains(t, body, `"content":"安全"`)
	assert.Contains(t, body, "[DONE]")
	assert.False(t, c.GetBool("sensitive_response_stream_blocked"))
}
