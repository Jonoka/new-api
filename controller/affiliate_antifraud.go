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

	if !model.AffiliateAccessRequired(s) {
		common.ApiSuccess(c, gin.H{
			"review_enabled":    false,
			"agreement_enabled": false,
			"status":            model.AffiliateGateStatusNotRequired,
			"can_invite":        true,
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
			"review_enabled":    s.ReviewEnabled,
			"agreement_enabled": s.AgreementEnabled,
			"status":            model.AffiliateGateStatusNone,
			"can_invite":        false,
			"eligibility":       checkUserEligibility(userId, s),
		})
		return
	}

	canInvite := model.AffiliateUserCanInvite(userId, s)
	status := app.Status
	if s.AgreementEnabled && !model.AffiliateApplicationSatisfiesAgreement(app, s) {
		status = model.AffiliateGateStatusNone
	} else if !canInvite && !s.ReviewEnabled && s.AgreementEnabled {
		status = model.AffiliateGateStatusNone
	}
	common.ApiSuccess(c, gin.H{
		"review_enabled":    s.ReviewEnabled,
		"agreement_enabled": s.AgreementEnabled,
		"status":            status,
		"can_invite":        canInvite,
		"application":       app,
		"rejected_reason":   app.RejectedReason,
		"eligibility":       checkUserEligibility(userId, s),
	})
}

type eligibilityResult struct {
	Eligible   bool                   `json:"eligible"`
	Reason     string                 `json:"reason,omitempty"`
	Conditions []eligibilityCondition `json:"conditions,omitempty"`
}

type eligibilityCondition struct {
	Type     string  `json:"type"`
	Required float64 `json:"required"`
	Current  float64 `json:"current"`
	Unit     string  `json:"unit"`
	Met      bool    `json:"met"`
}

func checkUserEligibility(userId int, s *setting.AffiliateSetting) eligibilityResult {
	if s.InviterMinAccountAgeDays <= 0 && s.InviterMinRechargeAmount <= 0 {
		return eligibilityResult{Eligible: true}
	}

	user, err := model.GetUserById(userId, false)
	if err != nil {
		return eligibilityResult{Eligible: false, Reason: "user not found"}
	}

	result := eligibilityResult{Eligible: true}
	if s.InviterMinAccountAgeDays > 0 {
		currentAgeDays := (common.GetTimestamp() - user.CreatedAt) / 86400
		if currentAgeDays < 0 {
			currentAgeDays = 0
		}
		condition := eligibilityCondition{
			Type:     "account_age_days",
			Required: float64(s.InviterMinAccountAgeDays),
			Current:  float64(currentAgeDays),
			Unit:     "days",
			Met:      currentAgeDays >= int64(s.InviterMinAccountAgeDays),
		}
		result.Conditions = append(result.Conditions, condition)
		if !condition.Met {
			result.Eligible = false
			result.Reason = "account age requirement not met"
		}
	}

	if s.InviterMinRechargeAmount > 0 {
		totalRecharge, err := model.GetUserTotalRechargeAmount(userId)
		if err != nil {
			return eligibilityResult{Eligible: false, Reason: "failed to load recharge history"}
		}
		condition := eligibilityCondition{
			Type:     "recharge_amount",
			Required: float64(s.InviterMinRechargeAmount),
			Current:  totalRecharge,
			Unit:     "currency",
			Met:      totalRecharge >= float64(s.InviterMinRechargeAmount),
		}
		result.Conditions = append(result.Conditions, condition)
		if !condition.Met {
			result.Eligible = false
			if result.Reason == "" {
				result.Reason = "recharge requirement not met"
			}
		}
	}

	return result
}

type applyAffiliateRequest struct {
	AgreementAccepted bool `json:"agreement_accepted"`
}

func ApplyAffiliate(c *gin.Context) {
	userId := c.GetInt("id")
	s := setting.GetAffiliateSetting()

	if !model.AffiliateAccessRequired(s) {
		common.ApiErrorMsg(c, "affiliate application is not required")
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

func AdminRevokeAffiliateApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid application ID")
		return
	}
	if err := model.RevokeAffiliateApplication(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// Admin: Fraud Detection

func AdminListFraudAlerts(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	query := model.FraudAlertQuery{
		Status:   c.DefaultQuery("status", ""),
		IP:       c.Query("ip"),
		Keyword:  c.Query("keyword"),
		Page:     pageInfo.Page,
		PageSize: pageInfo.PageSize,
	}
	if c.Query("flat") == "true" {
		alerts, total, err := model.SearchFraudAlerts(query)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		pageInfo.SetTotal(int(total))
		pageInfo.SetItems(alerts)
		common.ApiSuccess(c, pageInfo)
		return
	}
	alerts, total, err := model.SearchFraudAlertGroups(query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(alerts)
	common.ApiSuccess(c, pageInfo)
}

func AdminScanAffiliateFraud(c *gin.Context) {
	days := common.String2Int(c.DefaultQuery("days", "30"))
	newAlerts, err := model.DetectFraudBulk(days)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"new_alerts": newAlerts,
	})
}

func AdminScanAffiliateFraudDeep(c *gin.Context) {
	days := common.String2Int(c.DefaultQuery("days", "30"))
	newAlerts, err := model.DetectFraudDeep(days)
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

func AdminDeleteFraudAlert(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid alert ID")
		return
	}
	if err := model.DeleteFraudAlert(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
