package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useLogProjectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&Group{}, &GroupAlias{}, &Log{}))

	previousDB, previousLogDB := DB, LOG_DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousSQLite, previousMySQL, previousPostgreSQL := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	previousCommonGroupCol, previousCommonKeyCol := commonGroupCol, commonKeyCol
	previousCommonTrueVal, previousCommonFalseVal := commonTrueVal, commonFalseVal
	previousLogGroupCol, previousLogKeyCol := logGroupCol, logKeyCol
	DB, LOG_DB = database, database
	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousSQLite
		common.UsingMySQL = previousMySQL
		common.UsingPostgreSQL = previousPostgreSQL
		commonGroupCol, commonKeyCol = previousCommonGroupCol, previousCommonKeyCol
		commonTrueVal, commonFalseVal = previousCommonTrueVal, previousCommonFalseVal
		logGroupCol, logKeyCol = previousLogGroupCol, previousLogKeyCol
		require.NoError(t, sqlDB.Close())
	})
	return database
}

func TestFormatUserLogsUsesStrictProjectionWithoutMutatingSource(t *testing.T) {
	database := useLogProjectionTestDB(t)
	require.NoError(t, database.Create(&Group{Code: "paid", Name: "Paid", Ratio: 1, Status: GroupStatusActive}).Error)

	const other = `{
		"request_path":"/v1/responses",
		"model_ratio":2.5,
		"cache_tokens":17,
		"cache_write_tokens":4,
		"input_tokens_total":31,
		"usage_semantic":"anthropic",
		"billing_source":"subscription",
		"is_task":true,
		"subscription_id":9007199254740993,
		"request_conversion":["OpenAI Responses","Claude Messages"],
		"admin_info":{"secret":"admin-secret","nested":{"secret":"nested-admin-secret"}},
		"root_info":{"secret":"root-secret"},
		"audit_info":{"secret":"audit-secret"},
		"channel_name":"channel-secret",
		"reject_reason":"rejection-secret",
		"stream_status":{"end_error":"debug-secret"},
		"timing_diagnostics":{"total_ms":123},
		"po":["set secret-field"],
		"unknown":{"secret":"unknown-secret"}
	}`
	source := &Log{
		Id:                91,
		UserId:            7,
		CreatedAt:         1234,
		Type:              LogTypeConsume,
		Content:           "usage detail",
		Username:          "alice",
		TokenName:         "user-token",
		ModelName:         "gpt-test",
		Quota:             600,
		PromptTokens:      10,
		CompletionTokens:  20,
		UseTime:           3,
		IsStream:          true,
		ChannelId:         42,
		ChannelName:       "resolved-channel-secret",
		TokenId:           19,
		Group:             "paid",
		Ip:                "203.0.113.8",
		RequestId:         "request-public",
		UpstreamRequestId: "upstream-secret",
		Other:             other,
	}
	logs := []*Log{source}

	formatUserLogs(logs, 8)

	require.NotSame(t, source, logs[0])
	assert.Equal(t, 91, source.Id)
	assert.Equal(t, 42, source.ChannelId)
	assert.Equal(t, "203.0.113.8", source.Ip)
	assert.Equal(t, other, source.Other)

	projected := logs[0]
	assert.Equal(t, 9, projected.Id)
	assert.Zero(t, projected.UserId)
	assert.Empty(t, projected.Username)
	assert.Zero(t, projected.ChannelId)
	assert.Empty(t, projected.ChannelName)
	assert.Zero(t, projected.TokenId)
	assert.Empty(t, projected.Ip)
	assert.Empty(t, projected.UpstreamRequestId)
	assert.Equal(t, "request-public", projected.RequestId)
	assert.Equal(t, "user-token", projected.TokenName)
	assert.Equal(t, "gpt-test", projected.ModelName)
	assert.Equal(t, 600, projected.Quota)
	assert.Equal(t, 10, projected.PromptTokens)
	assert.Equal(t, 20, projected.CompletionTokens)
	assert.Equal(t, "Paid", projected.GroupName)
	assert.Contains(t, projected.Other, `"subscription_id":9007199254740993`)

	parsed, err := common.StrToMap(projected.Other)
	require.NoError(t, err)
	assert.Equal(t, "/v1/responses", parsed["request_path"])
	assert.Equal(t, 2.5, parsed["model_ratio"])
	assert.Equal(t, float64(17), parsed["cache_tokens"])
	assert.Equal(t, float64(4), parsed["cache_write_tokens"])
	assert.Equal(t, float64(31), parsed["input_tokens_total"])
	assert.Equal(t, "anthropic", parsed["usage_semantic"])
	assert.Equal(t, "subscription", parsed["billing_source"])
	assert.Equal(t, true, parsed["is_task"])
	assert.Equal(t, []interface{}{"OpenAI Responses", "Claude Messages"}, parsed["request_conversion"])
	for _, key := range []string{
		"admin_info", "root_info", "audit_info", "channel_name", "reject_reason",
		"stream_status", "timing_diagnostics", "po", "unknown",
	} {
		assert.NotContains(t, parsed, key)
	}
	encoded, err := common.Marshal(projected)
	require.NoError(t, err)
	for _, secret := range []string{
		"alice", "resolved-channel-secret", "203.0.113.8", "upstream-secret",
		"admin-secret", "nested-admin-secret", "root-secret", "audit-secret",
		"channel-secret", "rejection-secret", "debug-secret", "unknown-secret",
	} {
		assert.NotContains(t, string(encoded), secret)
	}
}

