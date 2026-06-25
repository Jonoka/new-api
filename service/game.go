package service

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/game_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrGameInvalidAmount             = errors.New("invalid game amount")
	ErrGameInsufficientBalance       = errors.New("insufficient game token balance")
	ErrGameInsufficientQuota         = errors.New("insufficient quota")
	ErrGamePredictionNotFound        = errors.New("game prediction not found")
	ErrGamePredictionClosed          = errors.New("game prediction is closed")
	ErrGamePredictionAlreadySettled  = errors.New("game prediction already settled")
	ErrGamePredictionAnswerNotSet    = errors.New("game prediction answer is not set")
	ErrGamePredictionInvalidTime     = errors.New("invalid game prediction time")
	ErrGamePredictionInvalidOption   = errors.New("invalid game prediction option")
	ErrGamePredictionInvalidOptions  = errors.New("game prediction requires two options")
	ErrGamePredictionInvalidQuestion = errors.New("game prediction title is required")
	ErrGameAmountOverflow            = errors.New("game amount overflow")
)

type CreateGamePredictionRequest struct {
	Title       string
	Description string
	Options     []string
	CloseTime   int64
	SettleTime  int64
	JudgeMode   string
	CreatedBy   int
}

type GamePredictionSettleResult struct {
	PredictionID int   `json:"prediction_id"`
	TotalPool    int64 `json:"total_pool"`
	WinningPool  int64 `json:"winning_pool"`
	TotalPayout  int64 `json:"total_payout"`
	TotalFee     int64 `json:"total_fee"`
	WinnerCount  int   `json:"winner_count"`
}

func nowUnix() int64 {
	return time.Now().Unix()
}

func GetOrCreateGameWallet(userID int) (*model.GameWallet, error) {
	return getOrCreateGameWallet(model.DB, userID)
}

func getOrCreateGameWallet(tx *gorm.DB, userID int) (*model.GameWallet, error) {
	now := nowUnix()
	wallet := &model.GameWallet{UserID: userID, CreatedAt: now, UpdatedAt: now}
	err := tx.Where("user_id = ?", userID).First(wallet).Error
	if err == nil {
		return wallet, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(wallet).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("user_id = ?", userID).First(wallet).Error; err != nil {
		return nil, err
	}
	return wallet, nil
}

func requireOneAffected(result *gorm.DB, fallback error) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fallback
	}
	return nil
}

func checkedAddInt64(a int64, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, ErrGameAmountOverflow
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, ErrGameAmountOverflow
	}
	return a + b, nil
}

func checkedAddInt(a int, b int) (int, error) {
	if b > 0 && a > math.MaxInt-b {
		return 0, ErrGameAmountOverflow
	}
	if b < 0 && a < math.MinInt-b {
		return 0, ErrGameAmountOverflow
	}
	return a + b, nil
}

func floorMulDivInt64(a int64, b int64, divisor int64) int64 {
	if a <= 0 || b <= 0 || divisor <= 0 {
		return 0
	}
	product := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	product.Div(product, big.NewInt(divisor))
	if !product.IsInt64() {
		return math.MaxInt64
	}
	return product.Int64()
}

func floorFeeAmount(profit int64, feeRate float64) int64 {
	if profit <= 0 {
		return 0
	}
	rate := game_setting.NormalizeAwardFeeRate(feeRate)
	if rate <= 0 {
		return 0
	}
	if rate >= 1 {
		return profit
	}
	rateRat, ok := new(big.Rat).SetString(strconv.FormatFloat(rate, 'f', -1, 64))
	if !ok {
		return 0
	}
	fee := new(big.Rat).Mul(new(big.Rat).SetInt64(profit), rateRat)
	result := new(big.Int).Quo(fee.Num(), fee.Denom())
	if !result.IsInt64() {
		return profit
	}
	return result.Int64()
}

func adjustUserQuotaCacheAfterGameExchange(userID int, delta int64) {
	if err := model.AdjustUserQuotaCache(userID, delta); err != nil {
		common.SysLog(fmt.Sprintf("failed to adjust user quota cache after game wallet update: user_id=%d delta=%d error=%v", userID, delta, err))
	}
}

func gamePredictionClosedForSettlement(prediction model.GamePrediction, now int64) bool {
	return prediction.CloseTime > 0 && prediction.CloseTime <= now
}

