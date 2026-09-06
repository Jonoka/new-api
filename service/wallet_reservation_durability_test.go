package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestWalletOrdinaryAndAuthoritativeTrustAdmissionsAreJournaled(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	trustQuota := common.GetTrustQuota()

	t.Run("ordinary paid", func(t *testing.T) {
		user, token := seedGroupBillingWallet(t, 1000, 1000)
		ctx := groupBillingContext(t)
		info := groupBillingInfo(user, token)
		info.PriceData.FreeModel = false
		require.Nil(t, ReconcileBillingReservation(ctx, 200, info))
		require.NotEmpty(t, info.TaskSubmissionID)
		submission, err := model.GetTaskSubmission(info.TaskSubmissionID)
		require.NoError(t, err)
		require.Equal(t, model.TaskSubmissionStateActive, submission.State)
		require.Equal(t, 200, submission.ReservedQuota)
		info.Billing.Refund(ctx)
	})

	t.Run("database grants trust despite stale low context", func(t *testing.T) {
		user, token := seedGroupBillingWallet(t, trustQuota+1000, trustQuota+1000)
		ctx := groupBillingContext(t)
		ctx.Set("token_quota", 0)
		info := groupBillingInfo(user, token)
		info.UserQuota = 0
		info.PriceData.FreeModel = false
		require.Nil(t, ReconcileBillingReservation(ctx, 0, info))
		require.Nil(t, ReconcileBillingReservation(ctx, 300, info))
		require.Zero(t, info.FinalPreConsumedQuota)
		submission, err := model.GetTaskSubmission(info.TaskSubmissionID)
		require.NoError(t, err)
		require.Zero(t, submission.ReservedQuota)
		info.Billing.Refund(ctx)
	})

	t.Run("stale high context cannot grant trust", func(t *testing.T) {
		user, token := seedGroupBillingWallet(t, 200, 200)
		ctx := groupBillingContext(t)
		ctx.Set("token_quota", trustQuota+1000)
		info := groupBillingInfo(user, token)
		info.UserQuota = trustQuota + 1000
		info.PriceData.FreeModel = false
		apiErr := ReconcileBillingReservation(ctx, 300, info)
		require.NotNil(t, apiErr)
		require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
		var count int64
		require.NoError(t, model.DB.Model(&model.TaskSubmission{}).Where("submission_id = ?", info.TaskSubmissionID).Count(&count).Error)
		require.Zero(t, count)
		userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
		require.Equal(t, 200, userQuota)
		require.Equal(t, 200, tokenRemain)
		require.Zero(t, tokenUsed)
	})

	t.Run("stale unlimited token projection cannot bypass admission", func(t *testing.T) {
		user, token := seedGroupBillingWallet(t, 1000, 100)
		ctx := groupBillingContext(t)
		info := groupBillingInfo(user, token)
		info.TokenUnlimited = true
		info.PriceData.FreeModel = false
		apiErr := ReconcileBillingReservation(ctx, 300, info)
		require.NotNil(t, apiErr)
		require.Equal(t, types.ErrorCodePreConsumeTokenQuotaFailed, apiErr.GetErrorCode())
		userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
		require.Equal(t, 1000, userQuota)
		require.Equal(t, 100, tokenRemain)
		require.Zero(t, tokenUsed)
	})
}

func TestWalletRealtimeUsesCumulativeReservationAndFinalSettlementOnce(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	user, token := seedGroupBillingWallet(t, 100000, 100000)
	ctx := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.OriginModelName = "gpt-4o-realtime-preview"
	info.UsingGroup = "default"
	info.ForcePreConsume = true
	info.PriceData.FreeModel = false
	require.Nil(t, ReconcileBillingReservation(ctx, 0, info))

	first := &dto.RealtimeUsage{TotalTokens: 10, InputTokens: 10}
	first.InputTokenDetails.TextTokens = 10
	require.NoError(t, PreWssConsumeQuota(ctx, info, first))
	firstQuota := info.Billing.GetPreConsumedQuota()
	require.Positive(t, firstQuota)

	total := &dto.RealtimeUsage{TotalTokens: 30, InputTokens: 30}
	total.InputTokenDetails.TextTokens = 30
	require.NoError(t, PreWssConsumeQuota(ctx, info, total))
	finalQuota := info.Billing.GetPreConsumedQuota()
	require.Greater(t, finalQuota, firstQuota)
	require.NoError(t, SettleBilling(ctx, info, finalQuota))
	require.NoError(t, SettleBilling(ctx, info, finalQuota))
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 100000-finalQuota, userQuota)
	require.Equal(t, 100000-finalQuota, tokenRemain)
	require.Equal(t, finalQuota, tokenUsed)
	modelRatio, _, _ := ratio_setting.GetModelRatio(info.OriginModelName)
	require.NotZero(t, modelRatio)
}

