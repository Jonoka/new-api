package controller

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	finalGroupRelayModel = "fg-relay-model"
	finalGroupRelayExpr  = `tier("controller", 200)`
	finalGroupBaseQuota  = 100
)

type finalGroupRelayBalance struct {
	userQuota   int
	tokenRemain int
	tokenUsed   int
	err         string
}

type finalGroupRelayUpstream struct {
	server  *httptest.Server
	calls   atomic.Int32
	mu      sync.Mutex
	samples []finalGroupRelayBalance
}

func newFinalGroupRelayUpstream(db *gorm.DB, userID, tokenID, status int) *finalGroupRelayUpstream {
	upstream := &finalGroupRelayUpstream{}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstream.calls.Add(1)
		var user model.User
		var token model.Token
		userErr := db.Select("quota").First(&user, userID).Error
		tokenErr := db.Select("remain_quota", "used_quota").First(&token, tokenID).Error
		sample := finalGroupRelayBalance{
			userQuota: user.Quota, tokenRemain: token.RemainQuota, tokenUsed: token.UsedQuota,
		}
		if userErr != nil || tokenErr != nil {
			sample.err = fmt.Sprintf("user=%v token=%v", userErr, tokenErr)
		}
		upstream.mu.Lock()
		upstream.samples = append(upstream.samples, sample)
		upstream.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(`{"error":{"message":"retry fixture","type":"upstream_error","code":"retry_fixture"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl-final","object":"chat.completion","created":1,"model":"fg-relay-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":10,"total_tokens":20}}`))
	}))
	return upstream
}

func (u *finalGroupRelayUpstream) balanceSamples() []finalGroupRelayBalance {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]finalGroupRelayBalance(nil), u.samples...)
}

func setupFinalGroupRelayDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	oldMemoryCache, oldBatch, oldRedis := common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.RedisEnabled
	oldRetryTimes := common.RetryTimes
	oldAutoDisable := common.AutomaticDisableChannelEnabled
	oldLogConsume, oldDataExport := common.LogConsumeEnabled, common.DataExportEnabled
	oldQuotaPerUnit := common.QuotaPerUnit
	oldCountToken, oldErrorLog := constant.CountToken, constant.ErrorLogEnabled
	oldFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	oldRetryRanges := append([]operation_setting.StatusCodeRange(nil), operation_setting.AutomaticRetryStatusCodeRanges...)
	savedConfig := config.GlobalConfig.ExportAllConfigs()

	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = oldSQLite, oldMySQL, oldPostgres
		common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.RedisEnabled = oldMemoryCache, oldBatch, oldRedis
		common.RetryTimes = oldRetryTimes
		common.AutomaticDisableChannelEnabled = oldAutoDisable
		common.LogConsumeEnabled, common.DataExportEnabled = oldLogConsume, oldDataExport
		common.QuotaPerUnit = oldQuotaPerUnit
		constant.CountToken, constant.ErrorLogEnabled = oldCountToken, oldErrorLog
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFreePreConsume
		operation_setting.AutomaticRetryStatusCodeRanges = oldRetryRanges
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
	})

	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = true, false, false
	common.MemoryCacheEnabled = false
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	common.RetryTimes = 1
	common.AutomaticDisableChannelEnabled = false
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	common.QuotaPerUnit = 500_000
	constant.CountToken = false
	constant.ErrorLogEnabled = false
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 599}}
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":          `{"fg-relay-model":"tiered_expr"}`,
		"billing_setting.billing_expr":          `{"fg-relay-model":"tier(\"controller\", 200)"}`,
		"group_ratio_setting.group_ratio":       `{"fg-free-a":0,"fg-paid-a":2,"fg-paid-b":1,"fg-free-b":0,"fg-paid-c":1,"fg-high-c":2,"fg-same":1,"fg-paid-d":1,"fg-high-d":2}`,
		"group_ratio_setting.group_group_ratio": `{}`,
	}))

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "final-group-relay.db")+"?_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.TaskSubmission{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedFinalGroupRelayFunding(t *testing.T, db *gorm.DB, index, quota int) (*model.User, *model.Token) {
	t.Helper()
	user := &model.User{
		Username: fmt.Sprintf("fgr%02d", index), Password: "test-pass", Quota: quota,
		AffCode: fmt.Sprintf("fgr%02d", index),
	}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{
		UserId: user.Id, Key: fmt.Sprintf("fgr-key-%02d", index), Name: fmt.Sprintf("fgr-token-%02d", index),
		RemainQuota: quota,
	}
	require.NoError(t, db.Create(token).Error)
	return user, token
}

