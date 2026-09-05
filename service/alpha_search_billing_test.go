package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func alphaSearchBillingFixture(t *testing.T, userQuota, tokenQuota int) (*gin.Context, *relaycommon.RelayInfo, *model.User, *model.Token, *model.Channel) {
	t.Helper()
	truncate(t)
	suffix := time.Now().UnixNano()
	user := &model.User{Username: fmt.Sprintf("alpha-%d", suffix), Password: "test", Quota: userQuota, Status: common.UserStatusEnabled}
	user.AffCode = user.Username
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: fmt.Sprintf("alpha-token-%d", suffix), Name: "alpha", Status: common.TokenStatusEnabled, RemainQuota: tokenQuota}
	require.NoError(t, model.DB.Create(token).Error)
	channel := &model.Channel{Name: fmt.Sprintf("alpha-channel-%d", suffix)}
	require.NoError(t, model.DB.Create(channel).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", nil)
	c.Set(common.RequestIdKey, fmt.Sprintf("alpha-request-%d", suffix))
	c.Set("username", user.Username)
	c.Set("token_name", token.Name)
	c.Set("token_quota", tokenQuota)
	info := &relaycommon.RelayInfo{
		UserId: user.Id, UserQuota: user.Quota, TokenId: token.Id, TokenKey: token.Key,
		OriginModelName: "gpt-4.1-alpha", UpstreamModelName: "mapped-alpha",
		UsingGroup: "paid", UserGroup: "default", RequestId: c.GetString(common.RequestIdKey),
		StartTime: time.Now().Add(-time.Second), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, IsModelMapped: true},
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only", QuotaWarningThreshold: 1},
	}
	return c, info, user, token, channel
}

func alphaExpectedQuota(t *testing.T, modelName string, groupRatio float64, multipliers map[string]float64) int {
	t.Helper()
	state, err := calculateAlphaSearchBilling(modelName, "test", types.GroupRatioInfo{GroupRatio: groupRatio}, multipliers)
	require.NoError(t, err)
	return state.quota
}

func readAlphaBalances(t *testing.T, userID, tokenID, channelID int) (model.User, model.Token, model.Channel) {
	t.Helper()
	var user model.User
	var token model.Token
	var channel model.Channel
	require.NoError(t, model.DB.First(&user, userID).Error)
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	require.NoError(t, model.DB.First(&channel, channelID).Error)
	return user, token, channel
}

func TestAlphaSearchBillingChargesOnlyConfiguredToolPriceAtomically(t *testing.T) {
	c, info, user, token, channel := alphaSearchBillingFixture(t, 1_000_000, 1_000_000)
	info.PriceData = types.PriceData{UsePrice: true, ModelPrice: 99, ModelRatio: 99, QuotaToPreConsume: 999999}
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "tier(\"base\", 1000000)", ModelName: info.OriginModelName}
	multipliers := map[string]float64{"priority": 2}
	expected := alphaExpectedQuota(t, info.OriginModelName, 1.5, multipliers)

	require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 1.5}, multipliers))
	require.Equal(t, expected, info.FinalPreConsumedQuota)
	require.Zero(t, info.PriceData.ModelPrice)
	require.Zero(t, info.PriceData.ModelRatio)
	require.False(t, info.PriceData.UsePrice)
	require.Equal(t, expected, info.PriceData.QuotaToPreConsume)

	require.NoError(t, SettleAlphaSearchBilling(c, info))
	storedUser, storedToken, storedChannel := readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Equal(t, 1_000_000-expected, storedUser.Quota)
	require.Equal(t, expected, storedUser.UsedQuota)
	require.Equal(t, 1, storedUser.RequestCount)
	require.Equal(t, 1_000_000-expected, storedToken.RemainQuota)
	require.Equal(t, expected, storedToken.UsedQuota)
	require.EqualValues(t, expected, storedChannel.UsedQuota)

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", info.RequestId).Take(&log).Error)
	require.Equal(t, info.OriginModelName, log.ModelName)
	require.Equal(t, info.UsingGroup, log.Group)
	require.Equal(t, expected, log.Quota)
	require.Zero(t, log.PromptTokens)
	require.Zero(t, log.CompletionTokens)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	require.Equal(t, true, other["alpha_search"])
	require.Equal(t, float64(1), other["web_search_call_count"])
	require.Equal(t, float64(25), other["web_search_price"])
	require.Equal(t, float64(2), other["priority"])
	require.Equal(t, "mapped-alpha", other["upstream_model_name"])

	var submission model.TaskSubmission
	require.NoError(t, model.DB.Where("submission_id = ?", info.TaskSubmissionID).Take(&submission).Error)
	require.Equal(t, model.TaskSubmissionStateSettled, submission.State)
	require.Equal(t, expected, submission.AcceptedQuota)
	var events int64
	require.NoError(t, model.DB.Model(&model.TaskAccountingEvent{}).Where("submission_id = ? AND kind = ?", info.TaskSubmissionID, "alpha_search").Count(&events).Error)
	require.EqualValues(t, 1, events)
}