func TestProjectUserLogOtherRejectsMalformedAndWrongShapes(t *testing.T) {
	assert.Equal(t, "{}", projectUserLogOther("not-json", LogTypeConsume))
	assert.Equal(t, "{}", projectUserLogOther("null", LogTypeConsume))
	assert.Equal(t, "{}", projectUserLogOther(`{"request_conversion":["OpenAI",null]}`, LogTypeConsume))
	assert.Equal(t, "{}", projectUserLogOther(`{"request_conversion":[null]}`, LogTypeConsume))

	projected := projectUserLogOther(`{
		"request_path":{"nested":"secret"},
		"model_ratio":{"nested":"secret"},
		"is_task":"true",
		"subscription_id":{"nested":"secret"},
		"request_conversion":["OpenAI",{"nested":"secret"}],
		"usage_semantic":"openai",
		"cache_write_tokens_source":"synthetic-secret-source",
		"n":{"nested":"secret"},
		"task_id":"task-public"
	}`, LogTypeConsume)
	parsed, err := common.StrToMap(projected)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"task_id": "task-public"}, parsed)
}

func TestProjectUserLogOtherKeepsWriterDefinedBillingFacts(t *testing.T) {
	tests := []struct {
		name     string
		other    string
		expected map[string]interface{}
	}{
		{
			name:  "anthropic cache usage",
			other: `{"usage_semantic":"anthropic","cache_tokens":10,"cache_write_tokens":4,"cache_write_tokens_source":"upstream"}`,
			expected: map[string]interface{}{
				"usage_semantic":            "anthropic",
				"cache_tokens":              float64(10),
				"cache_write_tokens":        float64(4),
				"cache_write_tokens_source": "upstream",
			},
		},
		{
			name:  "normalized openai input with inferred cache write",
			other: `{
				"input_tokens_total":20,
				"cache_write_tokens":4,
				"cache_write_tokens_source":"inferred_missing_field",
				"inferred_cache_write_tokens":4,
				"inferred_cache_write_billable":true
			}`,
			expected: map[string]interface{}{
				"input_tokens_total":            float64(20),
				"cache_write_tokens":            float64(4),
				"cache_write_tokens_source":     "inferred_missing_field",
				"inferred_cache_write_tokens":   float64(4),
				"inferred_cache_write_billable": true,
			},
		},
		{
			name:  "task and image numeric ratios",
			other: `{"is_task":true,"seconds":8,"n":2,"size":1.666667,"resolution":1.75,"dynamic_ratio":{"secret":"not-public"}}`,
			expected: map[string]interface{}{
				"is_task":    true,
				"seconds":    float64(8),
				"n":          float64(2),
				"size":       1.666667,
				"resolution": 1.75,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projected := projectUserLogOther(tt.other, LogTypeConsume)
			parsed, err := common.StrToMap(projected)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, parsed)
		})
	}
}

