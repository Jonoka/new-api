package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSensitiveRulesJSONStringNormalizesRules(t *testing.T) {
	raw := `{
		"rules": [
			{
				"id": " rule-1 ",
				"name": " Mask rule ",
				"enabled": true,
				"action": "mask",
				"scope": " response ",
				"replacement": " [MASK] ",
				"keywords": [" Secret ", "secret", "", "中文"]
			},
			{
				"id": "",
				"name": "",
				"enabled": true,
				"action": "unknown",
				"scope": "unknown",
				"keywords": [" block-me "]
			}
		]
	}`

	rules, err := ParseSensitiveRulesJSONString(raw)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	assert.Equal(t, "rule-1", rules[0].ID)
	assert.Equal(t, "Mask rule", rules[0].Name)
	assert.Equal(t, SensitiveRuleActionMask, rules[0].Action)
	assert.Equal(t, SensitiveRuleScopeResponse, rules[0].Scope)
	assert.Equal(t, "[MASK]", rules[0].Replacement)
	assert.Equal(t, []string{"Secret", "中文"}, rules[0].Keywords)

	assert.Equal(t, "block-me", rules[1].ID)
	assert.Equal(t, "block-me", rules[1].Name)
	assert.Equal(t, SensitiveRuleActionBlock, rules[1].Action)
	assert.Equal(t, SensitiveRuleScopeRequest, rules[1].Scope)
	assert.Equal(t, []string{"block-me"}, rules[1].Keywords)
}

func TestGetEffectiveSensitiveRulesFallsBackToLegacyWords(t *testing.T) {
	oldRules := SensitiveRules
	oldConfigured := SensitiveRulesConfigured
	oldWords := SensitiveWords
	defer func() {
		SensitiveRules = oldRules
		SensitiveRulesConfigured = oldConfigured
		SensitiveWords = oldWords
	}()

	SensitiveRules = nil
	SensitiveRulesConfigured = false
	SensitiveWords = []string{"legacy", "词"}

	rules := GetEffectiveSensitiveRules()
	require.Len(t, rules, 1)
	assert.Equal(t, "legacy-sensitive-words", rules[0].ID)
	assert.Equal(t, SensitiveRuleActionBlock, rules[0].Action)
	assert.Equal(t, SensitiveRuleScopeRequest, rules[0].Scope)
	assert.Equal(t, []string{"legacy", "词"}, rules[0].Keywords)
}

func TestGetEffectiveSensitiveRulesDoesNotFallbackAfterRulesConfigured(t *testing.T) {
	oldRules := SensitiveRules
	oldConfigured := SensitiveRulesConfigured
	oldWords := SensitiveWords
	defer func() {
		SensitiveRules = oldRules
		SensitiveRulesConfigured = oldConfigured
		SensitiveWords = oldWords
	}()

	SensitiveWords = []string{"legacy"}

	err := UpdateSensitiveRulesByJSONString(`{"rules":[]}`)
	require.NoError(t, err)

	assert.True(t, SensitiveRulesConfigured)
	assert.Empty(t, GetEffectiveSensitiveRules())
}

func TestGetEffectiveSensitiveRulesByScope(t *testing.T) {
	oldRules := SensitiveRules
	oldConfigured := SensitiveRulesConfigured
	oldWords := SensitiveWords
	defer func() {
		SensitiveRules = oldRules
		SensitiveRulesConfigured = oldConfigured
		SensitiveWords = oldWords
	}()

	SensitiveWords = nil
	SensitiveRulesConfigured = false
	SensitiveRules = []SensitiveRule{
		{
			ID:       "request",
			Name:     "Request",
			Enabled:  true,
			Action:   SensitiveRuleActionBlock,
			Scope:    SensitiveRuleScopeRequest,
			Keywords: []string{"request-only"},
		},
		{
			ID:       "response",
			Name:     "Response",
			Enabled:  true,
			Action:   SensitiveRuleActionBlock,
			Scope:    SensitiveRuleScopeResponse,
			Keywords: []string{"response-only"},
		},
		{
			ID:       "both",
			Name:     "Both",
			Enabled:  true,
			Action:   SensitiveRuleActionBlock,
			Scope:    SensitiveRuleScopeBoth,
			Keywords: []string{"both"},
		},
	}

	requestRules := GetEffectiveSensitiveRulesByScope(SensitiveRuleScopeRequest)
	require.Len(t, requestRules, 2)
	assert.Equal(t, []string{"request", "both"}, []string{requestRules[0].ID, requestRules[1].ID})

	responseRules := GetEffectiveSensitiveRulesByScope(SensitiveRuleScopeResponse)
	require.Len(t, responseRules, 2)
	assert.Equal(t, []string{"response", "both"}, []string{responseRules[0].ID, responseRules[1].ID})
}
