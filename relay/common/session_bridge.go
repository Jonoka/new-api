package common

import (
	"crypto/sha256"
	"fmt"
	"strings"

	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/google/uuid"
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
	rawSessionID, exists := raw["session_id"]
	if !exists {
		return nil
	}
	sessionID := normalizeOpenAIBridgeSessionID(info, rawSessionID)
	if sessionID == "" {
		return nil
	}
	out := map[string]interface{}{"session_id": sessionID}
	if conversationID := normalizeOpenAIBridgeSessionID(info, raw["conversation_id"]); conversationID != "" {
		out["conversation_id"] = conversationID
	}
	return out
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
	// x-client-request-id 只标识单次请求，不能参与跨轮会话桥接。
	for _, key := range []string{
		"x-claude-code-session-id",
		"x-codex-session-id",
		"conversation_id",
		"x-session-id",
	} {
		raw, exists := requestHeaders[key]
		if !exists || raw == nil {
			continue
		}
		if value := strings.TrimSpace(fmt.Sprintf("%v", raw)); value != "" {
			return value
		}
	}
	return ""
}

func normalizeOpenAIBridgeSessionID(info *RelayInfo, value interface{}) string {
	if value == nil {
		return ""
	}
	raw := strings.TrimSpace(fmt.Sprintf("%v", value))
	if raw == "" || strings.EqualFold(raw, "<nil>") {
		return ""
	}
	return generateStableSessionUUID(isolateOpenAISessionSeed(info, raw))
}

func NormalizeOpenAIBridgeSessionIDForCache(info *RelayInfo, value string) string {
	return normalizeOpenAIBridgeSessionID(info, value)
}

func isolateOpenAISessionSeed(info *RelayInfo, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	scope := 0
	if info != nil {
		switch {
		case info.ChannelMeta != nil && info.ChannelId > 0:
			scope = info.ChannelId
		case info.ChannelMeta != nil && info.ChannelMultiKeyIndex > 0:
			scope = info.ChannelMultiKeyIndex
		}
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("scope:%d:%s", scope, raw)))
	return fmt.Sprintf("%x", sum[:16])
}

func generateStableSessionUUID(seed string) string {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(seed))
	bytes := hash[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	parsed, err := uuid.FromBytes(bytes)
	if err != nil {
		return ""
	}
	return parsed.String()
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