func TestAlphaSearchBillingFreeToolStillCountsAndLogs(t *testing.T) {
	setting := config.GlobalConfig.Get("tool_price_setting").(*operation_setting.ToolPriceSetting)
	previous := make(map[string]float64, len(setting.Prices))
	for key, value := range setting.Prices {
		previous[key] = value
	}
	setting.Prices["web_search_preview:alpha-free*"] = 0
	operation_setting.RebuildToolPriceIndex()
	t.Cleanup(func() {
		setting.Prices = previous
		operation_setting.RebuildToolPriceIndex()
	})

	c, info, user, token, channel := alphaSearchBillingFixture(t, 1000, 1000)
	info.OriginModelName = "alpha-free-model"
	info.UserSetting.BillingPreference = "subscription_only"
	plan := &model.SubscriptionPlan{Title: fmt.Sprintf("alpha-free-%d", time.Now().UnixNano()), DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1000, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, model.DB.Create(plan).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	sub := &model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active"}
	require.NoError(t, model.DB.Create(sub).Error)

	require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 1}, nil))
	require.NotNil(t, info.Billing)
	require.Zero(t, info.FinalPreConsumedQuota)
	require.Equal(t, BillingSourceSubscription, info.BillingSource)
	require.NoError(t, SettleAlphaSearchBilling(c, info))

	storedUser, storedToken, storedChannel := readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Equal(t, 1000, storedUser.Quota)
	require.Zero(t, storedUser.UsedQuota)
	require.Equal(t, 1, storedUser.RequestCount)
	require.Equal(t, 1000, storedToken.RemainQuota)
	require.Zero(t, storedToken.UsedQuota)
	require.Zero(t, storedChannel.UsedQuota)
	var storedSub model.UserSubscription
	require.NoError(t, model.DB.First(&storedSub, sub.Id).Error)
	require.Zero(t, storedSub.AmountUsed)
	var reservations int64
	require.NoError(t, model.DB.Model(&model.SubscriptionPreConsumeRecord{}).Where("request_id = ?", info.TaskSubmissionID).Count(&reservations).Error)
	require.Zero(t, reservations)
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", info.RequestId).Take(&log).Error)
	require.Zero(t, log.Quota)
}

func TestAlphaSearchBillingPositiveToolPriceWithFreeGroupStillCounts(t *testing.T) {
	c, info, user, token, channel := alphaSearchBillingFixture(t, 1000, 1000)
	require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 0}, nil))
	require.NotNil(t, info.Billing)
	require.Zero(t, info.PriceData.Quota)
	require.Equal(t, "25", info.PriceData.BillingMeta["alpha_search_price_per_1k"])
	require.NoError(t, SettleAlphaSearchBilling(c, info))

	storedUser, storedToken, storedChannel := readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Equal(t, 1000, storedUser.Quota)
	require.Zero(t, storedUser.UsedQuota)
	require.Equal(t, 1, storedUser.RequestCount)
	require.Equal(t, 1000, storedToken.RemainQuota)
	require.Zero(t, storedToken.UsedQuota)
	require.Zero(t, storedChannel.UsedQuota)
}

func TestAlphaSearchBillingRejectsInsufficientFundsBeforeAdmission(t *testing.T) {
	c, info, user, token, channel := alphaSearchBillingFixture(t, 1, 1_000_000)
	apiErr := AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 1}, nil)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	require.Nil(t, info.Billing)

	storedUser, storedToken, storedChannel := readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Equal(t, 1, storedUser.Quota)
	require.Zero(t, storedUser.UsedQuota)
	require.Zero(t, storedUser.RequestCount)
	require.Equal(t, 1_000_000, storedToken.RemainQuota)
	require.Zero(t, storedToken.UsedQuota)
	require.Zero(t, storedChannel.UsedQuota)
	var submissions int64
	require.NoError(t, model.DB.Model(&model.TaskSubmission{}).Where("submission_id = ?", info.TaskSubmissionID).Count(&submissions).Error)
	require.Zero(t, submissions)
}

func TestAlphaSearchBillingReconcilesAttemptsAndRefundsFailureOnce(t *testing.T) {
	c, info, user, token, channel := alphaSearchBillingFixture(t, 1_000_000, 1_000_000)
	first := alphaExpectedQuota(t, info.OriginModelName, 2, nil)
	final := alphaExpectedQuota(t, info.OriginModelName, 0.5, nil)
	require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 2}, nil))
	require.Equal(t, first, info.FinalPreConsumedQuota)
	info.UsingGroup = "final"
	require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 0.5}, nil))
	require.Equal(t, final, info.FinalPreConsumedQuota)
	storedUser, storedToken, _ := readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Equal(t, 1_000_000-final, storedUser.Quota)
	require.Equal(t, 1_000_000-final, storedToken.RemainQuota)

	info.Billing.Refund(c)
	info.Billing.Refund(c)
	storedUser, storedToken, storedChannel := readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Equal(t, 1_000_000, storedUser.Quota)
	require.Equal(t, 1_000_000, storedToken.RemainQuota)
	require.Zero(t, storedToken.UsedQuota)
	require.Zero(t, storedUser.RequestCount)
	require.Zero(t, storedChannel.UsedQuota)
	var events int64
	require.NoError(t, model.DB.Model(&model.TaskAccountingEvent{}).Where("submission_id = ?", info.TaskSubmissionID).Count(&events).Error)
	require.Zero(t, events)
}