func seedFinalGroupRelayChannel(t *testing.T, db *gorm.DB, name, group, baseURL string, priority int64) *model.Channel {
	t.Helper()
	weight, autoBan, concurrencyLimit := uint(1), 0, 0
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI, Key: "fixture-key", Status: common.ChannelStatusEnabled,
		Name: name, Weight: &weight, ConcurrencyLimit: &concurrencyLimit, CreatedTime: time.Now().Unix(),
		BaseURL: &baseURL, Models: finalGroupRelayModel, Group: group, Priority: &priority, AutoBan: &autoBan,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: group, Model: finalGroupRelayModel, ChannelId: channel.Id, Enabled: true,
		Priority: &priority, Weight: weight,
	}).Error)
	return channel
}

func finalGroupRelayContext(t *testing.T, user *model.User, token *model.Token, first *model.Channel, firstGroup, tokenGroup, requestID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"model":"fg-relay-model","messages":[{"role":"user","content":"test"}],"max_tokens":8}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	common.SetContextKey(c, common.RequestIdKey, requestID)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, firstGroup)
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
	common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, tokenGroup)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, finalGroupRelayModel)
	common.SetContextKey(c, constant.ContextKeyChannelId, first.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, first.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, first.Type)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, first.GetBaseURL())
	common.SetContextKey(c, constant.ContextKeyChannelKey, first.Key)
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, false)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, first)
	c.Set("auto_group", firstGroup)
	c.Set("username", user.Username)
	c.Set("token_name", token.Name)
	c.Set("token_quota", token.RemainQuota)
	return c, recorder
}

func readFinalGroupRelayState(t *testing.T, db *gorm.DB, userID, tokenID, firstChannelID, secondChannelID int) (model.User, model.Token, model.Channel, model.Channel) {
	t.Helper()
	var user model.User
	var token model.Token
	var firstChannel model.Channel
	var secondChannel model.Channel
	require.NoError(t, db.First(&user, userID).Error)
	require.NoError(t, db.First(&token, tokenID).Error)
	require.NoError(t, db.First(&firstChannel, firstChannelID).Error)
	require.NoError(t, db.First(&secondChannel, secondChannelID).Error)
	return user, token, firstChannel, secondChannel
}

func requireFinalGroupRelayBalance(t *testing.T, upstream *finalGroupRelayUpstream, want finalGroupRelayBalance) {
	t.Helper()
	samples := upstream.balanceSamples()
	require.Len(t, samples, 1)
	require.Empty(t, samples[0].err)
	require.Equal(t, want.userQuota, samples[0].userQuota)
	require.Equal(t, want.tokenRemain, samples[0].tokenRemain)
	require.Equal(t, want.tokenUsed, samples[0].tokenUsed)
}

