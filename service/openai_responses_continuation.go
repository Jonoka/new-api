package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/cachex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIResponsesContinuationCacheNamespace = "new-api:openai_responses_continuation:v1"
	openAIResponsesContinuationTTL            = 30 * time.Minute
)

var (
	openAIResponsesContinuationCacheOnce sync.Once
	openAIResponsesContinuationCache     *cachex.HybridCache[string]
)

func getOpenAIResponsesContinuationCache() *cachex.HybridCache[string] {
	openAIResponsesContinuationCacheOnce.Do(func() {
		openAIResponsesContinuationCache = cachex.NewHybridCache[string](cachex.HybridCacheConfig[string]{
			Namespace: openAIResponsesContinuationCacheNamespace,
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.StringCodec{},
			Memory: func() *hot.HotCache[string, string] {
				return hot.NewHotCache[string, string](hot.LRU, 100_000).
					WithTTL(openAIResponsesContinuationTTL).
					WithJanitor().
					Build()
			},
		})
	})
	return openAIResponsesContinuationCache
}

func resolveOpenAIResponsesContinuationSessionKey(info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) string {
	if info == nil || req == nil {
		return ""
	}
	sessionID := resolveOpenAIResponsesContinuationSessionID(info, req)
	if sessionID == "" {
		return ""
	}
	return fmt.Sprintf("ch:%d:mk:%d:sess:%s", info.ChannelId, info.ChannelMultiKeyIndex, sessionID)
}

func resolveOpenAIResponsesContinuationSessionID(info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) string {
	if req != nil {
		if promptCacheKey := strings.TrimSpace(common.JsonRawMessageToString(req.PromptCacheKey)); promptCacheKey != "" {
			return relaycommon.NormalizeOpenAIBridgeSessionIDForCache(info, promptCacheKey)
		}
	}
	headers := relaycommon.GetEffectiveHeaderOverride(info)
	for _, key := range []string{"session_id", "conversation_id"} {
		if value := strings.TrimSpace(fmt.Sprintf("%v", headers[key])); value != "" {
			return relaycommon.NormalizeOpenAIBridgeSessionIDForCache(info, value)
		}
	}
	return ""
}

func GetOpenAIResponsesContinuationResponseID(info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) string {
	cacheKey := resolveOpenAIResponsesContinuationSessionKey(info, req)
	if cacheKey == "" {
		return ""
	}
	value, found, err := getOpenAIResponsesContinuationCache().Get(cacheKey)
	if err != nil || !found {
		return ""
	}
	return normalizeOpenAIResponsesPreviousResponseID(value)
}

func getOpenAIResponsesContinuationRequestFromInfo(info *relaycommon.RelayInfo) *dto.OpenAIResponsesRequest {
	if info == nil || info.Request == nil {
		return nil
	}
	req, _ := info.Request.(*dto.OpenAIResponsesRequest)
	return req
}

func GetOpenAIResponsesContinuationResponseIDFromInfo(info *relaycommon.RelayInfo) string {
	return GetOpenAIResponsesContinuationResponseID(info, getOpenAIResponsesContinuationRequestFromInfo(info))
}

func BindOpenAIResponsesContinuationResponseID(info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest, responseID string) {
	id := normalizeOpenAIResponsesPreviousResponseID(responseID)
	if id == "" {
		return
	}
	cacheKey := resolveOpenAIResponsesContinuationSessionKey(info, req)
	if cacheKey == "" {
		return
	}
	_ = getOpenAIResponsesContinuationCache().SetWithTTL(cacheKey, id, openAIResponsesContinuationTTL)
}

func BindOpenAIResponsesContinuationResponseIDFromInfo(info *relaycommon.RelayInfo, responseID string) {
	BindOpenAIResponsesContinuationResponseID(info, getOpenAIResponsesContinuationRequestFromInfo(info), responseID)
}

func DeleteOpenAIResponsesContinuationResponseID(info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) {
	cacheKey := resolveOpenAIResponsesContinuationSessionKey(info, req)
	if cacheKey == "" {
		return
	}
	_, _ = getOpenAIResponsesContinuationCache().DeleteMany([]string{cacheKey})
}

func DeleteOpenAIResponsesContinuationResponseIDFromInfo(info *relaycommon.RelayInfo) {
	DeleteOpenAIResponsesContinuationResponseID(info, getOpenAIResponsesContinuationRequestFromInfo(info))
}

func normalizeOpenAIResponsesPreviousResponseID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "resp_") {
		return value
	}
	return ""
}

func AttachOpenAIResponsesContinuation(info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) bool {
	if info == nil || req == nil {
		return false
	}
	if normalizeOpenAIResponsesPreviousResponseID(req.PreviousResponseID) != "" {
		req.PreviousResponseID = normalizeOpenAIResponsesPreviousResponseID(req.PreviousResponseID)
		return false
	}
	// 默认不自动注入 previous_response_id。
	// 大多数 OpenAI 兼容上游只支持 prompt_cache_key / session_id 亲和，
	// 并不支持 Responses continuation 续链语义。
	return false
}

