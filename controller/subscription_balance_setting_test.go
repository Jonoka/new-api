package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withBalanceSubscriptionSettings(t *testing.T, enabled bool, promoEnabled bool) {
	t.Helper()

	paymentSetting := operation_setting.GetPaymentSetting()
	originalEnabled := paymentSetting.BalanceSubscriptionEnabled
	originalPromoEnabled := paymentSetting.BalanceSubscriptionPromoEnabled
	paymentSetting.BalanceSubscriptionEnabled = enabled
	paymentSetting.BalanceSubscriptionPromoEnabled = promoEnabled
	t.Cleanup(func() {
		paymentSetting.BalanceSubscriptionEnabled = originalEnabled
		paymentSetting.BalanceSubscriptionPromoEnabled = originalPromoEnabled
	})
}

func TestSubscriptionRequestBalancePayRejectsWhenDisabled(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	withBalanceSubscriptionSettings(t, false, true)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 901).Update("quota", int(100*common.QuotaPerUnit)).Error)

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionBalancePayRequest{PlanId: plan.Id}, 901)
	SubscriptionRequestBalancePay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "余额购买订阅已关闭")
	var user model.User
	require.NoError(t, db.First(&user, 901).Error)
	assert.Equal(t, int(100*common.QuotaPerUnit), user.Quota)
	assertNoSubscriptionOrderCreated(t, db)

	err := model.PurchaseSubscriptionWithBalance(901, plan.Id, "", "203.0.113.10")
	require.ErrorContains(t, err, "余额购买订阅已关闭")
	assertNoSubscriptionOrderCreated(t, db)
}

func TestBalanceSubscriptionPromoSettingOnlyAffectsBalancePayment(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	withBalanceSubscriptionSettings(t, true, false)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 901).Update("quota", int(100*common.QuotaPerUnit)).Error)
	require.NoError(t, db.Create(&model.PromoCode{
		Name:                     "订阅优惠",
		Code:                     "SUB_PROMO",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             model.PromoCodeDiscountTypePercent,
		DiscountValue:            10,
		AppliesToAllSubscription: true,
		MaxRedeemCount:           10,
		CreatedTime:              common.GetTimestamp(),
	}).Error)

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionBalancePayRequest{
		PlanId:    plan.Id,
		PromoCode: "SUB_PROMO",
	}, 901)
	SubscriptionRequestBalancePay(ctx)
	assert.Contains(t, recorder.Body.String(), "余额购买订阅暂不支持优惠码")
	assertNoSubscriptionOrderCreated(t, db)

	ctx, recorder = newSubscriptionPaymentContext(t, SubscriptionAmountRequest{
		PlanId:        plan.Id,
		PromoCode:     "SUB_PROMO",
		PaymentMethod: model.PaymentMethodBalance,
	}, 901)
	SubscriptionRequestAmount(ctx)
	assert.Contains(t, recorder.Body.String(), "余额购买订阅暂不支持优惠码")

	ctx, recorder = newSubscriptionPaymentContext(t, SubscriptionAmountRequest{
		PlanId:        plan.Id,
		PromoCode:     "SUB_PROMO",
		PaymentMethod: "alipay",
	}, 901)
	SubscriptionRequestAmount(ctx)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
}

func TestGetTopUpInfoExposesBalanceSubscriptionSettings(t *testing.T) {
	withConfirmedPaymentCompliance(t)
	withBalanceSubscriptionSettings(t, false, false)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	GetTopUpInfo(ctx)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			BalanceEnabled bool `json:"enable_balance_subscription"`
			PromoEnabled   bool `json:"enable_balance_subscription_promo"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.False(t, response.Data.BalanceEnabled)
	assert.False(t, response.Data.PromoEnabled)
}