func ExchangeQuotaToGameTokens(userID int, quota int) (*model.GameWalletTransaction, error) {
	if quota <= 0 {
		return nil, ErrGameInvalidAmount
	}
	rate := game_setting.GetTokenExchangeRate()
	if int64(quota) > math.MaxInt64/int64(rate) {
		return nil, ErrGameInvalidAmount
	}
	tokenAmount := int64(quota) * int64(rate)
	var created model.GameWalletTransaction
	err := model.ConsumePendingUserQuotaDelta(userID, func(pendingQuotaDelta int) error {
		return model.DB.Transaction(func(tx *gorm.DB) error {
			var user model.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error; err != nil {
				return err
			}
			effectiveQuota, err := checkedAddInt(user.Quota, pendingQuotaDelta)
			if err != nil {
				return err
			}
			if effectiveQuota < quota {
				return ErrGameInsufficientQuota
			}
			nextQuota, err := checkedAddInt(effectiveQuota, -quota)
			if err != nil {
				return err
			}
			wallet, err := getOrCreateGameWallet(tx, userID)
			if err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", wallet.ID).First(wallet).Error; err != nil {
				return err
			}
			if _, err := checkedAddInt64(wallet.Balance, tokenAmount); err != nil {
				return err
			}
			result := tx.Model(&model.User{}).Where("id = ?", userID).Update("quota", nextQuota)
			if err := requireOneAffected(result, ErrGameInsufficientQuota); err != nil {
				return err
			}
			result = tx.Model(&model.GameWallet{}).Where("id = ?", wallet.ID).Updates(map[string]interface{}{
				"balance":    gorm.Expr("balance + ?", tokenAmount),
				"updated_at": nowUnix(),
			})
			if err := requireOneAffected(result, ErrGameInsufficientBalance); err != nil {
				return err
			}
			if err := tx.Select("balance").Where("id = ?", wallet.ID).First(wallet).Error; err != nil {
				return err
			}
			created = model.GameWalletTransaction{
				UserID:       userID,
				WalletID:     wallet.ID,
				Type:         model.GameWalletTransactionTypeExchangeIn,
				TokenAmount:  tokenAmount,
				QuotaAmount:  quota,
				BalanceAfter: wallet.Balance,
				Content:      fmt.Sprintf("余额兑换游戏 Token：扣除余额 %d，获得 Token %d", quota, tokenAmount),
				CreatedAt:    nowUnix(),
			}
			return tx.Create(&created).Error
		})
	})
	if err != nil {
		return nil, err
	}
	adjustUserQuotaCacheAfterGameExchange(userID, -int64(quota))
	model.RecordLog(userID, model.LogTypeManage, created.Content)
	return &created, nil
}

func ExchangeGameTokensToQuota(userID int, tokens int64) (*model.GameWalletTransaction, error) {
	if tokens <= 0 {
		return nil, ErrGameInvalidAmount
	}
	rate := int64(game_setting.GetTokenExchangeRate())
	if tokens%rate != 0 {
		return nil, ErrGameInvalidAmount
	}
	quota := int(tokens / rate)
	if quota <= 0 {
		return nil, ErrGameInvalidAmount
	}
	if int64(quota) != tokens/rate {
		return nil, ErrGameInvalidAmount
	}
	tokenAmount := int64(quota) * rate
	var created model.GameWalletTransaction
	err := model.ConsumePendingUserQuotaDelta(userID, func(pendingQuotaDelta int) error {
		return model.DB.Transaction(func(tx *gorm.DB) error {
			var user model.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error; err != nil {
				return err
			}
			effectiveQuota, err := checkedAddInt(user.Quota, pendingQuotaDelta)
			if err != nil {
				return err
			}
			nextQuota, err := checkedAddInt(effectiveQuota, quota)
			if err != nil {
				return err
			}
			wallet, err := getOrCreateGameWallet(tx, userID)
			if err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", wallet.ID).First(wallet).Error; err != nil {
				return err
			}
			if wallet.Balance < tokenAmount {
				return ErrGameInsufficientBalance
			}
			result := tx.Model(&model.GameWallet{}).Where("id = ? AND balance >= ?", wallet.ID, tokenAmount).Updates(map[string]interface{}{
				"balance":    gorm.Expr("balance - ?", tokenAmount),
				"updated_at": nowUnix(),
			})
			if err := requireOneAffected(result, ErrGameInsufficientBalance); err != nil {
				return err
			}
			result = tx.Model(&model.User{}).Where("id = ?", userID).Update("quota", nextQuota)
			if err := requireOneAffected(result, ErrGameInsufficientQuota); err != nil {
				return err
			}
			if err := tx.Select("balance").Where("id = ?", wallet.ID).First(wallet).Error; err != nil {
				return err
			}
			created = model.GameWalletTransaction{
				UserID:       userID,
				WalletID:     wallet.ID,
				Type:         model.GameWalletTransactionTypeExchangeOut,
				TokenAmount:  tokenAmount,
				QuotaAmount:  quota,
				BalanceAfter: wallet.Balance,
				Content:      fmt.Sprintf("游戏 Token 兑换余额：扣除 Token %d，获得余额 %d", tokenAmount, quota),
				CreatedAt:    nowUnix(),
			}
			return tx.Create(&created).Error
		})
	})
	if err != nil {
		return nil, err
	}
	adjustUserQuotaCacheAfterGameExchange(userID, int64(quota))
	model.RecordLog(userID, model.LogTypeManage, created.Content)
	return &created, nil
}