func TestAlphaSearchBillingSettlesOnlyTheFinalGroup(t *testing.T) {
	c, info, user, token, channel := alphaSearchBillingFixture(t, 1_000_000, 1_000_000)
	first := alphaExpectedQuota(t, info.OriginModelName, 2, nil)
	final := alphaExpectedQuota(t, info.OriginModelName, 0.5, nil)
	require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 2}, nil))
	require.Equal(t, first, info.FinalPreConsumedQuota)

	info.UsingGroup = "final"
	require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 0.5}, nil))
	require.Equal(t, final, info.FinalPreConsumedQuota)
	require.NoError(t, SettleAlphaSearchBilling(c, info))
	require.NoError(t, SettleAlphaSearchBilling(c, info))

	storedUser, storedToken, storedChannel := readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Equal(t, 1_000_000-final, storedUser.Quota)
	require.Equal(t, final, storedUser.UsedQuota)
	require.Equal(t, 1, storedUser.RequestCount)
	require.Equal(t, 1_000_000-final, storedToken.RemainQuota)
	require.Equal(t, final, storedToken.UsedQuota)
	require.EqualValues(t, final, storedChannel.UsedQuota)
	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", info.RequestId).Count(&logs).Error)
	require.EqualValues(t, 1, logs)
}

func TestAlphaSearchBillingSettlementFailureIsObservableAndRetryable(t *testing.T) {
	c, info, user, token, channel := alphaSearchBillingFixture(t, 1_000_000, 1_000_000)
	require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 1}, nil))
	db := model.DB
	callback := "test:alpha-search-settlement-failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "task_accounting_events" {
			tx.AddError(errors.New("injected alpha outbox failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })

	err := SettleAlphaSearchBilling(c, info)
	require.ErrorContains(t, err, "injected alpha outbox failure")
	require.True(t, info.Billing.NeedsRefund())
	storedUser, storedToken, storedChannel := readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Zero(t, storedUser.UsedQuota)
	require.Zero(t, storedUser.RequestCount)
	require.Zero(t, storedChannel.UsedQuota)
	require.Equal(t, info.FinalPreConsumedQuota, storedToken.UsedQuota)

	require.NoError(t, db.Callback().Create().Remove(callback))
	require.NoError(t, SettleAlphaSearchBilling(c, info))
	storedUser, storedToken, storedChannel = readAlphaBalances(t, user.Id, token.Id, channel.Id)
	require.Equal(t, info.PriceData.Quota, storedUser.UsedQuota)
	require.Equal(t, 1, storedUser.RequestCount)
	require.Equal(t, info.PriceData.Quota, storedToken.UsedQuota)
	require.EqualValues(t, info.PriceData.Quota, storedChannel.UsedQuota)
}

func TestAlphaSearchBillingExternalDatabases(t *testing.T) {
	fixtures := []struct {
		name      string
		dialector gorm.Dialector
	}{}
	if dsn := os.Getenv("NEW_API_TEST_POSTGRES_DSN"); dsn != "" {
		fixtures = append(fixtures, struct {
			name      string
			dialector gorm.Dialector
		}{"postgres", postgres.Open(dsn)})
	}
	if dsn := os.Getenv("NEW_API_TEST_MYSQL_DSN"); dsn != "" {
		fixtures = append(fixtures, struct {
			name      string
			dialector gorm.Dialector
		}{"mysql", mysql.Open(dsn)})
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			naming := schema.NamingStrategy{TablePrefix: "alpha_acct_"}
			db, err := gorm.Open(fixture.dialector, &gorm.Config{NamingStrategy: naming})
			require.NoError(t, err)
			require.NoError(t, migrateTaskAccountingFixture(db))
			logDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:alpha-%s-log?mode=memory&cache=shared", fixture.name)), &gorm.Config{NamingStrategy: naming})
			require.NoError(t, err)
			require.NoError(t, logDB.AutoMigrate(&model.Log{}, &model.TaskAccountingLogReceipt{}))
			useTaskAccountingDB(t, db, logDB, fixture.name)
			resetTaskAccountingFixture(t, db, logDB)
			c, info, user, token, channel := alphaSearchBillingFixture(t, 1_000_000, 1_000_000)
			expected := alphaExpectedQuota(t, info.OriginModelName, 1, nil)
			require.Nil(t, AdmitAlphaSearchBilling(c, info, types.GroupRatioInfo{GroupRatio: 1}, nil))
			require.NoError(t, SettleAlphaSearchBilling(c, info))
			storedUser, storedToken, storedChannel := readAlphaBalances(t, user.Id, token.Id, channel.Id)
			require.Equal(t, expected, storedUser.UsedQuota)
			require.Equal(t, 1, storedUser.RequestCount)
			require.Equal(t, expected, storedToken.UsedQuota)
			require.EqualValues(t, expected, storedChannel.UsedQuota)
		})
	}
}
