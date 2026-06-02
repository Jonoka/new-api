package service

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/game_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGameTest(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	require.NoError(t, model.DB.AutoMigrate(
		&model.User{},
		&model.Log{},
		&model.GameWallet{},
		&model.GameWalletTransaction{},
		&model.GamePrediction{},
		&model.GamePredictionOption{},
		&model.GamePredictionBet{},
	))
	require.NoError(t, model.DB.Exec("DELETE FROM game_prediction_bets").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM game_prediction_options").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM game_predictions").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM game_wallet_transactions").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM game_wallets").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM users").Error)
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM game_prediction_bets").Error
		_ = model.DB.Exec("DELETE FROM game_prediction_options").Error
		_ = model.DB.Exec("DELETE FROM game_predictions").Error
		_ = model.DB.Exec("DELETE FROM game_wallet_transactions").Error
		_ = model.DB.Exec("DELETE FROM game_wallets").Error
		_ = model.DB.Exec("DELETE FROM users").Error
		_ = model.LOG_DB.Exec("DELETE FROM logs").Error
	})
	game_setting.SetTokenExchangeRateForTest(100)
	game_setting.SetAwardFeeRateForTest(0.10)
}

func seedGameUser(t *testing.T, id int, quota int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{
		Id:       id,
		Username: fmt.Sprintf("game_user_%d", id),
		Quota:    quota,
		Status:   common.UserStatusEnabled,
		AffCode:  fmt.Sprintf("game_aff_%d", id),
	}).Error)
}

func getGameUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return user.Quota
}

func getGameWalletBalance(t *testing.T, userID int) int64 {
	t.Helper()
	var wallet model.GameWallet
	require.NoError(t, model.DB.Where("user_id = ?", userID).First(&wallet).Error)
	return wallet.Balance
}

func closeGamePredictionForTest(t *testing.T, predictionID int) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", predictionID).Update("close_time", time.Now().Add(-time.Minute).Unix()).Error)
}

func createSettledGamePredictionFixture(t *testing.T) int {
	t.Helper()
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:       "明天苹果会开发布会吗？",
		Description: "以公开新闻为准",
		Options:     []string{"确实会开", "不会开"},
		CloseTime:   time.Now().Add(time.Hour).Unix(),
		SettleTime:  time.Now().Add(2 * time.Hour).Unix(),
		JudgeMode:   model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)

	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 1000}).Error)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 2, Balance: 1000}).Error)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 100)
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(2, prediction.ID, prediction.Options[1].ID, 300)
	require.NoError(t, err)

	closeGamePredictionForTest(t, prediction.ID)
	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)
	require.NoError(t, err)
	return prediction.ID
}

func TestGameExchangeQuotaToTokensMovesBalance(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)

	tx, err := ExchangeQuotaToGameTokens(1, 500)

	require.NoError(t, err)
	assert.Equal(t, model.GameWalletTransactionTypeExchangeIn, tx.Type)
	assert.Equal(t, 500, tx.QuotaAmount)
	assert.EqualValues(t, 50000, tx.TokenAmount)
	assert.Equal(t, 500, getGameUserQuota(t, 1))
	assert.EqualValues(t, 50000, getGameWalletBalance(t, 1))
}

func TestGameExchangeQuotaToTokensHonorsPendingBatchQuotaUpdates(t *testing.T) {
	setupGameTest(t)
	userID := 990001
	seedGameUser(t, userID, 1000)
	common.BatchUpdateEnabled = true
	t.Cleanup(func() {
		common.BatchUpdateEnabled = false
	})
	require.NoError(t, model.DecreaseUserQuota(userID, 800, false))
	assert.Equal(t, 1000, getGameUserQuota(t, userID))

	_, err := ExchangeQuotaToGameTokens(userID, 500)

	require.ErrorIs(t, err, ErrGameInsufficientQuota)
	assert.Equal(t, 1000, getGameUserQuota(t, userID))
}