func CreateGamePrediction(req CreateGamePredictionRequest) (*model.GamePrediction, error) {
	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)
	optionA := ""
	optionB := ""
	if len(req.Options) > 0 {
		optionA = strings.TrimSpace(req.Options[0])
	}
	if len(req.Options) > 1 {
		optionB = strings.TrimSpace(req.Options[1])
	}
	if title == "" {
		return nil, ErrGamePredictionInvalidQuestion
	}
	if len(req.Options) != 2 || optionA == "" || optionB == "" {
		return nil, ErrGamePredictionInvalidOptions
	}
	now := nowUnix()
	if req.CloseTime <= now || (req.SettleTime > 0 && req.SettleTime < req.CloseTime) {
		return nil, ErrGamePredictionInvalidTime
	}
	judgeMode := NormalizeGameJudgeMode(req.JudgeMode)
	prediction := &model.GamePrediction{
		Title:       title,
		Description: description,
		Status:      model.GamePredictionStatusOpen,
		JudgeMode:   judgeMode,
		CloseTime:   req.CloseTime,
		SettleTime:  req.SettleTime,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
		Options: []model.GamePredictionOption{
			{Index: 1, Title: optionA, CreatedAt: now, UpdatedAt: now},
			{Index: 2, Title: optionB, CreatedAt: now, UpdatedAt: now},
		},
	}
	if err := model.DB.Create(prediction).Error; err != nil {
		return nil, err
	}
	return GetGamePrediction(prediction.ID)
}

func GetGamePrediction(predictionID int) (*model.GamePrediction, error) {
	var prediction model.GamePrediction
	if err := model.DB.Preload("Options", func(db *gorm.DB) *gorm.DB {
		return db.Order("option_index asc")
	}).Where("id = ?", predictionID).First(&prediction).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGamePredictionNotFound
		}
		return nil, err
	}
	return &prediction, nil
}

func ListGamePredictions(includeSettled bool) ([]model.GamePrediction, error) {
	var predictions []model.GamePrediction
	query := model.DB.Preload("Options", func(db *gorm.DB) *gorm.DB {
		return db.Order("option_index asc")
	}).Order("id desc")
	if !includeSettled {
		query = query.Where("status <> ?", model.GamePredictionStatusSettled)
	}
	if err := query.Find(&predictions).Error; err != nil {
		return nil, err
	}
	return predictions, nil
}

func ListGamePredictionsPage(includeSettled bool, pageInfo *common.PageInfo) ([]model.GamePrediction, int64, error) {
	var predictions []model.GamePrediction
	var total int64
	query := model.DB.Model(&model.GamePrediction{})
	if !includeSettled {
		query = query.Where("status <> ?", model.GamePredictionStatusSettled)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Preload("Options", func(db *gorm.DB) *gorm.DB {
		return db.Order("option_index asc")
	}).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&predictions).Error; err != nil {
		return nil, 0, err
	}
	return predictions, total, nil
}

