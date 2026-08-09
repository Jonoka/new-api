package constant

type ContextKey string

const (
	ContextKeyTokenCountMeta  ContextKey = "token_count_meta"
	ContextKeyPromptTokens    ContextKey = "prompt_tokens"
	ContextKeyEstimatedTokens ContextKey = "estimated_tokens"

	ContextKeyOriginalModel    ContextKey = "original_model"
	ContextKeyRequestStartTime ContextKey = "request_start_time"
	// ContextKeyRelayInfo 保存当前逻辑请求的 RelayInfo，供重试与可观测性流程共享。
	ContextKeyRelayInfo ContextKey = "relay_info"
	// ContextKeyChannelMetricState 保存当前请求的渠道指标生命周期状态。
	ContextKeyChannelMetricState ContextKey = "channel_metric_state"

	/* token related keys */
	ContextKeyTokenUnlimited         ContextKey = "token_unlimited_quota"
	ContextKeyTokenKey               ContextKey = "token_key"
	ContextKeyTokenId                ContextKey = "token_id"
	ContextKeyTokenGroup             ContextKey = "token_group"
	ContextKeyTokenSpecificChannelId ContextKey = "specific_channel_id"
	ContextKeyTokenModelLimitEnabled ContextKey = "token_model_limit_enabled"
	ContextKeyTokenModelLimit        ContextKey = "token_model_limit"
	ContextKeyTokenCrossGroupRetry   ContextKey = "token_cross_group_retry"
	ContextKeyTokenGroupRatioLimits  ContextKey = "token_group_ratio_limits"

	/* channel related keys */
	ContextKeyChannelId                     ContextKey = "channel_id"
	ContextKeyChannelName                   ContextKey = "channel_name"
	ContextKeyChannelCreateTime             ContextKey = "channel_create_time"
	ContextKeyChannelBaseUrl                ContextKey = "base_url"
	ContextKeyChannelType                   ContextKey = "channel_type"
	ContextKeyChannelSetting                ContextKey = "channel_setting"
	ContextKeyChannelOtherSetting           ContextKey = "channel_other_setting"
	ContextKeyChannelParamOverride          ContextKey = "param_override"
	ContextKeyChannelHeaderOverride         ContextKey = "header_override"
	ContextKeyChannelOrganization           ContextKey = "channel_organization"
	ContextKeyChannelAutoBan                ContextKey = "auto_ban"
	ContextKeyChannelModelMapping           ContextKey = "model_mapping"
	ContextKeyChannelStatusCodeMapping      ContextKey = "status_code_mapping"
	ContextKeyChannelIsMultiKey             ContextKey = "channel_is_multi_key"
	ContextKeyChannelMultiKeyIndex          ContextKey = "channel_multi_key_index"
	ContextKeyChannelPreferredMultiKeyIndex ContextKey = "channel_preferred_multi_key_index"
	ContextKeyChannelKey                    ContextKey = "channel_key"
	ContextKeySelectedChannel               ContextKey = "selected_channel"
	ContextKeySelectedChannelGroup          ContextKey = "selected_channel_group"

	ContextKeyAutoGroup           ContextKey = "auto_group"
	ContextKeyAutoGroupIndex      ContextKey = "auto_group_index"
	ContextKeyAutoGroupRetryIndex ContextKey = "auto_group_retry_index"

	/* user related keys */
	ContextKeyUserId      ContextKey = "id"
	ContextKeyUserSetting ContextKey = "user_setting"
	ContextKeyUserQuota   ContextKey = "user_quota"
	ContextKeyUserStatus  ContextKey = "user_status"
	ContextKeyUserEmail   ContextKey = "user_email"
	ContextKeyUserGroup   ContextKey = "user_group"
	ContextKeyUsingGroup  ContextKey = "group"
	ContextKeyUserName    ContextKey = "username"

	ContextKeyLocalCountTokens ContextKey = "local_count_tokens"

	ContextKeySystemPromptOverride ContextKey = "system_prompt_override"

	// ContextKeyFileSourcesToCleanup stores file sources that need cleanup when request ends
	ContextKeyFileSourcesToCleanup ContextKey = "file_sources_to_cleanup"

	// ContextKeyAdminRejectReason stores an admin-only reject/block reason extracted from upstream responses.
	// It is not returned to end users, but can be persisted into consume/error logs for debugging.
	ContextKeyAdminRejectReason ContextKey = "admin_reject_reason"

	// ContextKeyLanguage stores the user's language preference for i18n
	ContextKeyLanguage ContextKey = "language"
	ContextKeyIsStream ContextKey = "is_stream"

	// ContextKeyResponsesPreOutputRetry marks a native Responses capacity
	// failure that occurred before any client-visible output was committed.
	ContextKeyResponsesPreOutputRetry ContextKey = "responses_pre_output_retry"
)
