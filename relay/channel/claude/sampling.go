package claude

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

// NormalizeClaudeSamplingParameters 清理新版 Claude 不再接受的采样参数。
func NormalizeClaudeSamplingParameters(request *dto.ClaudeRequest) {
	if request == nil || !isClaudeSamplingDeprecatedModel(request.Model) {
		return
	}

	request.Temperature = nil
	request.TopP = nil
	request.TopK = nil
}

func isClaudeSamplingDeprecatedModel(model string) bool {
	normalized := strings.TrimSpace(model)
	if normalized == "" {
		return false
	}

	return strings.HasPrefix(normalized, "claude-sonnet-4-6") ||
		strings.HasPrefix(normalized, "claude-opus-4-6") ||
		strings.HasPrefix(normalized, "claude-opus-4-7") ||
		strings.HasPrefix(normalized, "claude-opus-4-8") ||
		strings.HasPrefix(normalized, "claude-fable-5")
}
