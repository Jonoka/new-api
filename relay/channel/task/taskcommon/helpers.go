package taskcommon

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// UnmarshalMetadata converts a map[string]any metadata to a typed struct via JSON round-trip.
// This replaces the repeated pattern: json.Marshal(metadata) → json.Unmarshal(bytes, &target).
func UnmarshalMetadata(metadata map[string]any, target any) error {
	if metadata == nil {
		return nil
	}
	// Prevent metadata from overriding model fields to avoid billing bypass.
	delete(metadata, "model")
	metaBytes, err := common.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata failed: %w", err)
	}
	if err := common.Unmarshal(metaBytes, target); err != nil {
		return fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	return nil
}

// DefaultString returns val if non-empty, otherwise fallback.
func DefaultString(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// DefaultInt returns val if non-zero, otherwise fallback.
func DefaultInt(val, fallback int) int {
	if val == 0 {
		return fallback
	}
	return val
}

// NormalizeVideoResolution 将常见分辨率参数归一化为计费档位名称。
// 返回空字符串表示无法识别，调用方应保留原请求或不应用分辨率价格。
func NormalizeVideoResolution(resolution string) string {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "480", "480p", "sd", "854x480":
		return "480p"
	case "720", "720p", "hd", "1280x720":
		return "720p"
	case "1080", "1080p", "fhd", "full-hd", "full_hd", "1920x1080":
		return "1080p"
	case "2k", "2048x1080", "2560x1440":
		return "2k"
	case "4k", "3840x2160", "4096x2160":
		return "4k"
	default:
		return ""
	}
}

// EncodeLocalTaskID encodes an upstream operation name to a URL-safe base64 string.
// Used by Gemini/Vertex to store upstream names as task IDs.
func EncodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

// DecodeLocalTaskID decodes a base64-encoded upstream operation name.
func DecodeLocalTaskID(id string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BuildProxyURL constructs the video proxy URL using the public task ID.
// e.g., "https://your-server.com/v1/videos/task_xxxx/content"
func BuildProxyURL(taskID string) string {
	return fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)
}

// Status-to-progress mapping constants for polling updates.
const (
	ProgressSubmitted  = "10%"
	ProgressQueued     = "20%"
	ProgressInProgress = "30%"
	ProgressComplete   = "100%"
)

// ---------------------------------------------------------------------------
// BaseBilling — embeddable no-op implementations for TaskAdaptor billing methods.
// Adaptors that do not need custom billing can embed this struct directly.
// ---------------------------------------------------------------------------

type BaseBilling struct{}

// EstimateBilling returns nil (no extra ratios; use base model price).
func (BaseBilling) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	return nil
}

// AdjustBillingOnSubmit returns nil (no submit-time adjustment).
func (BaseBilling) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}

// AdjustBillingOnComplete returns 0 (keep pre-charged amount).
func (BaseBilling) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

// AdjustPerSecondBillingOnComplete 使用上游返回的实际视频时长重算按秒固定价任务。
// 分辨率等其它倍率沿用提交时已冻结的计费快照，避免轮询期间配置变化影响结算。
func AdjustPerSecondBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if task == nil || taskResult == nil || taskResult.DurationSeconds <= 0 {
		return 0
	}
	billing := task.PrivateData.BillingContext
	if billing == nil || billing.ModelPriceUnit != types.ModelPriceUnitSecond {
		return 0
	}

	quotaValue := billing.ModelPrice * common.QuotaPerUnit * billing.GroupRatio * float64(taskResult.DurationSeconds)
	for key, ratio := range billing.OtherRatios {
		if key == "seconds" || !(ratio > 0) || math.IsInf(ratio, 1) || ratio == 1 {
			continue
		}
		quotaValue *= ratio
	}

	if billing.OtherRatios == nil {
		billing.OtherRatios = make(map[string]float64)
	}
	billing.OtherRatios["seconds"] = float64(taskResult.DurationSeconds)
	quota, _ := common.QuotaFromFloatChecked(quotaValue)
	return quota
}