func DropOpenAIResponsesPreviousResponseID(req *dto.OpenAIResponsesRequest) {
	if req == nil {
		return
	}
	req.PreviousResponseID = ""
}

func RemoveOpenAIResponsesPreviousResponseIDFromJSON(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	if !strings.Contains(string(data), "previous_response_id") {
		return data
	}
	updated := data
	for i := 0; i < 8; i++ {
		next, err := sjson.DeleteBytes(updated, "previous_response_id")
		if err != nil {
			return updated
		}
		updated = next
		if !strings.Contains(string(updated), "previous_response_id") {
			return updated
		}
	}
	return updated
}

func RemoveIncompleteOpenAIResponsesReasoningHistoryFromJSON(data []byte) ([]byte, bool) {
	if len(data) == 0 || !strings.Contains(string(data), `"reasoning"`) {
		return data, false
	}
	input := gjson.GetBytes(data, "input")
	if !input.IsArray() {
		return data, false
	}

	removed := false
	filtered := make([]any, 0)
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "reasoning" && strings.TrimSpace(item.Get("encrypted_content").String()) == "" {
			removed = true
			return true
		}

		var decoded any
		if err := common.Unmarshal([]byte(item.Raw), &decoded); err != nil {
			filtered = append(filtered, item.Value())
		} else {
			filtered = append(filtered, decoded)
		}
		return true
	})
	if !removed {
		return data, false
	}

	updated, err := sjson.SetBytes(data, "input", filtered)
	if err != nil {
		return data, false
	}
	updated = RemoveOpenAIResponsesPreviousResponseIDFromJSON(updated)
	return updated, true
}

func TrimOpenAIResponsesInputToLatestTurn(req *dto.OpenAIResponsesRequest) {
	if req == nil || len(req.Input) == 0 || common.GetJsonType(req.Input) != "array" {
		return
	}

	var items []map[string]any
	if err := common.Unmarshal(req.Input, &items); err != nil || len(items) == 0 {
		return
	}

	start := latestOpenAIResponsesInputTurnStart(items)
	if start <= 0 || start >= len(items) {
		return
	}

	trimmed := append([]map[string]any(nil), items[start:]...)
	inputRaw, err := common.Marshal(trimmed)
	if err != nil {
		return
	}
	req.Input = inputRaw
}

func latestOpenAIResponsesInputTurnStart(items []map[string]any) int {
	if len(items) == 0 {
		return 0
	}

	start := len(items) - 1
	lastType := strings.TrimSpace(common.Interface2String(items[start]["type"]))
	lastRole := strings.TrimSpace(common.Interface2String(items[start]["role"]))

	switch {
	case lastType == "function_call_output":
		for start > 0 && strings.TrimSpace(common.Interface2String(items[start-1]["type"])) == "function_call_output" {
			start--
		}
	case lastRole == "user":
		for start > 0 && strings.TrimSpace(common.Interface2String(items[start-1]["type"])) == "function_call_output" {
			start--
		}
	default:
		return start
	}

	return expandOpenAIResponsesInputToolCallStart(items, start)
}

func expandOpenAIResponsesInputToolCallStart(items []map[string]any, start int) int {
	if start < 0 || start >= len(items) {
		return start
	}

	needed := make(map[string]struct{})
	for i := start; i < len(items); i++ {
		if strings.TrimSpace(common.Interface2String(items[i]["type"])) != "function_call_output" {
			continue
		}
		callID := strings.TrimSpace(common.Interface2String(items[i]["call_id"]))
		if callID != "" {
			needed[callID] = struct{}{}
		}
	}
	if len(needed) == 0 {
		return start
	}

	expandedStart := start
	for i := start - 1; i >= 0 && len(needed) > 0; i-- {
		if strings.TrimSpace(common.Interface2String(items[i]["type"])) != "function_call" {
			continue
		}
		callID := strings.TrimSpace(common.Interface2String(items[i]["call_id"]))
		if _, ok := needed[callID]; !ok {
			continue
		}
		delete(needed, callID)
		expandedStart = i
	}
	return expandedStart
}

func IsOpenAIResponsesPreviousResponseRetryable(statusCode int, message string) bool {
	if statusCode != 400 && statusCode != 404 {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "previous_response_not_found") ||
		(strings.Contains(lower, "previous response") && strings.Contains(lower, "not found")) {
		return true
	}
	if !strings.Contains(lower, "previous_response_id") {
		return false
	}
	return strings.Contains(lower, "unsupported parameter") ||
		strings.Contains(lower, "not supported") ||
		((strings.Contains(lower, "only supported") || strings.Contains(lower, "supported only")) &&
			(strings.Contains(lower, "responses websocket") || strings.Contains(lower, "websocket v2"))) ||
		strings.Contains(lower, "invalid previous_response_id")
}
