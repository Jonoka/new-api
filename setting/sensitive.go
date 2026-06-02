package setting

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var CheckSensitiveEnabled = true
var CheckSensitiveOnPromptEnabled = true

//var CheckSensitiveOnCompletionEnabled = true

// StopOnSensitiveEnabled 如果检测到敏感词，是否立刻停止生成，否则替换敏感词
var StopOnSensitiveEnabled = true

// StreamCacheQueueLength 流模式缓存队列长度，0表示无缓存
var StreamCacheQueueLength = 0

// SensitiveWords 敏感词
// var SensitiveWords []string
var SensitiveWords = []string{
	"test_sensitive",
}

const (
	SensitiveRuleActionMask  = "mask"
	SensitiveRuleActionBlock = "block"

	SensitiveRuleScopeRequest  = "request"
	SensitiveRuleScopeResponse = "response"
	SensitiveRuleScopeBoth     = "both"

	DefaultSensitiveMaskReplacement = "[REDACTED]"
)

type SensitiveRule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Action      string   `json:"action"`
	Scope       string   `json:"scope,omitempty"`
	Replacement string   `json:"replacement,omitempty"`
	Keywords    []string `json:"keywords"`
	GroupRefs   []string `json:"group_refs,omitempty"`
}

type SensitiveRuleConfig struct {
	Rules []SensitiveRule `json:"rules"`
}

var SensitiveRules []SensitiveRule
var SensitiveRulesConfigured bool
var SensitiveRuleChannelIds []int

func SensitiveWordsToString() string {
	return strings.Join(SensitiveWords, "\n")
}

func SensitiveWordsFromString(s string) {
	SensitiveWords = []string{}
	sw := strings.Split(s, "\n")
	for _, w := range sw {
		w = strings.TrimSpace(w)
		if w != "" {
			SensitiveWords = append(SensitiveWords, w)
		}
	}
}

func ShouldCheckPromptSensitive() bool {
	return CheckSensitiveEnabled && CheckSensitiveOnPromptEnabled
}

func SensitiveRulesToJSONString() string {
	bytes, err := common.Marshal(SensitiveRuleConfig{Rules: NormalizeSensitiveRules(SensitiveRules)})
	if err != nil {
		return `{"rules":[]}`
	}
	return string(bytes)
}

func ParseSensitiveRulesJSONString(s string) ([]SensitiveRule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var config SensitiveRuleConfig
	if err := common.UnmarshalJsonStr(s, &config); err != nil {
		return nil, err
	}
	return NormalizeSensitiveRules(config.Rules), nil
}

func CheckSensitiveRulesJSONString(s string) error {
	_, err := ParseSensitiveRulesJSONString(s)
	return err
}

func UpdateSensitiveRulesByJSONString(s string) error {
	rules, err := ParseSensitiveRulesJSONString(s)
	if err != nil {
		return err
	}
	SensitiveRules = rules
	SensitiveRulesConfigured = true
	return nil
}

func SensitiveRuleChannelIdsToJSONString() string {
	bytes, err := common.Marshal(NormalizeSensitiveRuleChannelIds(SensitiveRuleChannelIds))
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func ParseSensitiveRuleChannelIdsJSONString(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var channelIds []int
	if err := common.UnmarshalJsonStr(s, &channelIds); err != nil {
		return nil, err
	}
	return NormalizeSensitiveRuleChannelIds(channelIds), nil
}

func CheckSensitiveRuleChannelIdsJSONString(s string) error {
	_, err := ParseSensitiveRuleChannelIdsJSONString(s)
	return err
}

func UpdateSensitiveRuleChannelIdsByJSONString(s string) error {
	channelIds, err := ParseSensitiveRuleChannelIdsJSONString(s)
	if err != nil {
		return err
	}
	SensitiveRuleChannelIds = channelIds
	return nil
}

func NormalizeSensitiveRuleChannelIds(channelIds []int) []int {
	if len(channelIds) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(channelIds))
	result := make([]int, 0, len(channelIds))
	for _, channelId := range channelIds {
		if channelId <= 0 {
			continue
		}
		if _, ok := seen[channelId]; ok {
			continue
		}
		seen[channelId] = struct{}{}
		result = append(result, channelId)
	}
	sort.Ints(result)
	return result
}

