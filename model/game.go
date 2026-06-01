package model

import "gorm.io/gorm"

const (
	GameWalletTransactionTypeExchangeIn  = "exchange_in"
	GameWalletTransactionTypeExchangeOut = "exchange_out"
	GameWalletTransactionTypeBet         = "bet"
	GameWalletTransactionTypePayout      = "payout"

	GamePredictionStatusOpen     = "open"
	GamePredictionStatusAnswered = "answered"
	GamePredictionStatusSettling = "settling"
	GamePredictionStatusSettled  = "settled"
	GamePredictionStatusFailed   = "failed"

	GamePredictionJudgeModeManual = "manual"
	GamePredictionJudgeModeAuto   = "auto"

	GamePredictionBetStatusActive = "active"
	GamePredictionBetStatusWon    = "won"
	GamePredictionBetStatusLost   = "lost"
)

type GameWallet struct {
	ID        int   `json:"id"`
	UserID    int   `json:"user_id" gorm:"uniqueIndex;not null"`
	Balance   int64 `json:"balance" gorm:"not null;default:0"`
	CreatedAt int64 `json:"created_at" gorm:"bigint;index"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint;index"`
	User      User  `json:"-" gorm:"foreignKey:UserID"`
}

type GameWalletTransaction struct {
	ID              int    `json:"id"`
	UserID          int    `json:"user_id" gorm:"index;not null"`
	WalletID        int    `json:"wallet_id" gorm:"index;not null"`
	Type            string `json:"type" gorm:"type:varchar(32);index;not null"`
	TokenAmount     int64  `json:"token_amount" gorm:"not null;default:0"`
	QuotaAmount     int    `json:"quota_amount" gorm:"not null;default:0"`
	FeeAmount       int64  `json:"fee_amount" gorm:"not null;default:0"`
	BalanceAfter    int64  `json:"balance_after" gorm:"not null;default:0"`
	PredictionID    int    `json:"prediction_id" gorm:"index;default:0"`
	PredictionBetID int    `json:"prediction_bet_id" gorm:"index;default:0"`
	Content         string `json:"content" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index"`
}

type GamePrediction struct {
	ID              int                    `json:"id"`
	Title           string                 `json:"title" gorm:"type:varchar(255);not null"`
	Description     string                 `json:"description" gorm:"type:text"`
	Status          string                 `json:"status" gorm:"type:varchar(32);index;not null"`
	JudgeMode       string                 `json:"judge_mode" gorm:"type:varchar(32);index;not null"`
	CloseTime       int64                  `json:"close_time" gorm:"bigint;index"`
	SettleTime      int64                  `json:"settle_time" gorm:"bigint;index"`
	AnswerOptionID  int                    `json:"answer_option_id" gorm:"index;default:0"`
	AnswerSetBy     int                    `json:"answer_set_by" gorm:"default:0"`
	AnsweredAt      int64                  `json:"answered_at" gorm:"bigint;default:0"`
	SettledAt       int64                  `json:"settled_at" gorm:"bigint;default:0"`
	SettledBy       int                    `json:"settled_by" gorm:"default:0"`
	TotalPool       int64                  `json:"total_pool" gorm:"not null;default:0"`
	WinningPool     int64                  `json:"winning_pool" gorm:"not null;default:0"`
	TotalPayout     int64                  `json:"total_payout" gorm:"not null;default:0"`
	TotalFee        int64                  `json:"total_fee" gorm:"not null;default:0"`
	WinnerCount     int                    `json:"winner_count" gorm:"not null;default:0"`
	JudgeResultJSON string                 `json:"judge_result_json" gorm:"type:text"`
	CreatedBy       int                    `json:"created_by" gorm:"index;default:0"`
	CreatedAt       int64                  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt       int64                  `json:"updated_at" gorm:"bigint;index"`
	DeletedAt       gorm.DeletedAt         `json:"-" gorm:"index"`
	Options         []GamePredictionOption `json:"options" gorm:"foreignKey:PredictionID"`
}

type GamePredictionOption struct {
	ID           int    `json:"id"`
	PredictionID int    `json:"prediction_id" gorm:"index;not null"`
	Index        int    `json:"index" gorm:"column:option_index;not null;default:0"`
	Title        string `json:"title" gorm:"type:varchar(255);not null"`
	PoolAmount   int64  `json:"pool_amount" gorm:"not null;default:0"`
	BetCount     int    `json:"bet_count" gorm:"not null;default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;index"`
}

type GamePredictionBet struct {
	ID           int    `json:"id"`
	PredictionID int    `json:"prediction_id" gorm:"index;not null"`
	OptionID     int    `json:"option_id" gorm:"index;not null"`
	UserID       int    `json:"user_id" gorm:"index;not null"`
	WalletID     int    `json:"wallet_id" gorm:"index;not null"`
	Amount       int64  `json:"amount" gorm:"not null"`
	Status       string `json:"status" gorm:"type:varchar(32);index;not null"`
	GrossPayout  int64  `json:"gross_payout" gorm:"not null;default:0"`
	FeeAmount    int64  `json:"fee_amount" gorm:"not null;default:0"`
	NetPayout    int64  `json:"net_payout" gorm:"not null;default:0"`
	PayoutTxID   int    `json:"payout_tx_id" gorm:"default:0"`
	SettledAt    int64  `json:"settled_at" gorm:"bigint;default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;index"`
}