func TestGameExchangeQuotaToTokensConsumesPendingBatchQuotaOnce(t *testing.T) {
	setupGameTest(t)
	userID := 990002
	seedGameUser(t, userID, 1000)
	common.BatchUpdateEnabled = true
	t.Cleanup(func() {
		common.BatchUpdateEnabled = false
	})
	require.NoError(t, model.DecreaseUserQuota(userID, 800, false))
	assert.Equal(t, 1000, getGameUserQuota(t, userID))

	_, err := ExchangeQuotaToGameTokens(userID, 100)

	require.NoError(t, err)
	assert.Equal(t, 100, getGameUserQuota(t, userID))
	assert.EqualValues(t, 10000, getGameWalletBalance(t, userID))
}

func TestGameExchangeTokensToQuotaMovesBalance(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 50000}).Error)

	tx, err := ExchangeGameTokensToQuota(1, 20000)

	require.NoError(t, err)
	assert.Equal(t, model.GameWalletTransactionTypeExchangeOut, tx.Type)
	assert.EqualValues(t, 20000, tx.TokenAmount)
	assert.Equal(t, 200, tx.QuotaAmount)
	assert.Equal(t, 1200, getGameUserQuota(t, 1))
	assert.EqualValues(t, 30000, getGameWalletBalance(t, 1))
}

func TestGameExchangeTokensToQuotaRequiresWholeExchangeUnit(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 50000}).Error)

	_, err := ExchangeGameTokensToQuota(1, 20050)

	require.ErrorIs(t, err, ErrGameInvalidAmount)
	assert.Equal(t, 1000, getGameUserQuota(t, 1))
	assert.EqualValues(t, 50000, getGameWalletBalance(t, 1))
}

func TestGameExchangeQuotaToTokensRejectsWalletOverflow(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)
	nearMax := int64(math.MaxInt64 - 10)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: nearMax}).Error)

	_, err := ExchangeQuotaToGameTokens(1, 1)

	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.Equal(t, 1000, getGameUserQuota(t, 1))
	assert.EqualValues(t, nearMax, getGameWalletBalance(t, 1))
}

func TestGameExchangeTokensToQuotaRejectsUserQuotaOverflow(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, math.MaxInt)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)

	_, err := ExchangeGameTokensToQuota(1, 100)

	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.Equal(t, math.MaxInt, getGameUserQuota(t, 1))
	assert.EqualValues(t, 100, getGameWalletBalance(t, 1))
}

func TestGameCreatePredictionValidatesAndTrimsServerSide(t *testing.T) {
	setupGameTest(t)

	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:       "  明天苹果会开发布会吗？  ",
		Description: "  以公开新闻为准  ",
		Options:     []string{"  确实会开  ", "  不会开  "},
		CloseTime:   time.Now().Add(time.Hour).Unix(),
		SettleTime:  time.Now().Add(2 * time.Hour).Unix(),
		JudgeMode:   model.GamePredictionJudgeModeManual,
	})

	require.NoError(t, err)
	assert.Equal(t, "明天苹果会开发布会吗？", prediction.Title)
	assert.Equal(t, "以公开新闻为准", prediction.Description)
	require.Len(t, prediction.Options, 2)
	assert.Equal(t, "确实会开", prediction.Options[0].Title)
	assert.Equal(t, "不会开", prediction.Options[1].Title)
}

func TestGameCreatePredictionRejectsInvalidTimes(t *testing.T) {
	setupGameTest(t)

	_, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "过期预测",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(-time.Minute).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.ErrorIs(t, err, ErrGamePredictionInvalidTime)

	_, err = CreateGamePrediction(CreateGamePredictionRequest{
		Title:      "结算时间错误",
		Options:    []string{"会", "不会"},
		CloseTime:  time.Now().Add(time.Hour).Unix(),
		SettleTime: time.Now().Add(30 * time.Minute).Unix(),
		JudgeMode:  model.GamePredictionJudgeModeManual,
	})
	require.ErrorIs(t, err, ErrGamePredictionInvalidTime)
}

func TestGameSetAnswerRejectsBeforeCloseTime(t *testing.T) {
	setupGameTest(t)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "未封盘不能判题",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)

	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)

	require.Error(t, err)
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.Equal(t, model.GamePredictionStatusOpen, updated.Status)
	assert.Equal(t, 0, updated.AnswerOptionID)
}

func TestGameSettlePredictionRejectsBeforeCloseTime(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "未封盘不能结算",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 10)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", prediction.ID).Updates(map[string]interface{}{
		"answer_option_id": prediction.Options[0].ID,
		"status":           model.GamePredictionStatusAnswered,
	}).Error)

	_, err = SettleGamePrediction(prediction.ID, 99)

	require.Error(t, err)
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.Equal(t, model.GamePredictionStatusAnswered, updated.Status)
	assert.EqualValues(t, 90, getGameWalletBalance(t, 1))
}