func ShouldApplySensitiveRulesToChannel(channelId int) bool {
	if channelId <= 0 {
		return false
	}
	for _, configuredId := range NormalizeSensitiveRuleChannelIds(SensitiveRuleChannelIds) {
		if configuredId == channelId {
			return true
		}
	}
	return false
}

func NormalizeSensitiveRules(rules []SensitiveRule) []SensitiveRule {
	normalized := make([]SensitiveRule, 0, len(rules))
	for _, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Action = strings.TrimSpace(strings.ToLower(rule.Action))
		rule.Scope = strings.TrimSpace(strings.ToLower(rule.Scope))
		rule.Replacement = strings.TrimSpace(rule.Replacement)
		if rule.Action != SensitiveRuleActionMask && rule.Action != SensitiveRuleActionBlock {
			rule.Action = SensitiveRuleActionBlock
		}
		if rule.Scope != SensitiveRuleScopeRequest && rule.Scope != SensitiveRuleScopeResponse && rule.Scope != SensitiveRuleScopeBoth {
			rule.Scope = SensitiveRuleScopeRequest
		}
		if rule.Action == SensitiveRuleActionMask && rule.Replacement == "" {
			rule.Replacement = DefaultSensitiveMaskReplacement
		}
		rule.Keywords = normalizeSensitiveKeywords(rule.Keywords)
		rule.GroupRefs = normalizeSensitiveGroupRefs(rule.GroupRefs)
		if len(rule.Keywords) == 0 && len(rule.GroupRefs) == 0 {
			continue
		}
		fallbackName := ""
		if len(rule.Keywords) > 0 {
			fallbackName = rule.Keywords[0]
		} else {
			fallbackName = rule.GroupRefs[0]
		}
		if rule.ID == "" {
			rule.ID = strings.ToLower(fallbackName)
		}
		if rule.Name == "" {
			rule.Name = fallbackName
		}
		normalized = append(normalized, rule)
	}
	return normalized
}

func GetEffectiveSensitiveRules() []SensitiveRule {
	rules := NormalizeSensitiveRules(SensitiveRules)
	if len(rules) > 0 {
		return rules
	}
	if SensitiveRulesConfigured {
		return nil
	}
	keywords := normalizeSensitiveKeywords(SensitiveWords)
	if len(keywords) == 0 {
		return nil
	}
	return []SensitiveRule{
		{
			ID:       "legacy-sensitive-words",
			Name:     "Legacy sensitive words",
			Enabled:  true,
			Action:   SensitiveRuleActionBlock,
			Scope:    SensitiveRuleScopeRequest,
			Keywords: keywords,
		},
	}
}

func GetEffectiveSensitiveRulesByScope(scope string) []SensitiveRule {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope != SensitiveRuleScopeRequest && scope != SensitiveRuleScopeResponse {
		return nil
	}
	rules := GetEffectiveSensitiveRules()
	if len(rules) == 0 {
		return nil
	}
	result := make([]SensitiveRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Scope == scope || rule.Scope == SensitiveRuleScopeBoth {
			result = append(result, rule)
		}
	}
	return result
}

func normalizeSensitiveKeywords(keywords []string) []string {
	result := make([]string, 0, len(keywords))
	seen := make(map[string]struct{}, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		key := strings.ToLower(keyword)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, keyword)
	}
	return result
}

func normalizeSensitiveGroupRefs(groupRefs []string) []string {
	result := make([]string, 0, len(groupRefs))
	seen := make(map[string]struct{}, len(groupRefs))
	for _, groupRef := range groupRefs {
		groupRef = strings.TrimSpace(groupRef)
		if groupRef == "" {
			continue
		}
		key := strings.ToLower(groupRef)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, groupRef)
	}
	return result
}

//func ShouldCheckCompletionSensitive() bool {
//	return CheckSensitiveEnabled && CheckSensitiveOnCompletionEnabled
//}