func TestWalletRealtimeCumulativeReservationUsesFrozenTieredPrice(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	user, token := seedGroupBillingWallet(t, 100000, 100000)
	ctx := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.OriginModelName = "realtime-tiered"
	info.ForcePreConsume = true
	info.PriceData = types.PriceData{ModelRatio: 0.000001, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0.000001}}
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
		BillingMode: "tiered_expr", ModelName: info.OriginModelName,
		ExprString: "p * 1000000", ExprHash: billingexpr.ExprHashString("p * 1000000"),
		Group: "default", GroupRatio: 2, QuotaPerUnit: 1, ExprVersion: billingexpr.DefaultExprVersion,
	}
	require.Nil(t, ReconcileBillingReservation(ctx, 0, info))
	usage := &dto.RealtimeUsage{TotalTokens: 30, InputTokens: 30}
	usage.InputTokenDetails.TextTokens = 30
	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	require.Equal(t, 60, info.Billing.GetPreConsumedQuota())
	require.NoError(t, SettleBilling(ctx, info, 60))
}

func TestWalletKnownSettlementFailureIsNotRecoveredAsAbandoned(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	user, token := seedGroupBillingWallet(t, 1000, 1000)
	ctx := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.ForcePreConsume = true
	info.PriceData.FreeModel = false
	require.Nil(t, ReconcileBillingReservation(ctx, 200, info))

	db := model.DB
	callback := "test:known-settlement-token-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("injected known settlement token failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
	settleErr := SettleBilling(ctx, info, 400)
	require.Error(t, settleErr)
	preserveKnownBillingSettlement(ctx, info, 400, settleErr)
	require.False(t, info.Billing.NeedsRefund())
	require.NoError(t, db.Callback().Update().Remove(callback))

	submission, err := model.GetTaskSubmission(info.TaskSubmissionID)
	require.NoError(t, err)
	require.Equal(t, model.TaskSubmissionStateSettlementPending, submission.State)
	require.Equal(t, 200, submission.ReservedQuota)
	require.Equal(t, 400, submission.AcceptedQuota)
	require.NoError(t, model.RecoverExpiredTaskSubmissions(ctx, 100))
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 800, userQuota)
	require.Equal(t, 800, tokenRemain)
	require.Equal(t, 200, tokenUsed)
}

func TestWalletViolationFeeMoneyCountersAndLogAreIdempotent(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	user, token := seedGroupBillingWallet(t, 100000, 100000)
	channel := &model.Channel{Name: fmt.Sprintf("fee-channel-%d", time.Now().UnixNano()), Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(channel).Error)
	ctx := groupBillingContext(t)
	ctx.Set("username", user.Username)
	ctx.Set("token_name", token.Name)
	info := groupBillingInfo(user, token)
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: channel.Id}
	info.BillingSource = BillingSourceWallet
	info.PriceData.GroupRatioInfo.GroupRatio = 1
	settings := model_setting.GetGrokSettings()
	old := *settings
	settings.ViolationDeductionEnabled = true
	settings.ViolationDeductionAmount = 0.05
	t.Cleanup(func() { *settings = old })
	err := types.NewError(errors.New(CSAMViolationMarker), types.ErrorCodeViolationFeeGrokCSAM)

	require.True(t, ChargeViolationFeeIfNeeded(ctx, info, err))
	require.True(t, ChargeViolationFeeIfNeeded(ctx, info, err))
	feeQuota := calcViolationFeeQuota(settings.ViolationDeductionAmount, 1)
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 100000-feeQuota, userQuota)
	require.Equal(t, 100000-feeQuota, tokenRemain)
	require.Equal(t, feeQuota, tokenUsed)
	var storedUser model.User
	var storedChannel model.Channel
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	require.NoError(t, model.DB.First(&storedChannel, channel.Id).Error)
	require.Equal(t, feeQuota, storedUser.UsedQuota)
	require.Equal(t, 1, storedUser.RequestCount)
	require.EqualValues(t, feeQuota, storedChannel.UsedQuota)
	var eventCount, logCount int64
	require.NoError(t, model.DB.Model(&model.TaskAccountingEvent{}).Where("kind = ?", "violation_fee").Count(&eventCount).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND quota = ?", user.Id, feeQuota).Count(&logCount).Error)
	require.EqualValues(t, 1, eventCount)
	require.EqualValues(t, 1, logCount)
}
