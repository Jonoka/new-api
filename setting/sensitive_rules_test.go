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
				"replacement": " [MASK] ",
				"keywords": [" Secret ", "secret", "", "中文"]
			},
			{
				"id": "",
				"name": "",
				"enabled": true,
				"action": "unknown",
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
	assert.Equal(t, "[MASK]", rules[0].Replacement)
	assert.Equal(t, []string{"Secret", "中文"}, rules[0].Keywords)

	assert.Equal(t, "block-me", rules[1].ID)
	assert.Equal(t, "block-me", rules[1].Name)
	assert.Equal(t, SensitiveRuleActionBlock, rules[1].Action)
	assert.Equal(t, []string{"block-me"}, rules[1].Keywords)
}

func TestGetEffectiveSensitiveRulesFallsBackToLegacyWords(t *testing.T) {
	oldRules := SensitiveRules
	oldWords := SensitiveWords
	defer func() {
		SensitiveRules = oldRules
		SensitiveWords = oldWords
	}()

	SensitiveRules = nil
	SensitiveWords = []string{"legacy", "词"}

	rules := GetEffectiveSensitiveRules()
	require.Len(t, rules, 1)
	assert.Equal(t, "legacy-sensitive-words", rules[0].ID)
	assert.Equal(t, SensitiveRuleActionBlock, rules[0].Action)
	assert.Equal(t, []string{"legacy", "词"}, rules[0].Keywords)
}
