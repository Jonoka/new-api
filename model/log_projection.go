package model

import (
	"encoding/json"
	"math"

	"github.com/QuantumNous/new-api/common"
)

type userLogOtherValueKind int

const (
	userLogOtherBoolean userLogOtherValueKind = iota
	userLogOtherNumber
	userLogOtherString
	userLogOtherStringOrNumber
	userLogOtherStringArray
)

// userLogOtherAllowlist is the complete metadata contract for self, token and
// user-export log reads. Unknown fields and values with an unexpected shape are
// omitted so newly added diagnostic metadata cannot become public by default.
var userLogOtherAllowlist = map[string]userLogOtherValueKind{
	"actual_quota":                  userLogOtherNumber,
	"audio":                         userLogOtherBoolean,
	"audio_completion_ratio":        userLogOtherNumber,
	"audio_input":                   userLogOtherNumber,
	"audio_input_price":             userLogOtherNumber,
	"audio_input_seperate_price":    userLogOtherBoolean,
	"audio_input_token_count":       userLogOtherNumber,
	"audio_output":                  userLogOtherNumber,
	"audio_ratio":                   userLogOtherNumber,
	"billing_mode":                  userLogOtherString,
	"billing_preference":            userLogOtherString,
	"billing_quality":               userLogOtherString,
	"billing_resolution":            userLogOtherString,
	"billing_source":                userLogOtherString,
	"billing_variant_price_status":  userLogOtherString,
	"cache_creation_ratio":          userLogOtherNumber,
	"cache_creation_ratio_1h":       userLogOtherNumber,
	"cache_creation_ratio_5m":       userLogOtherNumber,
	"cache_creation_tokens":         userLogOtherNumber,
	"cache_creation_tokens_1h":      userLogOtherNumber,
	"cache_creation_tokens_5m":      userLogOtherNumber,
	"cache_ratio":                   userLogOtherNumber,
	"cache_tokens":                  userLogOtherNumber,
	"claude":                        userLogOtherBoolean,
	"compatibility_requested_model": userLogOtherString,
	"compatibility_routed_model":    userLogOtherString,
	"completion_ratio":              userLogOtherNumber,
	"expr_b64":                      userLogOtherString,
	"fee_quota":                     userLogOtherNumber,
	"file_search":                   userLogOtherBoolean,
	"file_search_call_count":        userLogOtherNumber,
	"file_search_price":             userLogOtherNumber,
	"frt":                           userLogOtherNumber,
	"group":                         userLogOtherString,
	"group_ratio":                   userLogOtherNumber,
	"image":                         userLogOtherBoolean,
	"image_generation_call":         userLogOtherBoolean,
	"image_generation_call_price":   userLogOtherNumber,
	"image_output":                  userLogOtherNumber,
	"image_ratio":                   userLogOtherNumber,
	"is_model_mapped":               userLogOtherBoolean,
	"is_system_prompt_overwritten":  userLogOtherBoolean,
	"is_task":                       userLogOtherBoolean,
	"matched_tier":                  userLogOtherString,
	"model_price":                   userLogOtherNumber,
	"model_price_unit":              userLogOtherString,
	"model_ratio":                   userLogOtherNumber,
	"pre_consumed_quota":            userLogOtherNumber,
	"reason":                        userLogOtherString,
	"reasoning_effort":              userLogOtherString,
	"request_conversion":            userLogOtherStringArray,
	"request_path":                  userLogOtherString,
	"seconds":                       userLogOtherNumber,
	"subscription_consumed":         userLogOtherNumber,
	"subscription_id":               userLogOtherStringOrNumber,
	"subscription_plan_id":          userLogOtherStringOrNumber,
	"subscription_plan_title":       userLogOtherString,
	"subscription_post_delta":       userLogOtherNumber,
	"subscription_pre_consumed":     userLogOtherNumber,
	"subscription_remain":           userLogOtherNumber,
	"subscription_total":            userLogOtherNumber,
	"subscription_used":             userLogOtherNumber,
	"task_id":                       userLogOtherString,
	"text_input":                    userLogOtherNumber,
	"text_output":                   userLogOtherNumber,
	"upstream_model_name":           userLogOtherString,
	"use_time_ms":                   userLogOtherNumber,
	"user_group_ratio":              userLogOtherNumber,
	"violation_fee":                 userLogOtherBoolean,
	"violation_fee_code":            userLogOtherString,
	"violation_fee_marker":          userLogOtherString,
	"wallet_quota_deducted":         userLogOtherNumber,
	"web_search":                    userLogOtherBoolean,
	"web_search_call_count":         userLogOtherNumber,
	"web_search_price":              userLogOtherNumber,
	"ws":                            userLogOtherBoolean,
}

func userLogOtherValueMatches(raw json.RawMessage, kind userLogOtherValueKind) bool {
	jsonType := common.GetJsonType(raw)
	switch kind {
	case userLogOtherBoolean:
		return jsonType == "boolean"
	case userLogOtherNumber:
		if jsonType != "number" {
			return false
		}
		var value float64
		return common.Unmarshal(raw, &value) == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
	case userLogOtherString:
		return jsonType == "string"
	case userLogOtherStringOrNumber:
		return jsonType == "string" || userLogOtherValueMatches(raw, userLogOtherNumber)
	case userLogOtherStringArray:
		if jsonType != "array" {
			return false
		}
		var values []json.RawMessage
		if common.Unmarshal(raw, &values) != nil {
			return false
		}
		for _, value := range values {
			if common.GetJsonType(value) != "string" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func projectUserLogOther(value string) string {
	if value == "" {
		return ""
	}

	var source map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(value, &source); err != nil || source == nil {
		return "{}"
	}

	projected := make(map[string]json.RawMessage)
	for key, kind := range userLogOtherAllowlist {
		raw, ok := source[key]
		if !ok || !userLogOtherValueMatches(raw, kind) {
			continue
		}
		projected[key] = raw
	}
	encoded, err := common.Marshal(projected)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func projectUserLog(log *Log, displayId int) *Log {
	if log == nil {
		return nil
	}
	return &Log{
		Id:               displayId,
		CreatedAt:        log.CreatedAt,
		Type:             log.Type,
		Content:          log.Content,
		TokenName:        log.TokenName,
		ModelName:        log.ModelName,
		Quota:            log.Quota,
		PromptTokens:     log.PromptTokens,
		CompletionTokens: log.CompletionTokens,
		UseTime:          log.UseTime,
		IsStream:         log.IsStream,
		Group:            log.Group,
		GroupName:        log.GroupName,
		RequestId:        log.RequestId,
		Other:            projectUserLogOther(log.Other),
	}
}

func formatUserLogs(logs []*Log, startIdx int) {
	for i, log := range logs {
		logs[i] = projectUserLog(log, startIdx+i+1)
	}
	hydrateLogGroupNames(logs)
}