func TestProjectUserLogOtherAcceptsOnlyWriterDefinedEnumValues(t *testing.T) {
	for _, source := range []string{
		"upstream",
		"explicit_zero",
		"inferred_missing_field",
		"inferred_untrusted_explicit_zero",
	} {
		projected := projectUserLogOther(`{"cache_write_tokens_source":"` + source + `"}`, LogTypeConsume)
		parsed, err := common.StrToMap(projected)
		require.NoError(t, err)
		assert.Equal(t, source, parsed["cache_write_tokens_source"])
	}

	projected := projectUserLogOther(`{
		"usage_semantic":"anthropic",
		"cache_write_tokens_source":"synthetic-secret-source",
		"reason":"synthetic-reason-secret"
	}`, LogTypeConsume)
	parsed, err := common.StrToMap(projected)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"usage_semantic": "anthropic"}, parsed)
}

func TestProjectUserLogContentUsesExplicitPerTypeContract(t *testing.T) {
	tests := []struct {
		logType int
		want    string
	}{
		{LogTypeTopup, "controlled content"},
		{LogTypeConsume, "controlled content"},
		{LogTypeManage, "controlled content"},
		{LogTypeSystem, "controlled content"},
		{LogTypeError, userLogErrorContent},
		{LogTypeRefund, userLogRefundContent},
		{LogTypeUnknown, ""},
		{99, ""},
	}

	for _, tt := range tests {
		source := &Log{Type: tt.logType, Content: "controlled content"}
		projected := projectUserLog(source, 1)
		assert.Equal(t, tt.want, projected.Content)
		assert.Equal(t, "controlled content", source.Content)
	}
}

func requireLogType(t *testing.T, logs []*Log, logType int) *Log {
	t.Helper()
	for _, log := range logs {
		if log.Type == logType {
			return log
		}
	}
	t.Fatalf("log type %d not found", logType)
	return nil
}