func TestGamePlaceBetRejectsInsufficientBalanceWithoutChangingPool(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 50}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "余额不足下注测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)

	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 100)

	require.ErrorIs(t, err, ErrGameInsufficientBalance)
	assert.EqualValues(t, 50, getGameWalletBalance(t, 1))
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, updated.TotalPool)
	assert.EqualValues(t, 0, updated.Options[0].PoolAmount)
	var betCount int64
	require.NoError(t, model.DB.Model(&model.GamePredictionBet{}).Where("prediction_id = ?", prediction.ID).Count(&betCount).Error)
	assert.EqualValues(t, 0, betCount)
}

func TestGamePlaceBetRejectsClosedPredictionWithoutChangingWallet(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "关闭后下注测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", prediction.ID).Update("close_time", time.Now().Add(-time.Minute).Unix()).Error)

	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 10)

	require.ErrorIs(t, err, ErrGamePredictionClosed)
	assert.EqualValues(t, 100, getGameWalletBalance(t, 1))
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, updated.TotalPool)
}

func TestGamePlaceBetRejectsPoolOverflowWithoutChangingWallet(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "奖池溢出下注测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	nearMax := int64(math.MaxInt64 - 5)
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", prediction.ID).Update("total_pool", nearMax).Error)
	require.NoError(t, model.DB.Model(&model.GamePredictionOption{}).Where("id = ?", prediction.Options[0].ID).Update("pool_amount", nearMax).Error)

	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 10)

	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.EqualValues(t, 100, getGameWalletBalance(t, 1))
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.EqualValues(t, nearMax, updated.TotalPool)
	assert.EqualValues(t, nearMax, updated.Options[0].PoolAmount)
}

func TestGameSettlePredictionDistributesPoolAndDeductsProfitFee(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	predictionID := createSettledGamePredictionFixture(t)

	result, err := SettleGamePrediction(predictionID, 99)

	require.NoError(t, err)
	assert.EqualValues(t, 400, result.TotalPool)
	assert.EqualValues(t, 100, result.WinningPool)
	assert.EqualValues(t, 1, result.WinnerCount)
	assert.EqualValues(t, 30, result.TotalFee)
	assert.EqualValues(t, 370, result.TotalPayout)
	assert.EqualValues(t, 1270, getGameWalletBalance(t, 1))
	assert.EqualValues(t, 700, getGameWalletBalance(t, 2))

	var payout model.GameWalletTransaction
	require.NoError(t, model.DB.Where("user_id = ? AND type = ?", 1, model.GameWalletTransactionTypePayout).First(&payout).Error)
	assert.EqualValues(t, 400, payout.TokenAmount)
	assert.EqualValues(t, 30, payout.FeeAmount)
	assert.Contains(t, payout.Content, "手续费")
}

func TestGameSettlePredictionAllocatesRemainderToWinners(t *testing.T) {
	setupGameTest(t)
	game_setting.SetAwardFeeRateForTest(0)
	for userID := 1; userID <= 4; userID++ {
		seedGameUser(t, userID, 0)
		require.NoError(t, model.DB.Create(&model.GameWallet{UserID: userID, Balance: 100}).Error)
	}
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "尾差测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	for userID := 1; userID <= 3; userID++ {
		_, err = PlaceGamePredictionBet(userID, prediction.ID, prediction.Options[0].ID, 1)
		require.NoError(t, err)
	}
	_, err = PlaceGamePredictionBet(4, prediction.ID, prediction.Options[1].ID, 97)
	require.NoError(t, err)
	closeGamePredictionForTest(t, prediction.ID)
	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)
	require.NoError(t, err)

	result, err := SettleGamePrediction(prediction.ID, 99)

	require.NoError(t, err)
	assert.EqualValues(t, 100, result.TotalPool)
	assert.EqualValues(t, 100, result.TotalPayout+result.TotalFee)
	var payouts []model.GamePredictionBet
	require.NoError(t, model.DB.Where("prediction_id = ? AND status = ?", prediction.ID, model.GamePredictionBetStatusWon).Order("id asc").Find(&payouts).Error)
	require.Len(t, payouts, 3)
	assert.EqualValues(t, 100, payouts[0].GrossPayout+payouts[1].GrossPayout+payouts[2].GrossPayout)
}

