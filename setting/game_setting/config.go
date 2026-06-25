package game_setting

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/setting/config"
)

type GameSetting struct {
	TokenExchangeRate        int     `json:"token_exchange_rate"`
	AwardFeeRate             float64 `json:"award_fee_rate"`
	AutoJudgeEnabled         bool    `json:"auto_judge_enabled"`
	JudgePollIntervalSeconds int     `json:"judge_poll_interval_seconds"`
	JudgeProvider            string  `json:"judge_provider"`
	JudgeModel               string  `json:"judge_model"`
	JudgePrompt              string  `json:"judge_prompt"`
}

var gameSetting = GameSetting{
	TokenExchangeRate:        100,
	AwardFeeRate:             0.05,
	AutoJudgeEnabled:         false,
	JudgePollIntervalSeconds: 60,
	JudgeProvider:            "",
	JudgeModel:               "",
	JudgePrompt:              "",
}

func init() {
	config.GlobalConfig.Register("game_setting", &gameSetting)
}

func GetGameSetting() *GameSetting {
	return &gameSetting
}

func GetTokenExchangeRate() int {
	if gameSetting.TokenExchangeRate <= 0 {
		return 100
	}
	return gameSetting.TokenExchangeRate
}

func GetAwardFeeRate() float64 {
	return NormalizeAwardFeeRate(gameSetting.AwardFeeRate)
}

func NormalizeAwardFeeRate(rate float64) float64 {
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0
	}
	if rate < 0 {
		return 0
	}
	if rate > 1 {
		return 1
	}
	return rate
}

func IsAutoJudgeEnabled() bool {
	return gameSetting.AutoJudgeEnabled
}

func GetJudgePollInterval() time.Duration {
	seconds := gameSetting.JudgePollIntervalSeconds
	if seconds < 10 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func SetTokenExchangeRateForTest(rate int) {
	gameSetting.TokenExchangeRate = rate
}

func SetAwardFeeRateForTest(rate float64) {
	gameSetting.AwardFeeRate = rate
}
