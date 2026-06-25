package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/game_setting"
	"github.com/bytedance/gopkg/util/gopool"
)

const gameJudgeBatchSize = 100

var (
	gameJudgeOnce    sync.Once
	gameJudgeRunning atomic.Bool
)

func StartGamePredictionJudgeTask() {
	gameJudgeOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			interval := game_setting.GetJudgePollInterval()
			logger.LogInfo(context.Background(), fmt.Sprintf("game prediction judge task started: tick=%s", interval))
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			runGamePredictionJudgeOnce()
			for range ticker.C {
				runGamePredictionJudgeOnce()
			}
		})
	})
}

func runGamePredictionJudgeOnce() {
	if !game_setting.IsAutoJudgeEnabled() {
		return
	}
	if !gameJudgeRunning.CompareAndSwap(false, true) {
		return
	}
	defer gameJudgeRunning.Store(false)

	predictions, err := ListDueAutoJudgePredictions(gameJudgeBatchSize)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("game judge scan failed: %v", err))
		return
	}
	for _, prediction := range predictions {
		result := model.DB.Model(&model.GamePrediction{}).Where("id = ? AND status = ?", prediction.ID, model.GamePredictionStatusOpen).Updates(map[string]interface{}{
			"judge_mode": model.GamePredictionJudgeModeManual,
			"judge_result_json": common.MapToJsonStr(map[string]interface{}{
				"reason": "JudgeProvider 未实现，已回落为人工判题",
			}),
			"updated_at": nowUnix(),
		})
		if result.Error != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("game judge fallback prediction #%d failed: %v", prediction.ID, result.Error))
			continue
		}
		if result.RowsAffected == 1 {
			model.RecordLog(0, model.LogTypeSystem, fmt.Sprintf("游戏预测 #%d 自动判题未执行：JudgeProvider 未实现，已回落为人工判题", prediction.ID))
		}
	}
}
