package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const finalGroupTestModel = "final-group-billing-test"
const finalGroupTestExpr = `len < 272000 ? tier("Short", p * 5 + c * 30 + cr * 0.5) : tier("Long", p * 10 + c * 45 + cr)`

// Exercise the actual Relay loop, selected-channel setup, HTTP adapter, usage
// settlement, BillingSession and log writer together. No production data or
// external upstream is used, and all shared settings are restored.
func TestRelayFinalGroupBilling(t *testing.T) {
	cases := []struct {
		name       string
		groups     []string
		prompt     int
		cached     int
		completion int
		special    string
		wantRatio  float64
		allFail    bool
		pinned     bool
		auto       bool
		balance    int
	}{
		{name: "cross_group_short_cached", groups: []string{"a", "b", "c"}, prompt: 1000, cached: 800, completion: 100, wantRatio: .7},
		{name: "auto_cross_group_short_cached", groups: []string{"a", "b", "c"}, prompt: 1000, cached: 800, completion: 100, wantRatio: .7, auto: true},
		{name: "direct", groups: []string{"c"}, prompt: 1000, completion: 100, wantRatio: .7},
		{name: "pinned", groups: []string{"c"}, prompt: 1000, completion: 100, wantRatio: .7, pinned: true},
		{name: "same_group", groups: []string{"a", "a"}, prompt: 1000, completion: 100, wantRatio: .22},
		{name: "special", groups: []string{"a", "c"}, prompt: 1000, cached: 800, completion: 100, special: `{"customer":{"c":0.4}}`, wantRatio: .4},
		{name: "special_zero", groups: []string{"a", "c"}, prompt: 1000, completion: 100, special: `{"customer":{"c":0}}`},
		{name: "paid_to_zero", groups: []string{"a", "free"}, prompt: 1000, completion: 100},
		{name: "zero_to_paid", groups: []string{"free", "c"}, prompt: 1000, cached: 800, completion: 100, wantRatio: .7},
		{name: "zero_paid_zero", groups: []string{"free", "a", "free-last"}, prompt: 1000, completion: 100},
		{name: "below_boundary", groups: []string{"a", "c"}, prompt: 271999, completion: 100, wantRatio: .7},
		{name: "at_boundary_cached", groups: []string{"a", "c"}, prompt: 272000, cached: 250000, completion: 100, wantRatio: .7},
		{name: "above_boundary", groups: []string{"a", "c"}, prompt: 272001, completion: 100, wantRatio: .7},
		{name: "negative_settlement_delta", groups: []string{"a", "c"}, prompt: 1, completion: 1, wantRatio: .7},
		{name: "all_failed_refund", groups: []string{"a", "b", "c"}, allFail: true},
		{name: "zero_paid_all_failed_refund", groups: []string{"free", "c"}, allFail: true},
		{name: "zero_paid_insufficient_wallet", groups: []string{"free", "c"}, balance: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupFinalGroupBillingDB(t)
			balance := tc.balance
			if balance == 0 {
				balance = 4000000 // below the baseline trust threshold
			}
			user := model.User{Username: "billing-" + tc.name, AffCode: tc.name, Quota: balance, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
			require.NoError(t, db.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "synthetic-" + tc.name, RemainQuota: balance, Status: common.TokenStatusEnabled}
			require.NoError(t, db.Create(&token).Error)
			exprJSON, err := json.Marshal(map[string]string{finalGroupTestModel: finalGroupTestExpr})
			require.NoError(t, err)
			special := tc.special
			if special == "" {
				special = `{}`
			}
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode":          `{"final-group-billing-test":"tiered_expr"}`,
				"billing_setting.billing_expr":          string(exprJSON),
				"group_ratio_setting.group_ratio":       `{"a":0.22,"b":0.8,"c":0.7,"free":0,"free-last":0}`,
				"group_ratio_setting.group_group_ratio": special,
			}))

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"final-group-billing-test","messages":[{"role":"user","content":"hello"}],"max_tokens":100}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set(string(constant.ContextKeyUserId), user.Id)
			ctx.Set(string(constant.ContextKeyUserGroup), "customer")
			ctx.Set(string(constant.ContextKeyUsingGroup), tc.groups[0])
			ctx.Set(string(constant.ContextKeyAutoGroup), tc.groups[0])
			ordered := make([]string, 0, len(tc.groups))
			for _, group := range tc.groups {
				if !strings.Contains(","+strings.Join(ordered, ",")+",", ","+group+",") {
					ordered = append(ordered, group)
				}
			}
			ctx.Set(string(constant.ContextKeyTokenGroup), strings.Join(ordered, ","))
			if tc.auto {
				oldAuto, oldUsable, oldAutoConfig := setting.AutoGroups2JsonString(), setting.UserUsableGroups2JSONString(), setting.GetAutoGroupConfig()
				t.Cleanup(func() {
					require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto))
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
					oldConfigJSON, err := common.Marshal(oldAutoConfig)
					require.NoError(t, err)
					require.NoError(t, setting.UpdateAutoGroupConfigByJsonString(string(oldConfigJSON)))
				})
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["a","b","c"]`))
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"a":"a","b":"b","c":"c"}`))
				require.NoError(t, setting.UpdateAutoGroupConfigByJsonString(`{"user_selectable":true,"description":"auto"}`))
				ctx.Set(string(constant.ContextKeyTokenGroup), "auto")
				ctx.Set(string(constant.ContextKeyTokenCrossGroupRetry), true)
			}
			ctx.Set(string(constant.ContextKeyTokenId), token.Id)
			ctx.Set(string(constant.ContextKeyTokenKey), token.Key)
			ctx.Set(string(constant.ContextKeyUserSetting), dto.UserSetting{BillingPreference: "wallet_only", QuotaWarningThreshold: -1})
			ctx.Set(common.RequestIdKey, "billing-"+tc.name)
			if tc.pinned {
				ctx.Set("specific_channel_id", "1")
			}

			var mu sync.Mutex
			type attemptBillingFacts struct {
				snapshot     billingexpr.BillingSnapshot
				group        string
				groupRatio   float64
				specialRatio float64
				hasSpecial   bool
			}
			var attempts []attemptBillingFacts
			var owners []relaycommon.BillingSettler
			channels := make([]*model.Channel, 0, len(tc.groups))
			for i, group := range tc.groups {
				index := i
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					info := ctx.MustGet("relay_info").(*relaycommon.RelayInfo)
					mu.Lock()
					attempts = append(attempts, attemptBillingFacts{
						snapshot:     *info.TieredBillingSnapshot,
						group:        info.UsingGroup,
						groupRatio:   info.PriceData.GroupRatioInfo.GroupRatio,
						specialRatio: info.PriceData.GroupRatioInfo.GroupSpecialRatio,
						hasSpecial:   info.PriceData.GroupRatioInfo.HasSpecialRatio,
					})
					owners = append(owners, info.Billing)
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					if index < len(tc.groups)-1 || tc.allFail {
						// Retry must not reload changed global model pricing.
						if err := config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"final-group-billing-test":"tier(\"changed\", p * 9999 + c * 9999)"}`}); err != nil {
							t.Errorf("change test expression: %v", err)
						}
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte(`{"error":{"message":"synthetic capacity failure","type":"server_error"}}`))
						return
					}
					_, _ = fmt.Fprintf(w, `{"id":"synthetic","object":"chat.completion","model":"final-group-billing-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d}}}`, tc.prompt, tc.completion, tc.prompt+tc.completion, tc.cached)
				}))
				t.Cleanup(upstream.Close)
				priority := int64(len(tc.groups) - i)
				channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: fmt.Sprintf("synthetic-%d", i), Key: "synthetic-key", BaseURL: &upstream.URL, Group: group, Models: finalGroupTestModel, Status: common.ChannelStatusEnabled, AutoBan: common.GetPointer(0), Priority: &priority}
				require.NoError(t, db.Create(channel).Error)
				require.NoError(t, db.Create(&model.Ability{Group: group, Model: finalGroupTestModel, ChannelId: channel.Id, Enabled: true, Priority: &priority}).Error)
				channels = append(channels, channel)
			}
			model.InitChannelCache()
			require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, channels[0], finalGroupTestModel))
			Relay(ctx, types.RelayFormatOpenAI)
			for _, channel := range channels {
				model.ReleaseChannelConcurrency(channel.Id)
			}
			info := ctx.MustGet("relay_info").(*relaycommon.RelayInfo)
			mu.Lock()
			defer mu.Unlock()
			wantAttempts := len(tc.groups)
			if tc.balance != 0 {
				wantAttempts = 0 // insufficient paid admission must not send an upstream request
				if tc.groups[0] == "free" {
					wantAttempts = 1 // a free attempt may run before paid admission is checked
				}
			}
			require.Len(t, attempts, wantAttempts, recorder.Body.String())
			for i, facts := range attempts {
				snap := facts.snapshot
				require.Equal(t, tc.groups[i], facts.group)
				require.Equal(t, finalGroupTestExpr, snap.ExprString)
				require.Equal(t, attempts[0].snapshot.ExprHash, snap.ExprHash)
				require.Equal(t, attempts[0].snapshot.EstimatedQuotaBeforeGroup, snap.EstimatedQuotaBeforeGroup)
				require.Equal(t, attempts[0].snapshot.EstimatedCompletionTokens, snap.EstimatedCompletionTokens)
				require.Equal(t, attempts[0].snapshot.QuotaPerUnit, snap.QuotaPerUnit)
				if tc.special != "" && i == len(attempts)-1 {
					require.Equal(t, tc.wantRatio, facts.groupRatio)
					require.Equal(t, tc.wantRatio, facts.specialRatio)
					require.True(t, facts.hasSpecial)
				}
				if i > 0 && owners[i-1] != nil {
					require.Same(t, owners[i-1], owners[i], "retry must keep its existing reservation owner")
				}
			}
			if len(owners) > 0 && owners[0] != nil {
				require.Equal(t, attempts[0].snapshot.EstimatedQuotaAfterGroup, info.FinalPreConsumedQuota, "repricing must not replace actual reserved quota")
			}

			wantQuota := 0
			if !tc.allFail && tc.balance == 0 {
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				inputPrice, outputPrice, cachePrice, tier := 5.0, 30.0, .5, "Short"
				if tc.prompt >= 272000 {
					inputPrice, outputPrice, cachePrice, tier = 10, 45, 1, "Long"
				}
				wantQuota = billingexpr.QuotaRound((float64(tc.prompt-tc.cached)*inputPrice + float64(tc.completion)*outputPrice + float64(tc.cached)*cachePrice) * .5 * tc.wantRatio)
				require.Equal(t, tc.groups[len(tc.groups)-1], info.UsingGroup)
				require.Equal(t, tc.wantRatio, info.TieredBillingSnapshot.GroupRatio)
				require.Equal(t, tc.wantRatio, info.PriceData.GroupRatioInfo.GroupRatio)
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Equal(t, wantQuota, logs[0].Quota)
				require.Equal(t, info.UsingGroup, logs[0].Group)
				require.Equal(t, channels[len(channels)-1].Id, logs[0].ChannelId)
				var other map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(logs[0].Other), &other))
				require.Equal(t, tc.wantRatio, other["group_ratio"])
				require.Equal(t, tier, other["matched_tier"])
				if tc.special != "" {
					require.Equal(t, tc.wantRatio, other["user_group_ratio"])
					require.True(t, info.PriceData.GroupRatioInfo.HasSpecialRatio)
				}
				if info.Billing != nil {
					require.NoError(t, info.Billing.Settle(wantQuota))
					info.Billing.Refund(ctx) // no second debit or erroneous post-success refund
				}
			} else {
				require.GreaterOrEqual(t, recorder.Code, 400)
				if info.Billing != nil {
					info.Billing.Refund(ctx) // deferred failure refund is idempotent
				}
				var count int64
				require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&count).Error)
				require.Zero(t, count)
			}
			// Refund is asynchronous in this production baseline. Wait for both
			// money legs before restoring shared DB state.
			require.Eventually(t, func() bool {
				var gotUser model.User
				var gotToken model.Token
				return db.First(&gotUser, user.Id).Error == nil && db.First(&gotToken, token.Id).Error == nil && gotUser.Quota == balance-wantQuota && gotToken.RemainQuota == balance-wantQuota && gotToken.UsedQuota == wantQuota
			}, 5*time.Second, 10*time.Millisecond)
			var gotUser model.User
			require.NoError(t, db.First(&gotUser, user.Id).Error)
			require.Equal(t, wantQuota, gotUser.UsedQuota)
			if tc.allFail || tc.balance != 0 {
				require.Zero(t, gotUser.RequestCount)
			} else {
				require.Equal(t, 1, gotUser.RequestCount)
			}
			var totalChannelQuota int64
			require.NoError(t, db.Model(&model.Channel{}).Select("COALESCE(SUM(used_quota), 0)").Scan(&totalChannelQuota).Error)
			require.EqualValues(t, wantQuota, totalChannelQuota)
		})
	}
}

func setupFinalGroupBillingDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "billing.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.UserSubscription{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMemory, oldRedis, oldBatch, oldSQLite := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.UsingSQLite
	oldLog, oldExport, oldCount, oldRetries := common.LogConsumeEnabled, common.DataExportEnabled, constant.CountToken, common.RetryTimes
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	oldFree := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.UsingSQLite = true, false, false, true
	common.LogConsumeEnabled, common.DataExportEnabled, constant.CountToken, common.RetryTimes = true, false, false, 2
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.UsingSQLite = oldMemory, oldRedis, oldBatch, oldSQLite
		common.LogConsumeEnabled, common.DataExportEnabled, constant.CountToken, common.RetryTimes = oldLog, oldExport, oldCount, oldRetries
		if oldMemory && oldDB != nil {
			model.InitChannelCache()
		}
		require.NoError(t, sqlDB.Close())
	})
	return db
}