func TestSensitiveErrorAndRefundDiagnosticsRemainAdminOnly(t *testing.T) {
	database := useLogProjectionTestDB(t)
	require.NoError(t, database.Exec("CREATE TABLE channels (id integer primary key, name text)").Error)
	require.NoError(t, database.Exec("INSERT INTO channels (id, name) VALUES (?, ?)", 42, "private-channel").Error)

	now := time.Now().Unix()
	errorContent := "authorization failed: Bearer synthetic-error-secret"
	refundContent := "recalculation detail contains synthetic-refund-content-secret"
	errorOther := `{
		"request_path":"/v1/responses",
		"error_type":"synthetic-error-type-secret",
		"error_code":"synthetic-error-code-secret",
		"status_code":401,
		"channel_name":"synthetic-channel-secret",
		"message":"synthetic-message-secret",
		"fail_reason":"synthetic-fail-reason-secret",
		"detail":"synthetic-detail-secret",
		"diagnostic":"synthetic-diagnostic-secret",
		"authorization":"Bearer synthetic-authorization-secret",
		"use_time_ms":12
	}`
	refundOther := `{
		"task_id":"task-public",
		"reason":"authorization failed: Bearer synthetic-refund-reason-secret; channel=private-upstream",
		"error":"synthetic-refund-error-secret",
		"error_message":"synthetic-refund-message-secret",
		"api_key":"synthetic-api-key-secret",
		"actual_quota":40,
		"pre_consumed_quota":50,
		"n":2
	}`
	records := []*Log{
		{
			UserId: 7, CreatedAt: now, Type: LogTypeError, Content: errorContent,
			TokenName: "token-a", ModelName: "gpt-test", ChannelId: 42, TokenId: 19,
			Ip: "203.0.113.10", UpstreamRequestId: "synthetic-upstream-request-secret", Other: errorOther,
		},
		{
			UserId: 7, CreatedAt: now + 1, Type: LogTypeRefund, Content: refundContent,
			TokenName: "token-a", ModelName: "video-test", Quota: 10, ChannelId: 42, TokenId: 19,
			Ip: "203.0.113.11", UpstreamRequestId: "synthetic-refund-upstream-secret", Other: refundOther,
		},
	}
	require.NoError(t, database.Create(&records).Error)

	userLogs, total, err := GetUserLogs(7, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, userLogs, 2)
	tokenLogs, err := GetLogByTokenId(19)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 2)

	for _, publicLogs := range [][]*Log{userLogs, tokenLogs} {
		publicError := requireLogType(t, publicLogs, LogTypeError)
		assert.Equal(t, userLogErrorContent, publicError.Content)
		errorMetadata, err := common.StrToMap(publicError.Other)
		require.NoError(t, err)
		assert.Equal(t, userLogErrorReason, errorMetadata["reason"])
		assert.Equal(t, "/v1/responses", errorMetadata["request_path"])
		assert.Equal(t, float64(12), errorMetadata["use_time_ms"])

		publicRefund := requireLogType(t, publicLogs, LogTypeRefund)
		assert.Equal(t, userLogRefundContent, publicRefund.Content)
		refundMetadata, err := common.StrToMap(publicRefund.Other)
		require.NoError(t, err)
		assert.Equal(t, userLogRefundReason, refundMetadata["reason"])
		assert.Equal(t, "task-public", refundMetadata["task_id"])
		assert.Equal(t, float64(40), refundMetadata["actual_quota"])
		assert.Equal(t, float64(50), refundMetadata["pre_consumed_quota"])
		assert.Equal(t, float64(2), refundMetadata["n"])

		exported, err := common.Marshal(publicLogs)
		require.NoError(t, err)
		for _, secret := range []string{
			"synthetic-error-secret", "synthetic-refund-content-secret",
			"synthetic-error-type-secret", "synthetic-error-code-secret",
			"synthetic-channel-secret", "synthetic-message-secret",
			"synthetic-fail-reason-secret", "synthetic-refund-reason-secret",
			"synthetic-detail-secret", "synthetic-diagnostic-secret",
			"synthetic-authorization-secret", "synthetic-api-key-secret",
			"synthetic-refund-error-secret", "synthetic-refund-message-secret",
			"synthetic-upstream-request-secret", "synthetic-refund-upstream-secret",
		} {
			assert.NotContains(t, string(exported), secret)
		}
	}

	adminLogs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, adminLogs, 2)
	adminError := requireLogType(t, adminLogs, LogTypeError)
	assert.Equal(t, errorContent, adminError.Content)
	assert.Equal(t, errorOther, adminError.Other)
	adminRefund := requireLogType(t, adminLogs, LogTypeRefund)
	assert.Equal(t, refundContent, adminRefund.Content)
	assert.Equal(t, refundOther, adminRefund.Other)

	assert.Equal(t, errorContent, records[0].Content)
	assert.Equal(t, errorOther, records[0].Other)
	assert.Equal(t, refundContent, records[1].Content)
	assert.Equal(t, refundOther, records[1].Other)
}

func TestUserAndTokenReadsProjectWhileAdminReadRemainsComplete(t *testing.T) {
	database := useLogProjectionTestDB(t)
	require.NoError(t, database.Exec("CREATE TABLE channels (id integer primary key, name text)").Error)
	require.NoError(t, database.Exec("INSERT INTO channels (id, name) VALUES (?, ?)", 42, "private-channel").Error)
	require.NoError(t, database.Create(&Log{
		UserId:            7,
		CreatedAt:         time.Now().Unix(),
		Type:              LogTypeConsume,
		Username:          "alice",
		TokenName:         "token-a",
		ModelName:         "gpt-test",
		Quota:             50,
		PromptTokens:      3,
		CompletionTokens:  4,
		ChannelId:         42,
		TokenId:           19,
		Ip:                "203.0.113.9",
		RequestId:         "request-public",
		UpstreamRequestId: "upstream-secret",
		Other:             `{"model_ratio":1.5,"admin_info":{"secret":"admin-secret"},"unknown":"unknown-secret"}`,
	}).Error)

	userLogs, total, err := GetUserLogs(7, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, userLogs, 1)
	assert.Zero(t, userLogs[0].ChannelId)
	assert.Empty(t, userLogs[0].Ip)
	assert.Empty(t, userLogs[0].UpstreamRequestId)
	assert.NotContains(t, userLogs[0].Other, "admin-secret")
	assert.NotContains(t, userLogs[0].Other, "unknown-secret")

	tokenLogs, err := GetLogByTokenId(19)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 1)
	assert.Zero(t, tokenLogs[0].ChannelId)
	assert.Empty(t, tokenLogs[0].Ip)
	assert.Empty(t, tokenLogs[0].UpstreamRequestId)
	assert.NotContains(t, tokenLogs[0].Other, "admin-secret")

	adminLogs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, adminLogs, 1)
	assert.Equal(t, 42, adminLogs[0].ChannelId)
	assert.Equal(t, "private-channel", adminLogs[0].ChannelName)
	assert.Equal(t, "203.0.113.9", adminLogs[0].Ip)
	assert.Equal(t, "upstream-secret", adminLogs[0].UpstreamRequestId)
	assert.Contains(t, adminLogs[0].Other, "admin-secret")
	assert.Contains(t, adminLogs[0].Other, "unknown-secret")
}

