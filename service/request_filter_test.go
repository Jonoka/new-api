package service

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRequestFilterRules(t *testing.T, rules []setting.SensitiveRule) {
	t.Helper()
	oldEnabled := setting.CheckSensitiveEnabled
	oldPromptEnabled := setting.CheckSensitiveOnPromptEnabled
	oldRules := setting.SensitiveRules
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.CheckSensitiveOnPromptEnabled = true
	setting.SensitiveRules = rules
	setting.SensitiveRuleChannelIds = nil
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.CheckSensitiveOnPromptEnabled = oldPromptEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})
}

func newJSONFilterContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c
}

func storedBody(t *testing.T, c *gin.Context) string {
	t.Helper()
	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	body, err := storage.Bytes()
	require.NoError(t, err)
	_, err = storage.Seek(0, io.SeekStart)
	require.NoError(t, err)
	return string(body)
}

func setFilterChannelIds(ids ...int) {
	setting.SensitiveRuleChannelIds = ids
}

func TestApplySensitiveFilterToRequestBodyBlocksBeforeMasking(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "mask",
			Name:        "Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Replacement: "[MASK]",
			Keywords:    []string{"secret"},
		},
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(1)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.True(t, result.Blocked)
	assert.False(t, result.Mutated)
	require.Len(t, result.Matches, 1)
	assert.Equal(t, setting.SensitiveRuleActionBlock, result.Matches[0].Action)
	assert.Contains(t, storedBody(t, c), "secret")
}

func TestApplySensitiveFilterToRequestBodyMasksPromptFields(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "mask",
			Name:        "Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Replacement: "[MASK]",
			Keywords:    []string{"Secret", "词"},
		},
	})
	setFilterChannelIds(1)

	tests := []struct {
		name        string
		format      types.RelayFormat
		body        string
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:   "openai chat",
			format: types.RelayFormatOpenAI,
			body: `{
				"model":"gpt-test",
				"messages":[
					{"role":"user","content":"Secret text"},
					{"role":"user","content":[{"type":"text","text":"包含词"}]}
				],
				"prompt":"Secret prompt",
				"metadata":{"note":"do-not-touch"}
			}`,
			wantPresent: []string{"[MASK] text", "包含[MASK]", "[MASK] prompt", "do-not-touch"},
			wantAbsent:  []string{"Secret text", "包含词", "Secret prompt"},
		},
		{
			name:   "responses",
			format: types.RelayFormatOpenAIResponses,
			body: `{
				"model":"gpt-test",
				"instructions":"Secret instructions",
				"input":[{"role":"user","content":[{"type":"input_text","text":"Secret input"}]}],
				"metadata":{"note":"do-not-touch"}
			}`,
			wantPresent: []string{"[MASK] instructions", "[MASK] input", "do-not-touch"},
			wantAbsent:  []string{"Secret instructions", "Secret input"},
		},
		{
			name:   "claude",
			format: types.RelayFormatClaude,
			body: `{
				"model":"claude-test",
				"system":"Secret system",
				"messages":[{"role":"user","content":[{"type":"text","text":"Secret message"}]}]
			}`,
			wantPresent: []string{"[MASK] system", "[MASK] message"},
			wantAbsent:  []string{"Secret system", "Secret message"},
		},
		{
			name:   "gemini",
			format: types.RelayFormatGemini,
			body: `{
				"systemInstruction":{"parts":[{"text":"Secret system"}]},
				"contents":[{"role":"user","parts":[{"text":"Secret message"}]}]
			}`,
			wantPresent: []string{"[MASK] system", "[MASK] message"},
			wantAbsent:  []string{"Secret system", "Secret message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newJSONFilterContext(t, tt.body)
			common.SetContextKey(c, constant.ContextKeyChannelId, 1)

			result, err := ApplySensitiveFilterToRequestBody(c, tt.format)
			require.NoError(t, err)

			assert.False(t, result.Blocked)
			assert.True(t, result.Mutated)
			body := storedBody(t, c)
			for _, want := range tt.wantPresent {
				assert.Contains(t, body, want)
			}
			for _, want := range tt.wantAbsent {
				assert.NotContains(t, body, want)
			}
		})
	}
}

func TestApplySensitiveFilterToRequestBodySkipsWhenNoChannelsConfigured(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds()

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 10)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.False(t, result.Mutated)
	assert.Contains(t, storedBody(t, c), "secret")
}

func TestApplySensitiveFilterToRequestBodySkipsWhenChannelNotSelected(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(10, 20)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 30)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.False(t, result.Mutated)
	assert.Contains(t, storedBody(t, c), "secret")
}

func TestApplySensitiveFilterToRequestBodyBlocksWhenChannelSelected(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(10, 20)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 20)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.True(t, result.Blocked)
	assert.False(t, result.Mutated)
}

func TestApplySensitiveFilterToRequestBodyMasksWhenChannelSelected(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "mask",
			Name:        "Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Replacement: "[MASK]",
			Keywords:    []string{"secret"},
		},
	})
	setFilterChannelIds(10, 20)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 10)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.True(t, result.Mutated)
	body := storedBody(t, c)
	assert.Contains(t, body, "[MASK]")
	assert.NotContains(t, body, "secret")
}
