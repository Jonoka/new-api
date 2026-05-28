package controller

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type affiliateSettingResponse struct {
	FirstLevelEnabled          bool   `json:"first_level_enabled"`
	FirstLevelRatio            int    `json:"first_level_ratio"`
	SecondLevelEnabled         bool   `json:"second_level_enabled"`
	SecondLevelRatio           int    `json:"second_level_ratio"`
	SettlementDelaySeconds     int64  `json:"settlement_delay_seconds"`
	MinWithdrawalAmount        int    `json:"min_withdrawal_amount"`
	TriggerTopupEnabled        bool   `json:"trigger_topup_enabled"`
	TriggerSubscriptionEnabled bool   `json:"trigger_subscription_enabled"`
	UsdtChain                  string `json:"usdt_chain"`
}

type affiliateDisplayResponse struct {
	QuotaPerUnit       float64 `json:"quota_per_unit"`
	QuotaDisplayType   string  `json:"quota_display_type"`
	USDExchangeRate    float64 `json:"usd_exchange_rate"`
	CustomCurrency     string  `json:"custom_currency"`
	CustomExchangeRate float64 `json:"custom_exchange_rate"`
}

type affiliatePayoutAccountRequest struct {
	UsdtAddress   string `json:"usdt_address"`
	AlipayAccount string `json:"alipay_account"`
	AlipayName    string `json:"alipay_name"`
	AlipayQrPath  string `json:"alipay_qr_path"`
	WechatAccount string `json:"wechat_account"`
	WechatName    string `json:"wechat_name"`
	WechatQrPath  string `json:"wechat_qr_path"`
}

type affiliateWithdrawRequest struct {
	Method string `json:"method"`
	Quota  int    `json:"quota"`
}

type affiliateAdminWithdrawalRequest struct {
	Remark string `json:"remark"`
}

func affiliateSettingPayload() affiliateSettingResponse {
	affiliateSetting := setting.GetAffiliateSetting()
	return affiliateSettingResponse{
		FirstLevelEnabled:          affiliateSetting.FirstLevelEnabled,
		FirstLevelRatio:            affiliateSetting.FirstLevelRatio,
		SecondLevelEnabled:         affiliateSetting.SecondLevelEnabled,
		SecondLevelRatio:           affiliateSetting.SecondLevelRatio,
		SettlementDelaySeconds:     affiliateSetting.SettlementDelaySeconds,
		MinWithdrawalAmount:        affiliateSetting.MinWithdrawalAmount,
		TriggerTopupEnabled:        affiliateSetting.TriggerTopupEnabled,
		TriggerSubscriptionEnabled: affiliateSetting.TriggerSubscriptionEnabled,
		UsdtChain:                  affiliateSetting.UsdtChain,
	}
}

func affiliateDisplayPayload() affiliateDisplayResponse {
	generalSetting := operation_setting.GetGeneralSetting()
	return affiliateDisplayResponse{
		QuotaPerUnit:       common.QuotaPerUnit,
		QuotaDisplayType:   operation_setting.GetQuotaDisplayType(),
		USDExchangeRate:    operation_setting.USDExchangeRate,
		CustomCurrency:     generalSetting.CustomCurrencySymbol,
		CustomExchangeRate: generalSetting.CustomCurrencyExchangeRate,
	}
}

func buildAffiliateInviteLink(c *gin.Context, affCode string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	if base == "" {
		scheme := c.GetHeader("X-Forwarded-Proto")
		if scheme == "" {
			scheme = "http"
			if c.Request.TLS != nil {
				scheme = "https"
			}
		}
		base = scheme + "://" + c.Request.Host
	}
	return fmt.Sprintf("%s/register?aff=%s", base, url.QueryEscape(affCode))
}

func ensureAffiliateCode(user *model.User) error {
	if user.AffCode != "" {
		return nil
	}
	user.AffCode = common.GetRandomString(4)
	return user.Update(false)
}