func TestSumUsedQuotaKeepsHistoricalQuotaAndRecentRatesWithFilters(t *testing.T) {
	database := useLogProjectionTestDB(t)
	group := &Group{Code: "paid", Name: "Paid", Ratio: 1, Status: GroupStatusActive}
	require.NoError(t, database.Create(group).Error)
	require.NoError(t, database.Create(&GroupAlias{Alias: "legacy-paid", GroupId: group.Id}).Error)

	now := time.Now().Unix()
	matching := []*Log{
		{UserId: 7, Username: "alice", TokenName: "token-a", ModelName: "gpt-test", ChannelId: 9, Group: "paid", CreatedAt: now - 120, Type: LogTypeConsume, Quota: 500, PromptTokens: 10, CompletionTokens: 20},
		{UserId: 7, Username: "alice", TokenName: "token-a", ModelName: "gpt-test", ChannelId: 9, Group: "legacy-paid", CreatedAt: now - 5, Type: LogTypeConsume, Quota: 100, PromptTokens: 30, CompletionTokens: 40},
	}
	require.NoError(t, database.Create(&matching).Error)
	require.NoError(t, database.Create(&Log{
		UserId: 7, Username: "alice", TokenName: "old-token", ModelName: "gpt-test", ChannelId: 9, Group: "paid",
		CreatedAt: now - 120, Type: LogTypeConsume, Quota: 250, PromptTokens: 5, CompletionTokens: 6,
	}).Error)
	decoys := []*Log{
		{Username: "bob", TokenName: "token-a", ModelName: "gpt-test", ChannelId: 9, Group: "paid", CreatedAt: now - 5, Type: LogTypeConsume, Quota: 1000, PromptTokens: 100, CompletionTokens: 100},
		{Username: "alice", TokenName: "token-b", ModelName: "gpt-test", ChannelId: 9, Group: "paid", CreatedAt: now - 5, Type: LogTypeConsume, Quota: 1000, PromptTokens: 100, CompletionTokens: 100},
		{Username: "alice", TokenName: "token-a", ModelName: "other-model", ChannelId: 9, Group: "paid", CreatedAt: now - 5, Type: LogTypeConsume, Quota: 1000, PromptTokens: 100, CompletionTokens: 100},
		{Username: "alice", TokenName: "token-a", ModelName: "gpt-test", ChannelId: 10, Group: "paid", CreatedAt: now - 5, Type: LogTypeConsume, Quota: 1000, PromptTokens: 100, CompletionTokens: 100},
		{Username: "alice", TokenName: "token-a", ModelName: "gpt-test", ChannelId: 9, Group: "other", CreatedAt: now - 5, Type: LogTypeConsume, Quota: 1000, PromptTokens: 100, CompletionTokens: 100},
	}
	require.NoError(t, database.Create(&decoys).Error)

	stat, err := SumUsedQuota(LogTypeUnknown, now-1000, now+1, "gpt-test", "alice", "token-a", 9, "Paid")
	require.NoError(t, err)
	assert.Equal(t, 600, stat.Quota)
	assert.Equal(t, 1, stat.Rpm)
	assert.Equal(t, 70, stat.Tpm)

	oldOnlyStat, err := SumUsedQuota(LogTypeUnknown, now-1000, now+1, "gpt-test", "alice", "old-token", 9, "Paid")
	require.NoError(t, err)
	assert.Equal(t, 250, oldOnlyStat.Quota)
	assert.Zero(t, oldOnlyStat.Rpm)
	assert.Zero(t, oldOnlyStat.Tpm)
}