func ListGameWalletTransactions(userID int, pageInfo *common.PageInfo) ([]model.GameWalletTransaction, int64, error) {
	var transactions []model.GameWalletTransaction
	var total int64
	query := model.DB.Model(&model.GameWalletTransaction{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&transactions).Error; err != nil {
		return nil, 0, err
	}
	return transactions, total, nil
}

func ListAdminGamePredictionsPage(pageInfo *common.PageInfo) ([]model.GamePrediction, int64, error) {
	return ListGamePredictionsPage(true, pageInfo)
}

func PlaceGamePredictionBet(userID int, predictionID int, optionID int, amount int64) (*model.GamePredictionBet, error) {
	if amount <= 0 {
		return nil, ErrGameInvalidAmount
	}
	var created model.GamePredictionBet
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var prediction model.GamePrediction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", predictionID).First(&prediction).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrGamePredictionNotFound
			}
			return err
		}
		if prediction.Status != model.GamePredictionStatusOpen || (prediction.CloseTime > 0 && prediction.CloseTime <= nowUnix()) {
			return ErrGamePredictionClosed
		}
		var option model.GamePredictionOption
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND prediction_id = ?", optionID, predictionID).First(&option).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrGamePredictionInvalidOption
			}
			return err
		}
		wallet, err := getOrCreateGameWallet(tx, userID)
		if err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", wallet.ID).First(wallet).Error; err != nil {
			return err
		}
		if wallet.Balance < amount {
			return ErrGameInsufficientBalance
		}
		if _, err := checkedAddInt64(prediction.TotalPool, amount); err != nil {
			return err
		}
		if _, err := checkedAddInt64(option.PoolAmount, amount); err != nil {
			return err
		}
		result := tx.Model(&model.GameWallet{}).Where("id = ? AND balance >= ?", wallet.ID, amount).Updates(map[string]interface{}{
			"balance":    gorm.Expr("balance - ?", amount),
			"updated_at": nowUnix(),
		})
		if err := requireOneAffected(result, ErrGameInsufficientBalance); err != nil {
			return err
		}
		result = tx.Model(&model.GamePrediction{}).Where(
			"id = ? AND status = ? AND (close_time = ? OR close_time > ?)",
			predictionID,
			model.GamePredictionStatusOpen,
			0,
			nowUnix(),
		).Updates(map[string]interface{}{
			"total_pool": gorm.Expr("total_pool + ?", amount),
			"updated_at": nowUnix(),
		})
		if err := requireOneAffected(result, ErrGamePredictionClosed); err != nil {
			return err
		}
		result = tx.Model(&model.GamePredictionOption{}).Where("id = ?", optionID).Updates(map[string]interface{}{
			"pool_amount": gorm.Expr("pool_amount + ?", amount),
			"bet_count":   gorm.Expr("bet_count + ?", 1),
			"updated_at":  nowUnix(),
		})
		if err := requireOneAffected(result, ErrGamePredictionInvalidOption); err != nil {
			return err
		}
		created = model.GamePredictionBet{
			PredictionID: predictionID,
			OptionID:     optionID,
			UserID:       userID,
			WalletID:     wallet.ID,
			Amount:       amount,
			Status:       model.GamePredictionBetStatusActive,
			CreatedAt:    nowUnix(),
			UpdatedAt:    nowUnix(),
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Select("balance").Where("id = ?", wallet.ID).First(wallet).Error; err != nil {
			return err
		}
		return tx.Create(&model.GameWalletTransaction{
			UserID:          userID,
			WalletID:        wallet.ID,
			Type:            model.GameWalletTransactionTypeBet,
			TokenAmount:     amount,
			BalanceAfter:    wallet.Balance,
			PredictionID:    predictionID,
			PredictionBetID: created.ID,
			Content:         fmt.Sprintf("预测下注：%s，下注 Token %d", prediction.Title, amount),
			CreatedAt:       nowUnix(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func SetGamePredictionAnswer(predictionID int, optionID int, adminID int) (*model.GamePrediction, error) {
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var prediction model.GamePrediction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", predictionID).First(&prediction).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrGamePredictionNotFound
			}
			return err
		}
		if prediction.Status == model.GamePredictionStatusSettled {
			return ErrGamePredictionAlreadySettled
		}
		if !gamePredictionClosedForSettlement(prediction, nowUnix()) {
			return ErrGamePredictionClosed
		}
		var option model.GamePredictionOption
		if err := tx.Where("id = ? AND prediction_id = ?", optionID, predictionID).First(&option).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrGamePredictionInvalidOption
			}
			return err
		}
		return tx.Model(&model.GamePrediction{}).Where("id = ?", predictionID).Updates(map[string]interface{}{
			"answer_option_id": optionID,
			"answer_set_by":    adminID,
			"answered_at":      nowUnix(),
			"status":           model.GamePredictionStatusAnswered,
			"updated_at":       nowUnix(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return GetGamePrediction(predictionID)
}

func SetGamePredictionAnswerByIndex(predictionID int, answerIndex int, adminID int) (*model.GamePrediction, error) {
	var option model.GamePredictionOption
	err := model.DB.Where("prediction_id = ? AND option_index = ?", predictionID, answerIndex).First(&option).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGamePredictionInvalidOption
		}
		return nil, err
	}
	return SetGamePredictionAnswer(predictionID, option.ID, adminID)
}

func SettleGamePrediction(predictionID int, adminID int) (*GamePredictionSettleResult, error) {
	result := &GamePredictionSettleResult{PredictionID: predictionID}
	winnerLogContents := make([]struct {
		userID  int
		content string
	}, 0)
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var prediction model.GamePrediction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", predictionID).First(&prediction).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrGamePredictionNotFound
			}
			return err
		}
		if prediction.Status == model.GamePredictionStatusSettled || prediction.SettledAt > 0 {
			return ErrGamePredictionAlreadySettled
		}
		if prediction.AnswerOptionID == 0 {
			return ErrGamePredictionAnswerNotSet
		}
		if !gamePredictionClosedForSettlement(prediction, nowUnix()) {
			return ErrGamePredictionClosed
		}
		if !slices.Contains([]string{model.GamePredictionStatusOpen, model.GamePredictionStatusAnswered}, prediction.Status) {
			return ErrGamePredictionClosed
		}
		lockResult := tx.Model(&model.GamePrediction{}).Where(
			"id = ? AND status IN ? AND settled_at = ?",
			predictionID,
			[]string{model.GamePredictionStatusOpen, model.GamePredictionStatusAnswered},
			0,
		).Updates(map[string]interface{}{
			"status":     model.GamePredictionStatusSettling,
			"updated_at": nowUnix(),
		})
		if err := requireOneAffected(lockResult, ErrGamePredictionAlreadySettled); err != nil {
			return err
		}
		var bets []model.GamePredictionBet
		if err := tx.Where("prediction_id = ? AND status = ?", predictionID, model.GamePredictionBetStatusActive).Order("id asc").Find(&bets).Error; err != nil {
			return err
		}
		totalPool := int64(0)
		winningPool := int64(0)
		winnerCount := 0
		for _, bet := range bets {
			totalPool += bet.Amount
			if bet.OptionID == prediction.AnswerOptionID {
				winningPool += bet.Amount
				winnerCount++
			}
		}
		result.TotalPool = totalPool
		result.WinningPool = winningPool
		result.WinnerCount = winnerCount
		if winnerCount == 0 || winningPool == 0 || totalPool == 0 {
			if err := tx.Model(&model.GamePredictionBet{}).Where("prediction_id = ? AND status = ?", predictionID, model.GamePredictionBetStatusActive).Updates(map[string]interface{}{
				"status":     model.GamePredictionBetStatusLost,
				"settled_at": nowUnix(),
				"updated_at": nowUnix(),
			}).Error; err != nil {
				return err
			}
			result := tx.Model(&model.GamePrediction{}).Where("id = ? AND status = ?", predictionID, model.GamePredictionStatusSettling).Updates(map[string]interface{}{
				"status":       model.GamePredictionStatusSettled,
				"settled_at":   nowUnix(),
				"settled_by":   adminID,
				"total_pool":   totalPool,
				"winning_pool": winningPool,
				"winner_count": winnerCount,
				"updated_at":   nowUnix(),
			})
			return requireOneAffected(result, ErrGamePredictionAlreadySettled)
		}
		feeRate := game_setting.GetAwardFeeRate()
		allocatedGross := int64(0)
		winningSeen := 0
		for _, bet := range bets {
			if bet.OptionID != prediction.AnswerOptionID {
				result := tx.Model(&model.GamePredictionBet{}).Where("id = ? AND status = ?", bet.ID, model.GamePredictionBetStatusActive).Updates(map[string]interface{}{
					"status":     model.GamePredictionBetStatusLost,
					"settled_at": nowUnix(),
					"updated_at": nowUnix(),
				})
				if err := requireOneAffected(result, ErrGamePredictionAlreadySettled); err != nil {
					return err
				}
				continue
			}
			winningSeen++
			gross := floorMulDivInt64(totalPool, bet.Amount, winningPool)
			if winningSeen == winnerCount {
				gross = totalPool - allocatedGross
			}
			if gross < bet.Amount {
				gross = bet.Amount
			}
			allocatedGross += gross
			profit := gross - bet.Amount
			fee := floorFeeAmount(profit, feeRate)
			net := gross - fee
			var wallet model.GameWallet
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", bet.WalletID).First(&wallet).Error; err != nil {
				return err
			}
			if _, err := checkedAddInt64(wallet.Balance, net); err != nil {
				return err
			}
			updateResult := tx.Model(&model.GameWallet{}).Where("id = ?", wallet.ID).Updates(map[string]interface{}{
				"balance":    gorm.Expr("balance + ?", net),
				"updated_at": nowUnix(),
			})
			if err := requireOneAffected(updateResult, ErrGameInsufficientBalance); err != nil {
				return err
			}
			if err := tx.Select("balance").Where("id = ?", wallet.ID).First(&wallet).Error; err != nil {
				return err
			}
			content := fmt.Sprintf("预测派奖：本金 %d，毛奖励 %d，手续费 %d，净到账 %d", bet.Amount, gross, fee, net)
			payoutTx := model.GameWalletTransaction{
				UserID:          bet.UserID,
				WalletID:        wallet.ID,
				Type:            model.GameWalletTransactionTypePayout,
				TokenAmount:     gross,
				FeeAmount:       fee,
				BalanceAfter:    wallet.Balance,
				PredictionID:    predictionID,
				PredictionBetID: bet.ID,
				Content:         content,
				CreatedAt:       nowUnix(),
			}
			if err := tx.Create(&payoutTx).Error; err != nil {
				return err
			}
			updateResult = tx.Model(&model.GamePredictionBet{}).Where("id = ? AND status = ? AND payout_tx_id = ?", bet.ID, model.GamePredictionBetStatusActive, 0).Updates(map[string]interface{}{
				"status":       model.GamePredictionBetStatusWon,
				"gross_payout": gross,
				"fee_amount":   fee,
				"net_payout":   net,
				"payout_tx_id": payoutTx.ID,
				"settled_at":   nowUnix(),
				"updated_at":   nowUnix(),
			})
			if err := requireOneAffected(updateResult, ErrGamePredictionAlreadySettled); err != nil {
				return err
			}
			result.TotalPayout += net
			result.TotalFee += fee
			winnerLogContents = append(winnerLogContents, struct {
				userID  int
				content string
			}{userID: bet.UserID, content: content})
		}
		result := tx.Model(&model.GamePrediction{}).Where("id = ? AND status = ?", predictionID, model.GamePredictionStatusSettling).Updates(map[string]interface{}{
			"status":       model.GamePredictionStatusSettled,
			"settled_at":   nowUnix(),
			"settled_by":   adminID,
			"total_pool":   totalPool,
			"winning_pool": winningPool,
			"total_payout": result.TotalPayout,
			"total_fee":    result.TotalFee,
			"winner_count": winnerCount,
			"updated_at":   nowUnix(),
			"judge_result_json": common.MapToJsonStr(map[string]interface{}{
				"answer_option_id": prediction.AnswerOptionID,
				"settled_by":       adminID,
			}),
		})
		return requireOneAffected(result, ErrGamePredictionAlreadySettled)
	})
	if err != nil {
		return nil, err
	}
	for _, item := range winnerLogContents {
		model.RecordLog(item.userID, model.LogTypeManage, item.content)
	}
	return result, nil
}

func ListDueAutoJudgePredictions(limit int) ([]model.GamePrediction, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var predictions []model.GamePrediction
	err := model.DB.Where(
		"judge_mode = ? AND status = ? AND settle_time > 0 AND settle_time <= ?",
		model.GamePredictionJudgeModeAuto,
		model.GamePredictionStatusOpen,
		nowUnix(),
	).Order("settle_time asc").Limit(limit).Find(&predictions).Error
	return predictions, err
}

func NormalizeGameJudgeMode(mode string) string {
	mode = strings.TrimSpace(mode)
	switch mode {
	case model.GamePredictionJudgeModeAuto:
		return model.GamePredictionJudgeModeAuto
	default:
		return model.GamePredictionJudgeModeManual
	}
}
