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

	DefaultSensitiveMaskReplacement = "[REDACTED]"
)

type SensitiveRule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Action      string   `json:"action"`
	Replacement string   `json:"replacement,omitempty"`
	Keywords    []string `json:"keywords"`
}

type SensitiveRuleConfig struct {
	Rules []SensitiveRule `json:"rules"`
}

var SensitiveRules []SensitiveRule
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
		rule.Replacement = strings.TrimSpace(rule.Replacement)
		if rule.Action != SensitiveRuleActionMask && rule.Action != SensitiveRuleActionBlock {
			rule.Action = SensitiveRuleActionBlock
		}
		if rule.Action == SensitiveRuleActionMask && rule.Replacement == "" {
			rule.Replacement = DefaultSensitiveMaskReplacement
		}
		rule.Keywords = normalizeSensitiveKeywords(rule.Keywords)
		if len(rule.Keywords) == 0 {
			continue
		}
		if rule.ID == "" {
			rule.ID = strings.ToLower(rule.Keywords[0])
		}
		if rule.Name == "" {
			rule.Name = rule.Keywords[0]
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
			Keywords: keywords,
		},
	}
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

//func ShouldCheckCompletionSensitive() bool {
//	return CheckSensitiveEnabled && CheckSensitiveOnCompletionEnabled
//}