func TestGameSettlePredictionUsesIntegerMathForLargePools(t *testing.T) {
	setupGameTest(t)
	game_setting.SetAwardFeeRateForTest(0)
	largePool := int64(9_007_199_254_740_993)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: largePool}).Error)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 2, Balance: largePool}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "大额池子精度测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, largePool)
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(2, prediction.ID, prediction.Options[1].ID, 1)
	require.NoError(t, err)
	closeGamePredictionForTest(t, prediction.ID)
	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)
	require.NoError(t, err)

	result, err := SettleGamePrediction(prediction.ID, 99)

	require.NoError(t, err)
	assert.EqualValues(t, largePool+1, result.TotalPool)
	assert.EqualValues(t, largePool+1, result.TotalPayout)
	var winner model.GamePredictionBet
	require.NoError(t, model.DB.Where("prediction_id = ? AND user_id = ?", prediction.ID, 1).First(&winner).Error)
	assert.EqualValues(t, largePool+1, winner.GrossPayout)
}

func TestGameAutoJudgeFallbackKeepsRoundManualAndUnsettled(t *testing.T) {
	setupGameTest(t)
	oldAutoJudge := game_setting.GetGameSetting().AutoJudgeEnabled
	t.Cleanup(func() {
		game_setting.GetGameSetting().AutoJudgeEnabled = oldAutoJudge
	})
	game_setting.GetGameSetting().AutoJudgeEnabled = true
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:      "自动判题回落测试",
		Options:    []string{"会", "不会"},
		CloseTime:  time.Now().Add(time.Hour).Unix(),
		SettleTime: time.Now().Add(2 * time.Hour).Unix(),
		JudgeMode:  model.GamePredictionJudgeModeAuto,
	})
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 10)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", prediction.ID).Update("settle_time", time.Now().Add(-time.Minute).Unix()).Error)

	runGamePredictionJudgeOnce()

	var updated model.GamePrediction
	require.NoError(t, model.DB.Where("id = ?", prediction.ID).First(&updated).Error)
	assert.Equal(t, model.GamePredictionStatusOpen, updated.Status)
	assert.Equal(t, model.GamePredictionJudgeModeManual, updated.JudgeMode)
	var bet model.GamePredictionBet
	require.NoError(t, model.DB.Where("prediction_id = ?", prediction.ID).First(&bet).Error)
	assert.Equal(t, model.GamePredictionBetStatusActive, bet.Status)
	assert.EqualValues(t, 90, getGameWalletBalance(t, 1))
}

func TestGameSettlePredictionRejectsWalletOverflow(t *testing.T) {
	setupGameTest(t)
	game_setting.SetAwardFeeRateForTest(0)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: math.MaxInt64}).Error)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 2, Balance: 1}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "派奖溢出测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 1)
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(2, prediction.ID, prediction.Options[1].ID, 1)
	require.NoError(t, err)
	closeGamePredictionForTest(t, prediction.ID)
	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)
	require.NoError(t, err)

	_, err = SettleGamePrediction(prediction.ID, 99)

	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.EqualValues(t, int64(math.MaxInt64-1), getGameWalletBalance(t, 1))
	var updated model.GamePrediction
	require.NoError(t, model.DB.Where("id = ?", prediction.ID).First(&updated).Error)
	assert.Equal(t, model.GamePredictionStatusAnswered, updated.Status)
}

func TestGameSettlePredictionIsIdempotent(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	predictionID := createSettledGamePredictionFixture(t)

	_, err := SettleGamePrediction(predictionID, 99)
	require.NoError(t, err)
	_, err = SettleGamePrediction(predictionID, 99)

	require.ErrorIs(t, err, ErrGamePredictionAlreadySettled)
	assert.EqualValues(t, 1270, getGameWalletBalance(t, 1))
	var payoutCount int64
	require.NoError(t, model.DB.Model(&model.GameWalletTransaction{}).Where("type = ?", model.GameWalletTransactionTypePayout).Count(&payoutCount).Error)
	assert.EqualValues(t, 1, payoutCount)
}
