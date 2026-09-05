package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestAlphaSearchRequestFilterPreservesOpaqueNumbers(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{{
		ID: "alpha-mask", Name: "Mask", Enabled: true,
		Action: setting.SensitiveRuleActionMask, Replacement: "[MASK]", Keywords: []string{"secret"},
	}})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{"model":"alpha-model","input":"secret","opaque":{"large":9007199254740993,"zero":0,"null":null,"flag":false}}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAIAlphaSearch)
	require.NoError(t, err)
	require.True(t, result.Mutated)
	got := storedBody(t, c)
	require.Contains(t, got, `"input":"[MASK]"`)
	require.Contains(t, got, `"large":9007199254740993`)
	require.Contains(t, got, `"zero":0`)
	require.Contains(t, got, `"null":null`)
	require.Contains(t, got, `"flag":false`)
}