func TestFinalGroupRelayControllerRetryAdmissionAndSettlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupFinalGroupRelayDB(t)

	tests := []struct {
		name              string
		firstGroup        string
		secondGroup       string
		initialQuota      int
		firstReserved     int
		secondReserved    int
		wantSecondCalls   int32
		wantStatus        int
		wantFinalQuota    int
		wantFinalReserved int
	}{
		{name: "free_to_paid", firstGroup: "fg-free-a", secondGroup: "fg-paid-a", initialQuota: 1000, firstReserved: 0, secondReserved: 200, wantSecondCalls: 1, wantStatus: http.StatusOK, wantFinalQuota: 200, wantFinalReserved: 200},
		{name: "paid_to_free", firstGroup: "fg-paid-b", secondGroup: "fg-free-b", initialQuota: 1000, firstReserved: 100, secondReserved: 0, wantSecondCalls: 1, wantStatus: http.StatusOK, wantFinalQuota: 0, wantFinalReserved: 0},
		{name: "paid_to_paid", firstGroup: "fg-paid-c", secondGroup: "fg-high-c", initialQuota: 1000, firstReserved: 100, secondReserved: 200, wantSecondCalls: 1, wantStatus: http.StatusOK, wantFinalQuota: 200, wantFinalReserved: 200},
		{name: "same_group_no_duplicate", firstGroup: "fg-same", secondGroup: "fg-same", initialQuota: 1000, firstReserved: 100, secondReserved: 100, wantSecondCalls: 1, wantStatus: http.StatusOK, wantFinalQuota: 100, wantFinalReserved: 100},
		{name: "insufficient_additional_quota", firstGroup: "fg-paid-d", secondGroup: "fg-high-d", initialQuota: 150, firstReserved: 100, secondReserved: 200, wantSecondCalls: 0, wantStatus: http.StatusForbidden, wantFinalQuota: 0, wantFinalReserved: 0},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user, token := seedFinalGroupRelayFunding(t, db, index+1, test.initialQuota)
			firstUpstream := newFinalGroupRelayUpstream(db, user.Id, token.Id, http.StatusInternalServerError)
			secondUpstream := newFinalGroupRelayUpstream(db, user.Id, token.Id, http.StatusOK)
			t.Cleanup(firstUpstream.server.Close)
			t.Cleanup(secondUpstream.server.Close)

			firstPriority := int64(100)
			secondPriority := int64(100)
			if test.firstGroup == test.secondGroup {
				secondPriority = 10
			}
			firstChannel := seedFinalGroupRelayChannel(t, db, "fgr-first-"+strconv.Itoa(index+1), test.firstGroup, firstUpstream.server.URL, firstPriority)
			secondChannel := seedFinalGroupRelayChannel(t, db, "fgr-second-"+strconv.Itoa(index+1), test.secondGroup, secondUpstream.server.URL, secondPriority)
			t.Cleanup(func() { model.ReleaseChannelConcurrency(secondChannel.Id) })

			tokenGroup := test.firstGroup
			if test.firstGroup != test.secondGroup {
				tokenGroup += "," + test.secondGroup
			}
			requestID := fmt.Sprintf("fgr-request-%02d", index+1)
			c, recorder := finalGroupRelayContext(t, user, token, firstChannel, test.firstGroup, tokenGroup, requestID)

			Relay(c, types.RelayFormatOpenAI)

			require.Equal(t, test.wantStatus, recorder.Code)
			require.EqualValues(t, 1, firstUpstream.calls.Load())
			require.Equal(t, test.wantSecondCalls, secondUpstream.calls.Load())
			requireFinalGroupRelayBalance(t, firstUpstream, finalGroupRelayBalance{
				userQuota: test.initialQuota - test.firstReserved, tokenRemain: test.initialQuota - test.firstReserved, tokenUsed: test.firstReserved,
			})
			if test.wantSecondCalls == 1 {
				requireFinalGroupRelayBalance(t, secondUpstream, finalGroupRelayBalance{
					userQuota: test.initialQuota - test.secondReserved, tokenRemain: test.initialQuota - test.secondReserved, tokenUsed: test.secondReserved,
				})
			} else {
				require.Empty(t, secondUpstream.balanceSamples())
			}

			value, ok := c.Get("relay_info")
			require.True(t, ok)
			info, ok := value.(*relaycommon.RelayInfo)
			require.True(t, ok)
			require.Equal(t, test.secondGroup, info.UsingGroup)
			require.Equal(t, test.secondReserved, info.PriceData.QuotaToPreConsume)
			require.Equal(t, float64(test.secondReserved)/finalGroupBaseQuota, info.PriceData.GroupRatioInfo.GroupRatio)
			require.Equal(t, test.wantFinalReserved, info.FinalPreConsumedQuota)
			require.NotNil(t, info.TieredBillingSnapshot)
			require.Equal(t, finalGroupRelayExpr, info.TieredBillingSnapshot.ExprString)
			require.Equal(t, test.secondGroup, info.TieredBillingSnapshot.Group)
			require.Equal(t, info.PriceData.GroupRatioInfo.GroupRatio, info.TieredBillingSnapshot.GroupRatio)
			require.Equal(t, test.secondReserved, info.TieredBillingSnapshot.EstimatedQuotaAfterGroup)
			require.Equal(t, "controller", info.TieredBillingSnapshot.EstimatedTier)
			require.NotNil(t, info.Billing)
			require.Equal(t, test.wantFinalReserved, info.Billing.GetPreConsumedQuota())
			require.Equal(t, "wallet", info.BillingSource)
			require.Equal(t, test.secondReserved == 0, info.PriceData.FreeModel)
			if test.wantSecondCalls == 1 {
				require.Equal(t, secondChannel.Id, info.ChannelId)
			} else {
				require.Equal(t, firstChannel.Id, info.ChannelId)
			}
			require.Equal(t, []string{strconv.Itoa(firstChannel.Id), strconv.Itoa(secondChannel.Id)}, c.GetStringSlice("use_channel"))

			gotUser, gotToken, gotFirstChannel, gotSecondChannel := readFinalGroupRelayState(t, db, user.Id, token.Id, firstChannel.Id, secondChannel.Id)
			require.Equal(t, test.initialQuota-test.wantFinalQuota, gotUser.Quota)
			require.Equal(t, test.initialQuota-test.wantFinalQuota, gotToken.RemainQuota)
			require.Equal(t, test.wantFinalQuota, gotToken.UsedQuota)
			require.Equal(t, test.wantFinalQuota, gotUser.UsedQuota)
			require.Zero(t, gotFirstChannel.UsedQuota)
			require.EqualValues(t, test.wantFinalQuota, gotSecondChannel.UsedQuota)

			var logs []model.Log
			require.NoError(t, db.Where("request_id = ?", requestID).Find(&logs).Error)
			if test.wantSecondCalls == 0 {
				require.Zero(t, gotUser.RequestCount)
				require.Empty(t, logs)
				require.Contains(t, recorder.Body.String(), string(types.ErrorCodeInsufficientUserQuota))
				return
			}

			require.Equal(t, 1, gotUser.RequestCount)
			require.Len(t, logs, 1)
			logEntry := logs[0]
			require.Equal(t, model.LogTypeConsume, logEntry.Type)
			require.Equal(t, user.Id, logEntry.UserId)
			require.Equal(t, token.Id, logEntry.TokenId)
			require.Equal(t, token.Name, logEntry.TokenName)
			require.Equal(t, finalGroupRelayModel, logEntry.ModelName)
			require.Equal(t, test.secondGroup, logEntry.Group)
			require.Equal(t, secondChannel.Id, logEntry.ChannelId)
			require.Equal(t, test.wantFinalQuota, logEntry.Quota)
			require.Equal(t, requestID, logEntry.RequestId)
			var other map[string]interface{}
			require.NoError(t, common.UnmarshalJsonStr(logEntry.Other, &other))
			require.Equal(t, "tiered_expr", other["billing_mode"])
			require.Equal(t, "controller", other["matched_tier"])
			require.Equal(t, base64.StdEncoding.EncodeToString([]byte(finalGroupRelayExpr)), other["expr_b64"])
			require.Equal(t, info.PriceData.GroupRatioInfo.GroupRatio, other["group_ratio"])
			adminInfo, ok := other["admin_info"].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, []interface{}{strconv.Itoa(firstChannel.Id), strconv.Itoa(secondChannel.Id)}, adminInfo["use_channel"])
		})
	}
}