func GetAffiliateSummary(c *gin.Context) {
	userId := c.GetInt("id")
	if err := model.SettleMatureAffiliateRecords(userId); err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := ensureAffiliateCode(user); err != nil {
		common.ApiError(c, err)
		return
	}
	balance, err := model.GetAffiliateBalance(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	inviteLink := buildAffiliateInviteLink(c, user.AffCode)
	promotionText := strings.ReplaceAll(setting.GetAffiliateSetting().PromotionTemplate, "{invite_link}", inviteLink)
	common.ApiSuccess(c, gin.H{
		"balance":        balance,
		"aff_code":       user.AffCode,
		"aff_count":      user.AffCount,
		"invite_link":    inviteLink,
		"promotion_text": promotionText,
		"setting":        affiliateSettingPayload(),
		"display":        affiliateDisplayPayload(),
	})
}

func GetAffiliateRecords(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	records, total, err := model.GetAffiliateRecords(userId, c.Query("status"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, pageInfo)
}

func GetAffiliateWithdrawals(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	withdrawals, total, err := model.GetAffiliateWithdrawals(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(withdrawals)
	common.ApiSuccess(c, pageInfo)
}

func GetAffiliateLeaderboard(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := model.GetAffiliateLeaderboard(c.DefaultQuery("period", "month"), limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func GetAffiliatePayoutAccount(c *gin.Context) {
	account, err := model.GetAffiliatePayoutAccount(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, account)
}

func UpdateAffiliatePayoutAccount(c *gin.Context) {
	req := affiliatePayoutAccountRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	account := &model.AffiliatePayoutAccount{
		UserId:        c.GetInt("id"),
		UsdtAddress:   req.UsdtAddress,
		UsdtChain:     setting.GetAffiliateSetting().UsdtChain,
		AlipayAccount: req.AlipayAccount,
		AlipayName:    req.AlipayName,
		AlipayQrPath:  req.AlipayQrPath,
		WechatAccount: req.WechatAccount,
		WechatName:    req.WechatName,
		WechatQrPath:  req.WechatQrPath,
	}
	if err := model.SaveAffiliatePayoutAccount(account); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, account)
}

func CreateAffiliateWithdrawal(c *gin.Context) {
	req := affiliateWithdrawRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	withdrawal, err := model.CreateAffiliateWithdrawal(c.GetInt("id"), req.Method, req.Quota)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, withdrawal)
}

func TransferAffiliateToBalance(c *gin.Context) {
	req := TransferAffQuotaRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.TransferAffiliateQuotaToBalance(c.GetInt("id"), req.Quota); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func UploadAffiliateQr(c *gin.Context) {
	method := strings.ToLower(strings.TrimSpace(c.PostForm("method")))
	if method != model.AffiliatePayoutMethodAlipay && method != model.AffiliatePayoutMethodWechat {
		common.ApiErrorMsg(c, "仅支持上传支付宝或微信收款码")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if file.Size > 2*1024*1024 {
		common.ApiErrorMsg(c, "收款码图片不能超过 2MB")
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedExt := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}
	if !allowedExt[ext] {
		common.ApiErrorMsg(c, "仅支持 png、jpg、jpeg、webp、gif 图片")
		return
	}
	dir := filepath.Join("upload", "affiliate_qr")
	if err := os.MkdirAll(dir, 0755); err != nil {
		common.ApiError(c, err)
		return
	}
	fileName := fmt.Sprintf("%d_%s_%d%s", c.GetInt("id"), method, common.GetTimestamp(), ext)
	savePath := filepath.Join(dir, fileName)
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		common.ApiError(c, err)
		return
	}
	publicPath := "/upload/affiliate_qr/" + fileName
	account, err := model.GetAffiliatePayoutAccount(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if method == model.AffiliatePayoutMethodAlipay {
		account.AlipayQrPath = publicPath
	} else {
		account.WechatQrPath = publicPath
	}
	account.UsdtChain = setting.GetAffiliateSetting().UsdtChain
	if err := model.SaveAffiliatePayoutAccount(account); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"path": publicPath})
}

func AdminListAffiliateWithdrawals(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	withdrawals, total, err := model.GetAllAffiliateWithdrawals(c.Query("status"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(withdrawals)
	common.ApiSuccess(c, pageInfo)
}

func parseAffiliateWithdrawalId(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的提现申请 ID")
		return 0, false
	}
	return id, true
}

func AdminApproveAffiliateWithdrawal(c *gin.Context) {
	id, ok := parseAffiliateWithdrawalId(c)
	if !ok {
		return
	}
	req := affiliateAdminWithdrawalRequest{}
	_ = c.ShouldBindJSON(&req)
	if err := model.ApproveAffiliateWithdrawal(id, c.GetInt("id"), req.Remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminRejectAffiliateWithdrawal(c *gin.Context) {
	id, ok := parseAffiliateWithdrawalId(c)
	if !ok {
		return
	}
	req := affiliateAdminWithdrawalRequest{}
	_ = c.ShouldBindJSON(&req)
	if err := model.RejectAffiliateWithdrawal(id, c.GetInt("id"), req.Remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminMarkAffiliateWithdrawalPaid(c *gin.Context) {
	id, ok := parseAffiliateWithdrawalId(c)
	if !ok {
		return
	}
	req := affiliateAdminWithdrawalRequest{}
	_ = c.ShouldBindJSON(&req)
	if err := model.MarkAffiliateWithdrawalPaid(id, c.GetInt("id"), req.Remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
