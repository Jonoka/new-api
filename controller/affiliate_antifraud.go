package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func GetAffiliateAgreement(c *gin.Context) {
	s := setting.GetAffiliateSetting()
	common.ApiSuccess(c, gin.H{
		"agreement_enabled": s.AgreementEnabled,
		"agreement_text":    s.AgreementText,
		"review_enabled":    s.ReviewEnabled,
	})
}

func GetAffiliateApplicationStatus(c *gin.Context) {
	userId := c.GetInt("id")
	s := setting.GetAffiliateSetting()

	if !s.ReviewEnabled {
		common.ApiSuccess(c, gin.H{
			"review_enabled": false,
			"status":         "not_required",
		})
		return
	}

	app, err := model.GetAffiliateApplicationByUserId(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if app == nil {
		common.ApiSuccess(c, gin.H{
			"review_enabled": true,
			"status":         "none",
			"eligibility":    checkUserEligibility(userId, s),
		})
		return
	}

	common.ApiSuccess(c, gin.H{
		"review_enabled":  true,
		"status":          app.Status,
		"application":     app,
		"rejected_reason": app.RejectedReason,
	})
}

type eligibilityResult struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}

func checkUserEligibility(userId int, s *setting.AffiliateSetting) eligibilityResult {
	if s.InviterMinAccountAgeDays <= 0 && s.InviterMinRechargeAmount <= 0 {
		return eligibilityResult{Eligible: true}
	}

	user, err := model.GetUserById(userId, false)
	if err != nil {
		return eligibilityResult{Eligible: false, Reason: "user not found"}
	}

	if s.InviterMinAccountAgeDays > 0 {
		requiredAge := int64(s.InviterMinAccountAgeDays) * 86400
		if common.GetTimestamp()-user.CreatedAt < requiredAge {
			return eligibilityResult{
				Eligible: false,
				Reason:   "account age requirement not met",
			}
		}
	}

	if s.InviterMinRechargeAmount > 0 {
		var totalRecharge int64
		model.DB.Model(&model.TopUp{}).
			Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).
			Select("COALESCE(SUM(quota), 0)").
			Scan(&totalRecharge)
		if totalRecharge < int64(s.InviterMinRechargeAmount) {
			return eligibilityResult{
				Eligible: false,
				Reason:   "recharge requirement not met",
			}
		}
	}

	return eligibilityResult{Eligible: true}
}

type applyAffiliateRequest struct {
	AgreementAccepted bool `json:"agreement_accepted"`
}

func ApplyAffiliate(c *gin.Context) {
	userId := c.GetInt("id")
	s := setting.GetAffiliateSetting()

	if !s.ReviewEnabled {
		common.ApiErrorMsg(c, "affiliate review is not enabled")
		return
	}

	var req applyAffiliateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}

	if s.AgreementEnabled && !req.AgreementAccepted {
		common.ApiErrorMsg(c, "you must agree to the terms")
		return
	}

	if err := model.CreateAffiliateApplication(userId, s.AgreementText); err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, nil)
}

// Admin: Applications

func AdminListAffiliateApplications(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status := c.DefaultQuery("status", "")
	apps, total, err := model.GetPendingApplications(pageInfo.Page, pageInfo.PageSize, status)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(apps)
	common.ApiSuccess(c, pageInfo)
}

type adminApplicationActionRequest struct {
	Remark string `json:"remark"`
	Reason string `json:"reason"`
}

func AdminApproveAffiliateApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid application ID")
		return
	}
	var req adminApplicationActionRequest
	_ = c.ShouldBindJSON(&req)

	adminId := c.GetInt("id")
	if err := model.ApproveAffiliateApplication(id, adminId, req.Remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminRejectAffiliateApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid application ID")
		return
	}
	var req adminApplicationActionRequest
	_ = c.ShouldBindJSON(&req)

	adminId := c.GetInt("id")
	if err := model.RejectAffiliateApplication(id, adminId, req.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// Admin: Fraud Detection

func AdminListFraudAlerts(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status := c.DefaultQuery("status", "")
	alerts, total, err := model.GetFraudAlerts(status, pageInfo.Page, pageInfo.PageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(alerts)
	common.ApiSuccess(c, pageInfo)
}

func AdminScanAffiliateFraud(c *gin.Context) {
	newAlerts, err := model.DetectFraudBulk()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"new_alerts": newAlerts,
	})
}

type adminFraudActionRequest struct {
	Remark string `json:"remark"`
}

func AdminUnbindFraudAlert(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid alert ID")
		return
	}
	var req adminFraudActionRequest
	_ = c.ShouldBindJSON(&req)

	adminId := c.GetInt("id")
	if err := model.UnbindAffiliateRelationship(id, adminId, false); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminClawbackFraudAlert(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid alert ID")
		return
	}
	var req adminFraudActionRequest
	_ = c.ShouldBindJSON(&req)

	adminId := c.GetInt("id")
	if err := model.UnbindAffiliateRelationship(id, adminId, true); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminDismissFraudAlert(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid alert ID")
		return
	}
	var req adminFraudActionRequest
	_ = c.ShouldBindJSON(&req)

	adminId := c.GetInt("id")
	if err := model.DismissFraudAlert(id, adminId, req.Remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
