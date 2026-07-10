package model_setting

import (
	"regexp"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	MissingCacheWriteFallbackModeObserve = "observe"
	MissingCacheWriteFallbackModeBill    = "bill"
)

type MissingCacheWriteFallbackPolicy struct {
	Enabled       bool     `json:"enabled"`
	Mode          string   `json:"mode"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids,omitempty"`
	ChannelTypes  []int    `json:"channel_types,omitempty"`
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

func (p MissingCacheWriteFallbackPolicy) IsEnabledFor(channelID int, channelType int, model string) bool {
	if !p.Enabled || (p.Mode != MissingCacheWriteFallbackModeObserve && p.Mode != MissingCacheWriteFallbackModeBill) {
		return false
	}
	channelEnabled := p.AllChannels ||
		(channelID > 0 && slices.Contains(p.ChannelIDs, channelID)) ||
		(channelType > 0 && slices.Contains(p.ChannelTypes, channelType))
	if !channelEnabled || model == "" {
		return false
	}
	for _, pattern := range p.ModelPatterns {
		matched, err := regexp.MatchString(pattern, model)
		if err == nil && matched {
			return true
		}
	}
	return false
}

type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids,omitempty"`
	ChannelTypes  []int    `json:"channel_types,omitempty"`
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}

	if channelID > 0 && len(p.ChannelIDs) > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && len(p.ChannelTypes) > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

type GlobalSettings struct {
	PassThroughRequestEnabled        bool                             `json:"pass_through_request_enabled"`
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`
	MissingCacheWriteFallback        MissingCacheWriteFallbackPolicy  `json:"missing_cache_write_fallback"`
}

// 默认配置
var defaultOpenaiSettings = GlobalSettings{
	PassThroughRequestEnabled: false,
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
	ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
	MissingCacheWriteFallback: MissingCacheWriteFallbackPolicy{
		Enabled: false,
		Mode:    MissingCacheWriteFallbackModeObserve,
	},
}

// 全局实例
var globalSettings = defaultOpenaiSettings

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("global", &globalSettings)
}

func GetGlobalSettings() *GlobalSettings {
	return &globalSettings
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking/-low/-high/-medium 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	for _, entry := range globalSettings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}
