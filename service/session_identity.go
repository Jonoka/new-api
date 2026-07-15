package service

import "strings"

// 仅会话级身份可以作为自动缓存键或渠道亲和键。
var stableOpenAISessionHeaderNames = []string{
	"x-claude-code-session-id",
	"x-codex-session-id",
	"conversation_id",
	"x-session-id",
	"session_id",
}

func resolveStableOpenAISessionHeader(getHeader func(string) string) string {
	if getHeader == nil {
		return ""
	}
	for _, name := range stableOpenAISessionHeaderNames {
		if value := strings.TrimSpace(getHeader(name)); value != "" {
			return value
		}
	}
	return ""
}

func resolveStableOpenAISessionHeaderFromMap(headers map[string]string) string {
	return resolveStableOpenAISessionHeader(func(target string) string {
		for name, value := range headers {
			if strings.EqualFold(name, target) {
				return value
			}
		}
		return ""
	})
}
