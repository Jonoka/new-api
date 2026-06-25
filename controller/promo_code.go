package controller

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func GetAllPromoCodes(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	promoCodes, total, err := model.GetAllPromoCodes(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(promoCodes)
	common.ApiSuccess(c, pageInfo)
}

func SearchPromoCodes(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	promoCodes, total, err := model.SearchPromoCodes(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(promoCodes)
	common.ApiSuccess(c, pageInfo)
}

func GetPromoCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	promoCode, err := model.GetPromoCodeById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, promoCode)
}

func AddPromoCode(c *gin.Context) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return
	}
	promoCode := model.PromoCode{}
	if err := c.ShouldBindJSON(&promoCode); err != nil {
		common.ApiError(c, err)
		return
	}
	if utf8.RuneCountInString(strings.TrimSpace(promoCode.Name)) == 0 || utf8.RuneCountInString(promoCode.Name) > 40 {
		common.ApiErrorMsg(c, "优惠码名称长度应为 1-40 个字符")
		return
	}
	promoCode.UserId = c.GetInt("id")
	if err := promoCode.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, promoCode)
}

func UpdatePromoCode(c *gin.Context) {
	statusOnly := c.Query("status_only")
	promoCode := model.PromoCode{}
	if err := c.ShouldBindJSON(&promoCode); err != nil {
		common.ApiError(c, err)
		return
	}
	cleanPromoCode, err := model.GetPromoCodeById(promoCode.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly != "" {
		cleanPromoCode.Status = promoCode.Status
	} else {
		cleanPromoCode.Name = promoCode.Name
		cleanPromoCode.Code = promoCode.Code
		cleanPromoCode.DiscountType = promoCode.DiscountType
		cleanPromoCode.DiscountValue = promoCode.DiscountValue
		cleanPromoCode.AppliesToTopup = promoCode.AppliesToTopup
		cleanPromoCode.AppliesToAllSubscription = promoCode.AppliesToAllSubscription
		cleanPromoCode.SubscriptionPlanIds = promoCode.SubscriptionPlanIds
		cleanPromoCode.MaxRedeemCount = promoCode.MaxRedeemCount
		cleanPromoCode.ExpiredTime = promoCode.ExpiredTime
		if cleanPromoCode.MaxRedeemCount > 0 && cleanPromoCode.MaxRedeemCount < cleanPromoCode.RedeemedCount {
			common.ApiErrorMsg(c, "使用次数不能小于已使用次数")
			return
		}
		if cleanPromoCode.Status == common.RedemptionCodeStatusUsed &&
			(cleanPromoCode.MaxRedeemCount == 0 || cleanPromoCode.RedeemedCount < cleanPromoCode.MaxRedeemCount) {
			cleanPromoCode.Status = common.RedemptionCodeStatusEnabled
		}
	}
	if err := cleanPromoCode.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, cleanPromoCode)
}

func DeletePromoCode(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := model.DeletePromoCodeById(id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
