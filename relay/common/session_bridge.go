package common

import (
	"fmt"
	"strings"

	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/tidwall/gjson"
)

func shouldBridgeOpenAISessionHeader(info *RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	if info.ApiType != rootconstant.APITypeOpenAI && info.ApiType != rootconstant.APITypeCodex {
		return false
	}
	switch info.GetFinalRequestRelayFormat() {
	case types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		return true
	default:
		return false
	}
}

func buildOpenAISessionBridgeOverride(info *RelayInfo, jsonData []byte) map[string]interface{} {
	if !shouldBridgeOpenAISessionHeader(info) {
		return nil
	}

	overrideCtx := BuildParamOverrideContext(info)
	headerOverride := ensureMapKeyInContext(overrideCtx, paramOverrideContextHeaderOverride)
	if existing, ok := getHeaderValueFromContext(overrideCtx, "session_id"); ok && strings.TrimSpace(existing) != "" {
		headerOverride["session_id"] = existing
	} else if !hasPromptCacheKeyInJSON(jsonData) {
		if seed := resolveOpenAISessionSeedFromRequestHeaders(overrideCtx); seed != "" {
			headerOverride["session_id"] = seed
		}
	}

	if len(jsonData) == 0 {
		jsonData = []byte(`{}`)
	}
	if _, err := ApplyParamOverride(jsonData, map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "sync_fields",
				"from": "header:session_id",
				"to":   "json:prompt_cache_key",
			},
		},
	}, overrideCtx); err != nil {
		return nil
	}

	raw, ok := overrideCtx[paramOverrideContextHeaderOverride].(map[string]interface{})
	if !ok {
		return nil
	}
	sessionID := strings.TrimSpace(fmt.Sprintf("%v", raw["session_id"]))
	if sessionID == "" {
		return nil
	}
	return map[string]interface{}{
		"session_id": sessionID,
	}
}

func hasPromptCacheKeyInJSON(jsonData []byte) bool {
	if len(jsonData) == 0 {
		return false
	}
	value := gjson.GetBytes(jsonData, "prompt_cache_key")
	if !value.Exists() || value.Type == gjson.Null {
		return false
	}
	return strings.TrimSpace(value.String()) != ""
}

func resolveOpenAISessionSeedFromRequestHeaders(context map[string]interface{}) string {
	requestHeaders, _ := context[paramOverrideContextRequestHeaders].(map[string]interface{})
	if len(requestHeaders) == 0 {
		return ""
	}
	for _, key := range []string{
		"x-claude-code-session-id",
		"x-codex-session-id",
		"conversation_id",
		"x-session-id",
		"x-client-request-id",
	} {
		if value := strings.TrimSpace(fmt.Sprintf("%v", requestHeaders[key])); value != "" {
			return value
		}
	}
	return ""
}

func MergeOpenAISessionBridgeOverride(info *RelayInfo, jsonData []byte) {
	if info == nil {
		return
	}
	bridge := buildOpenAISessionBridgeOverride(info, jsonData)
	if len(bridge) == 0 {
		return
	}

	base := GetEffectiveHeaderOverride(info)
	merged := make(map[string]interface{}, len(base)+len(bridge))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range bridge {
		if strings.TrimSpace(fmt.Sprintf("%v", value)) == "" {
			continue
		}
		merged[key] = value
	}
	info.RuntimeHeadersOverride = sanitizeHeaderOverrideMap(merged)
	info.UseRuntimeHeadersOverride = true
}
