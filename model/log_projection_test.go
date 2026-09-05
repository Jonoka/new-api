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
	assert.Equal(t, "{}", projectUserLogOther("not-json"))
	assert.Equal(t, "{}", projectUserLogOther("null"))

	projected := projectUserLogOther(`{
		"request_path":{"nested":"secret"},
		"model_ratio":{"nested":"secret"},
		"is_task":"true",
		"subscription_id":{"nested":"secret"},
		"request_conversion":["OpenAI",{"nested":"secret"}],
		"task_id":"task-public"
	}`)
	parsed, err := common.StrToMap(projected)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"task_id": "task-public"}, parsed)
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
