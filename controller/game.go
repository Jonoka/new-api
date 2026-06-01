package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type gameQuotaExchangeRequest struct {
	Quota int `json:"quota"`
}

type gameTokenExchangeRequest struct {
	Tokens int64 `json:"tokens"`
}

type gamePredictionBetRequest struct {
	OptionID int   `json:"option_id"`
	Amount   int64 `json:"amount"`
}

type gamePredictionCreateRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Options     []string `json:"options"`
	CloseTime   int64    `json:"close_time"`
	SettleTime  int64    `json:"settle_time"`
	JudgeMode   string   `json:"judge_mode"`
}

type gamePredictionAnswerRequest struct {
	OptionID    int `json:"option_id"`
	AnswerIndex int `json:"answer_index"`
}

func GetGameWallet(c *gin.Context) {
	wallet, err := service.GetOrCreateGameWallet(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, wallet)
}

func GetGameWalletTransactions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	transactions, total, err := service.ListGameWalletTransactions(c.GetInt("id"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(transactions)
	common.ApiSuccess(c, pageInfo)
}

func ExchangeQuotaToGameTokens(c *gin.Context) {
	var req gameQuotaExchangeRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	tx, err := service.ExchangeQuotaToGameTokens(c.GetInt("id"), req.Quota)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, tx)
}

func ExchangeGameTokensToQuota(c *gin.Context) {
	var req gameTokenExchangeRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	tx, err := service.ExchangeGameTokensToQuota(c.GetInt("id"), req.Tokens)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, tx)
}

func ListGamePredictions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	predictions, total, err := service.ListGamePredictionsPage(false, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(predictions)
	common.ApiSuccess(c, pageInfo)
}

func GetGamePrediction(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	prediction, err := service.GetGamePrediction(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, prediction)
}

func PlaceGamePredictionBet(c *gin.Context) {
	predictionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req gamePredictionBetRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	bet, err := service.PlaceGamePredictionBet(c.GetInt("id"), predictionID, req.OptionID, req.Amount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, bet)
}

func AdminListGamePredictions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	predictions, total, err := service.ListAdminGamePredictionsPage(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(predictions)
	common.ApiSuccess(c, pageInfo)
}

func AdminCreateGamePrediction(c *gin.Context) {
	var req gamePredictionCreateRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	prediction, err := service.CreateGamePrediction(service.CreateGamePredictionRequest{
		Title:       req.Title,
		Description: req.Description,
		Options:     req.Options,
		CloseTime:   req.CloseTime,
		SettleTime:  req.SettleTime,
		JudgeMode:   service.NormalizeGameJudgeMode(req.JudgeMode),
		CreatedBy:   c.GetInt("id"),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, prediction)
}

func AdminSetGamePredictionAnswer(c *gin.Context) {
	predictionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req gamePredictionAnswerRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	var prediction *model.GamePrediction
	if req.OptionID > 0 {
		prediction, err = service.SetGamePredictionAnswer(predictionID, req.OptionID, c.GetInt("id"))
	} else {
		prediction, err = service.SetGamePredictionAnswerByIndex(predictionID, req.AnswerIndex, c.GetInt("id"))
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, prediction)
}

func AdminSettleGamePrediction(c *gin.Context) {
	predictionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := service.SettleGamePrediction(predictionID, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
