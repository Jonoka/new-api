package dto

type ChannelSettings struct {
	ForceFormat            bool   `json:"force_format,omitempty"`
	ThinkingToContent      bool   `json:"thinking_to_content,omitempty"`
	Proxy                  string `json:"proxy"`
	PassThroughBodyEnabled bool   `json:"pass_through_body_enabled,omitempty"`
	SystemPrompt           string `json:"system_prompt,omitempty"`
	SystemPromptOverride   bool   `json:"system_prompt_override,omitempty"`
}

type VertexKeyType string

const (
	VertexKeyTypeJSON   VertexKeyType = "json"
	VertexKeyTypeAPIKey VertexKeyType = "api_key"
)

type AwsKeyType string

const (
	AwsKeyTypeAKSK   AwsKeyType = "ak_sk" // 默认
	AwsKeyTypeApiKey AwsKeyType = "api_key"
)

type ChannelOtherSettings struct {
	AzureResponsesVersion                 string        `json:"azure_responses_version,omitempty"`
	VertexKeyType                         VertexKeyType `json:"vertex_key_type,omitempty"` // "json" or "api_key"
	OpenRouterEnterprise                  *bool         `json:"openrouter_enterprise,omitempty"`
	ClaudeBetaQuery                       bool          `json:"claude_beta_query,omitempty"`                         // Claude 渠道是否强制追加 ?beta=true
	ClaudeCodeFingerprintEnabled          bool   `json:"claude_code_fingerprint_enabled,omitempty"`           // Claude 渠道是否使用 Claude Code 指纹
	ClaudeCodeTransportFingerprintEnabled bool   `json:"claude_code_transport_fingerprint_enabled,omitempty"` // Claude 渠道是否使用 Claude Code Transport 指纹
	ClaudeCodeVersion                     string `json:"claude_code_version,omitempty"`                      // 自定义 Claude Code 版本号（用于 User-Agent），留空使用默认值
	AllowServiceTier                      bool          `json:"allow_service_tier,omitempty"`                        // 是否允许 service_tier 透传（默认过滤以避免额外计费）
	AllowInferenceGeo                     bool          `json:"allow_inference_geo,omitempty"`                       // 是否允许 inference_geo 透传（仅 Claude，默认过滤以满足数据驻留合规
	AllowSpeed                            bool          `json:"allow_speed,omitempty"`                               // 是否允许 speed 透传（仅 Claude，默认过滤以避免意外切换推理速度模式）
	AllowSafetyIdentifier                 bool          `json:"allow_safety_identifier,omitempty"`                   // 是否允许 safety_identifier 透传（默认过滤以保护用户隐私）
	DisableStore                          bool          `json:"disable_store,omitempty"`                             // 是否禁用 store 透传（默认允许透传，禁用后可能导致 Codex 无法使用）
	AllowIncludeObfuscation               bool          `json:"allow_include_obfuscation,omitempty"`                 // 是否允许 stream_options.include_obfuscation 透传（默认过滤以避免关闭流混淆保护）
	AwsKeyType                            AwsKeyType    `json:"aws_key_type,omitempty"`
	UpstreamModelUpdateCheckEnabled       bool          `json:"upstream_model_update_check_enabled,omitempty"`        // 是否检测上游模型更新
	UpstreamModelUpdateAutoSyncEnabled    bool          `json:"upstream_model_update_auto_sync_enabled,omitempty"`    // 是否自动同步上游模型更新
	UpstreamModelUpdateLastCheckTime      int64         `json:"upstream_model_update_last_check_time,omitempty"`      // 上次检测时间
	UpstreamModelUpdateLastDetectedModels []string      `json:"upstream_model_update_last_detected_models,omitempty"` // 上次检测到的可加入模型
	UpstreamModelUpdateLastRemovedModels  []string      `json:"upstream_model_update_last_removed_models,omitempty"`  // 上次检测到的可删除模型
	UpstreamModelUpdateIgnoredModels      []string      `json:"upstream_model_update_ignored_models,omitempty"`       // 手动忽略的模型
	MonitorEnabled                        *bool         `json:"monitor_enabled,omitempty"`                            // 是否启用单渠道自动监控，nil 表示继承全局
	MonitorTestIntervalMinutes            *float64      `json:"monitor_test_interval_minutes,omitempty"`              // 单渠道测试间隔，nil 表示继承全局调度
	MonitorResponseTimeThresholdSeconds   *float64      `json:"monitor_response_time_threshold_seconds,omitempty"`    // 单渠道最长响应时间，nil 表示继承全局
	MonitorAutoDisableEnabled             *bool         `json:"monitor_auto_disable_enabled,omitempty"`               // 单渠道失败自动禁用开关，nil 表示继承全局
	MonitorAutoEnableEnabled              *bool         `json:"monitor_auto_enable_enabled,omitempty"`                // 单渠道成功自动启用开关，nil 表示继承全局
	MonitorDisableThreshold               *int          `json:"monitor_disable_threshold,omitempty"`                  // 连续失败阈值，nil 表示继承全局
	MonitorEnableThreshold                *int          `json:"monitor_enable_threshold,omitempty"`                   // 连续成功阈值，nil 表示继承全局
	MonitorLastTestTime                   int64         `json:"monitor_last_test_time,omitempty"`                     // 上次自动监控测试时间
	MonitorConsecutiveFailures            int           `json:"monitor_consecutive_failures,omitempty"`               // 连续失败次数
	MonitorConsecutiveSuccesses           int           `json:"monitor_consecutive_successes,omitempty"`              // 连续成功次数
}

func (s *ChannelOtherSettings) IsOpenRouterEnterprise() bool {
	if s == nil || s.OpenRouterEnterprise == nil {
		return false
	}
	return *s.OpenRouterEnterprise
}
