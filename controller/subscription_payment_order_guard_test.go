package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubscriptionPaymentControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Log{},
		&model.TopUp{},
		&model.PromoCode{},
		&model.PromoCodeUsage{},
		&model.SubscriptionPlan{},
		&model.SubscriptionOrder{},
		&model.UserSubscription{},
		&model.AffiliateRecord{},
		&model.AffiliateBalance{},
	))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func withConfirmedPaymentCompliance(t *testing.T) {
	t.Helper()

	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion

	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
}

func newSubscriptionPaymentContext(t *testing.T, body any, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	payload, err := common.Marshal(body)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/pay", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	return ctx, recorder
}

func seedSubscriptionPaymentUserAndPlan(t *testing.T, db *gorm.DB, planOverrides func(*model.SubscriptionPlan)) *model.SubscriptionPlan {
	t.Helper()

	require.NoError(t, db.Create(&model.User{
		Id:       901,
		Username: common.GetRandomString(8),
		Password: "password",
		Status:   common.UserStatusEnabled,
		Email:    "buyer@example.com",
		Group:    "default",
		AffCode:  common.GetRandomString(8),
	}).Error)

	plan := &model.SubscriptionPlan{
		Title:         "测试套餐",
		PriceAmount:   19.99,
		Currency:      "USD",
		DurationUnit:  "month",
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	if planOverrides != nil {
		planOverrides(plan)
	}
	require.NoError(t, db.Create(plan).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	return plan
}

func seedFullDiscountPromoCodeForPlan(t *testing.T, db *gorm.DB, planID int) {
	t.Helper()

	require.NoError(t, db.Create(&model.PromoCode{
		Name:                     "全额优惠",
		Code:                     "FREE_SUB",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             model.PromoCodeDiscountTypePercent,
		DiscountValue:            100,
		AppliesToAllSubscription: false,
		SubscriptionPlanIds:      fmt.Sprintf("%d", planID),
		MaxRedeemCount:           10,
		CreatedTime:              common.GetTimestamp(),
	}).Error)
}

func assertNoSubscriptionOrderCreated(t *testing.T, db *gorm.DB) {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSubscriptionStripePay_ConfigMissingDoesNotCreatePendingOrder(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.StripePriceId = "price_test"
	})

	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	setting.StripeApiSecret = ""
	setting.StripeWebhookSecret = ""
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionStripePayRequest{PlanId: plan.Id}, 901)
	SubscriptionRequestStripePay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Stripe 未配置或密钥无效")
	assertNoSubscriptionOrderCreated(t, db)
}

func TestSubscriptionEpay_ConfigMissingDoesNotCreatePendingOrder(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)

	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	operation_setting.PayAddress = ""
	operation_setting.EpayId = ""
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionEpayPayRequest{
		PlanId:        plan.Id,
		PaymentMethod: "alipay",
	}, 901)
	SubscriptionRequestEpay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "当前管理员未配置支付信息")
	assertNoSubscriptionOrderCreated(t, db)
}

func TestSubscriptionWaffoPancakePay_ConfigMissingDoesNotCreatePendingOrder(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.WaffoPancakeProductId = "prod_test"
	})

	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = ""
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionWaffoPancakePayRequest{PlanId: plan.Id}, 901)
	SubscriptionRequestWaffoPancakePay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Waffo Pancake 未配置或密钥无效")
	assertNoSubscriptionOrderCreated(t, db)
}

func TestSubscriptionStripePay_FreePromoCompletesWithoutStripeConfig(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	seedFullDiscountPromoCodeForPlan(t, db, plan.Id)

	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	setting.StripeApiSecret = ""
	setting.StripeWebhookSecret = ""
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionStripePayRequest{
		PlanId:    plan.Id,
		PromoCode: "FREE_SUB",
	}, 901)
	SubscriptionRequestStripePay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"completed":true`)

	var order model.SubscriptionOrder
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.InDelta(t, 0, order.Money, 0.000001)
	assert.Equal(t, model.PaymentProviderStripe, order.PaymentProvider)
}

func TestSubscriptionEpay_FreePromoCompletesWithoutEpayConfig(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	seedFullDiscountPromoCodeForPlan(t, db, plan.Id)

	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	operation_setting.PayAddress = ""
	operation_setting.EpayId = ""
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionEpayPayRequest{
		PlanId:        plan.Id,
		PaymentMethod: "alipay",
		PromoCode:     "FREE_SUB",
	}, 901)
	SubscriptionRequestEpay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"completed":true`)

	var order model.SubscriptionOrder
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.InDelta(t, 0, order.Money, 0.000001)
	assert.Equal(t, model.PaymentProviderEpay, order.PaymentProvider)
}

func TestSubscriptionEpayPay_UsesEpayPriceForOrderAndGatewayAmount(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.PriceAmount = 10
	})

	require.NoError(t, db.Create(&model.PromoCode{
		Name:                     "订阅半价",
		Code:                     "HALF_SUB",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             model.PromoCodeDiscountTypePercent,
		DiscountValue:            50,
		AppliesToAllSubscription: true,
		MaxRedeemCount:           10,
		CreatedTime:              common.GetTimestamp(),
	}).Error)

	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	originalPrice := operation_setting.Price
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	operation_setting.Price = 1.03
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
		operation_setting.Price = originalPrice
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionEpayPayRequest{
		PlanId:        plan.Id,
		PaymentMethod: "alipay",
		PromoCode:     "HALF_SUB",
	}, 901)
	SubscriptionRequestEpay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
		URL     string            `json:"url"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "success", response.Message)
	require.NotEmpty(t, response.URL)
	require.Equal(t, "5.15", response.Data["money"])

	var order model.SubscriptionOrder
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Equal(t, model.PaymentProviderEpay, order.PaymentProvider)
	assert.InDelta(t, 10.30, order.OriginalMoney, 0.000001)
	assert.InDelta(t, 5.15, order.DiscountMoney, 0.000001)
	assert.InDelta(t, 5.15, order.ActualMoney, 0.000001)
	assert.InDelta(t, 5.15, order.Money, 0.000001)
	assert.Equal(t, int(5*common.QuotaPerUnit), order.AffiliateSourceQuota)
}

func TestSubscriptionWaffoPancakePay_FreePromoCompletesWithoutPancakeConfig(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	seedFullDiscountPromoCodeForPlan(t, db, plan.Id)

	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = ""
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionWaffoPancakePayRequest{
		PlanId:    plan.Id,
		PromoCode: "FREE_SUB",
	}, 901)
	SubscriptionRequestWaffoPancakePay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"completed":true`)

	var order model.SubscriptionOrder
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.InDelta(t, 0, order.Money, 0.000001)
	assert.Equal(t, model.PaymentProviderWaffoPancake, order.PaymentProvider)
}

func TestSubscriptionRequestAmount_AppliesPromoCodeDiscount(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.PriceAmount = 80
	})

	require.NoError(t, db.Create(&model.PromoCode{
		Name:                     "半价订阅",
		Code:                     "SUB_HALF",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             model.PromoCodeDiscountTypePercent,
		DiscountValue:            50,
		AppliesToAllSubscription: true,
		MaxRedeemCount:           10,
		CreatedTime:              common.GetTimestamp(),
	}).Error)

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionAmountRequest{
		PlanId:    plan.Id,
		PromoCode: "sub_half",
	}, 901)
	SubscriptionRequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Message  string                         `json:"message"`
		Data     string                         `json:"data"`
		Discount *model.PromoCodeDiscountResult `json:"discount"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "success", response.Message)
	assert.Equal(t, "40.00", response.Data)
	require.NotNil(t, response.Discount)
	assert.Equal(t, "SUB_HALF", response.Discount.Code)
	assert.InDelta(t, 80, response.Discount.OriginalAmount, 0.000001)
	assert.InDelta(t, 40, response.Discount.DiscountAmount, 0.000001)
	assert.InDelta(t, 40, response.Discount.PaidAmount, 0.000001)

	var count int64
	require.NoError(t, db.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)

	paid, err := strconv.ParseFloat(response.Data, 64)
	require.NoError(t, err)
	assert.InDelta(t, 40, paid, 0.000001)
}

func TestAdminCreateSubscriptionPlan_PreservesConfiguredCurrency(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)

	ctx, recorder := newSubscriptionPaymentContext(t, AdminUpsertSubscriptionPlanRequest{
		Plan: model.SubscriptionPlan{
			Title:         "人民币套餐",
			PriceAmount:   188,
			Currency:      "cny",
			DurationUnit:  model.SubscriptionDurationMonth,
			DurationValue: 1,
			Enabled:       true,
			TotalAmount:   1000,
		},
	}, 901)
	AdminCreateSubscriptionPlan(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var plan model.SubscriptionPlan
	require.NoError(t, db.Where("title = ?", "人民币套餐").First(&plan).Error)
	assert.Equal(t, model.SubscriptionCurrencyCNY, plan.Currency)
}

func TestSubscriptionRequestAmount_UsesPaymentSpecificRatesForCnyPlan(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.PriceAmount = 188
		plan.Currency = model.SubscriptionCurrencyCNY
	})

	originalPrice := operation_setting.Price
	originalBepusdtUnitPrice := setting.BepusdtUnitPrice
	originalPayMethods := operation_setting.PayMethods
	operation_setting.Price = 1.03
	setting.BepusdtUnitPrice = 7.2
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		setting.BepusdtUnitPrice = originalBepusdtUnitPrice
		operation_setting.PayMethods = originalPayMethods
	})

	t.Run("epay applies payment price for CNY plan", func(t *testing.T) {
		ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionAmountRequest{
			PlanId:        plan.Id,
			PaymentMethod: "alipay",
		}, 901)
		SubscriptionRequestAmount(ctx)

		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Message   string  `json:"message"`
			Data      string  `json:"data"`
			Amount    float64 `json:"amount"`
			Currency  string  `json:"currency"`
			AmountUSD float64 `json:"amount_usd"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Equal(t, "success", response.Message)
		assert.Equal(t, model.SubscriptionCurrencyCNY, response.Currency)
		assert.Equal(t, "193.64", response.Data)
		assert.InDelta(t, 193.64, response.Amount, 0.000001)
		assert.InDelta(t, 188/1.03, response.AmountUSD, 0.000001)
	})

	t.Run("bepusdt keeps configured CNY plan price", func(t *testing.T) {
		ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionAmountRequest{
			PlanId:        plan.Id,
			PaymentMethod: model.PaymentMethodBepusdt,
		}, 901)
		SubscriptionRequestAmount(ctx)

		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Message  string  `json:"message"`
			Data     string  `json:"data"`
			Amount   float64 `json:"amount"`
			Currency string  `json:"currency"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Equal(t, "success", response.Message)
		assert.Equal(t, model.SubscriptionCurrencyCNY, response.Currency)
		assert.Equal(t, "188.00", response.Data)
		assert.InDelta(t, 188, response.Amount, 0.000001)
	})

	t.Run("bepusdt keeps CNY discount amount", func(t *testing.T) {
		require.NoError(t, db.Create(&model.PromoCode{
			Name:                     "人民币套餐半价",
			Code:                     "BEPUSDT_HALF",
			Status:                   common.RedemptionCodeStatusEnabled,
			DiscountType:             model.PromoCodeDiscountTypePercent,
			DiscountValue:            50,
			AppliesToAllSubscription: true,
			MaxRedeemCount:           10,
			CreatedTime:              common.GetTimestamp(),
		}).Error)

		ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionAmountRequest{
			PlanId:        plan.Id,
			PaymentMethod: model.PaymentMethodBepusdt,
			PromoCode:     "BEPUSDT_HALF",
		}, 901)
		SubscriptionRequestAmount(ctx)

		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Message  string                         `json:"message"`
			Data     string                         `json:"data"`
			Amount   float64                        `json:"amount"`
			Currency string                         `json:"currency"`
			Discount *model.PromoCodeDiscountResult `json:"discount"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Equal(t, "success", response.Message)
		assert.Equal(t, model.SubscriptionCurrencyCNY, response.Currency)
		assert.Equal(t, "94.00", response.Data)
		assert.InDelta(t, 94, response.Amount, 0.000001)
		require.NotNil(t, response.Discount)
		assert.InDelta(t, 188, response.Discount.OriginalAmount, 0.000001)
		assert.InDelta(t, 94, response.Discount.DiscountAmount, 0.000001)
		assert.InDelta(t, 94, response.Discount.PaidAmount, 0.000001)
	})
}

func TestSubscriptionRequestAmount_UsesEpayRateForUsdPlan(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.PriceAmount = 188
		plan.Currency = model.SubscriptionCurrencyUSD
	})

	originalPrice := operation_setting.Price
	originalPayMethods := operation_setting.PayMethods
	operation_setting.Price = 1.03
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		operation_setting.PayMethods = originalPayMethods
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionAmountRequest{
		PlanId:        plan.Id,
		PaymentMethod: "alipay",
	}, 901)
	SubscriptionRequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message   string  `json:"message"`
		Data      string  `json:"data"`
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		AmountUSD float64 `json:"amount_usd"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "success", response.Message)
	assert.Equal(t, model.SubscriptionCurrencyCNY, response.Currency)
	assert.Equal(t, "193.64", response.Data)
	assert.InDelta(t, 193.64, response.Amount, 0.000001)
	assert.InDelta(t, 188, response.AmountUSD, 0.000001)

	var count int64
	require.NoError(t, db.Model(&model.SubscriptionOrder{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestSubscriptionEpayPay_UsesEpayPriceForCnyPlanOrderAndGatewayAmount(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.PriceAmount = 188
		plan.Currency = model.SubscriptionCurrencyCNY
	})

	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	originalPrice := operation_setting.Price
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	operation_setting.Price = 1.03
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
		operation_setting.Price = originalPrice
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionEpayPayRequest{
		PlanId:        plan.Id,
		PaymentMethod: "alipay",
	}, 901)
	SubscriptionRequestEpay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
		URL     string            `json:"url"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "success", response.Message)
	require.NotEmpty(t, response.URL)
	require.Equal(t, "193.64", response.Data["money"])

	var order model.SubscriptionOrder
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Equal(t, model.PaymentProviderEpay, order.PaymentProvider)
	assert.InDelta(t, 193.64, order.OriginalMoney, 0.000001)
	assert.InDelta(t, 193.64, order.ActualMoney, 0.000001)
	assert.InDelta(t, 193.64, order.Money, 0.000001)
	expectedPaidUSD, err := model.SubscriptionPlanPriceUSD(plan)
	require.NoError(t, err)
	assert.Equal(t, subscriptionPaidQuotaFromUSD(expectedPaidUSD), order.AffiliateSourceQuota)
}

func TestSubscriptionRequestAmount_PreviewsInvoiceFeeWithoutTitle(t *testing.T) {
	_ = setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, model.DB, func(plan *model.SubscriptionPlan) {
		plan.PriceAmount = 100
		plan.Currency = model.SubscriptionCurrencyCNY
	})

	originalInvoiceEnabled := model.InvoiceEnabled
	originalInvoiceTypes := model.InvoiceTypes
	originalInvoiceFeeRules := model.InvoiceFeeRules
	originalPrice := operation_setting.Price
	originalPayMethods := operation_setting.PayMethods
	model.InvoiceEnabled = true
	model.InvoiceTypes = `["personal","company"]`
	model.InvoiceFeeRules = `[{"min":0,"max":500,"type":"fixed","value":50}]`
	operation_setting.Price = 1
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	t.Cleanup(func() {
		model.InvoiceEnabled = originalInvoiceEnabled
		model.InvoiceTypes = originalInvoiceTypes
		model.InvoiceFeeRules = originalInvoiceFeeRules
		operation_setting.Price = originalPrice
		operation_setting.PayMethods = originalPayMethods
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionAmountRequest{
		PlanId:        plan.Id,
		PaymentMethod: "alipay",
		Invoice: model.InvoiceRequest{
			Required: true,
			Type:     model.InvoiceTypePersonal,
		},
	}, 901)
	SubscriptionRequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message         string  `json:"message"`
		Data            string  `json:"data"`
		Amount          float64 `json:"amount"`
		InvoiceRequired bool    `json:"invoice_required"`
		InvoiceFee      float64 `json:"invoice_fee"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "success", response.Message)
	assert.Equal(t, "150.00", response.Data)
	assert.InDelta(t, 150, response.Amount, 0.000001)
	assert.True(t, response.InvoiceRequired)
	assert.InDelta(t, 50, response.InvoiceFee, 0.000001)
}

func TestPurchaseSubscriptionWithBalance_UsesCnyPlanAsUsdBase(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.PriceAmount = 73
		plan.Currency = model.SubscriptionCurrencyCNY
	})
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 901).Update("quota", int(10*common.QuotaPerUnit)).Error)

	originalPrice := operation_setting.Price
	operation_setting.Price = 7.3
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
	})

	require.NoError(t, model.PurchaseSubscriptionWithBalance(901, plan.Id, "", ""))

	var user model.User
	require.NoError(t, db.First(&user, 901).Error)
	assert.Equal(t, 0, user.Quota)

	var order model.SubscriptionOrder
	require.NoError(t, db.Where("payment_provider = ?", model.PaymentProviderBalance).First(&order).Error)
	assert.InDelta(t, 10, order.Money, 0.000001)
	assert.Equal(t, int(10*common.QuotaPerUnit), order.AffiliateSourceQuota)
}

func TestBepusdtWebhookCompletesSubscriptionOrder(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	order := &model.SubscriptionOrder{
		UserId:          901,
		PlanId:          plan.Id,
		Money:           19.99,
		TradeNo:         "BEPUSDT_SUB_TEST",
		PaymentMethod:   model.PaymentMethodBepusdt,
		PaymentProvider: model.PaymentProviderBepusdt,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/bepusdt/notify", nil)

	handleBepusdtPaymentSuccess(ctx, &bepusdtNotifyPayload{
		OrderId:      order.TradeNo,
		TradeId:      "trade_123",
		Amount:       "19.99",
		Status:       2,
		ActualAmount: "2.77",
	})

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "ok", recorder.Body.String())

	var savedOrder model.SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", order.TradeNo).First(&savedOrder).Error)
	assert.Equal(t, common.TopUpStatusSuccess, savedOrder.Status)

	var subCount int64
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", 901, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 1, subCount)
}

func TestSubscriptionOkpayPayCreatesPendingOrder(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	withConfirmedPaymentCompliance(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, func(plan *model.SubscriptionPlan) {
		plan.PriceAmount = 10
		plan.Currency = model.SubscriptionCurrencyUSD
	})

	var capturedForm url.Values
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		capturedForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"order_id":"okpay-provider-order","pay_url":"https://pay.example.test/order"}}`))
	}))
	t.Cleanup(gateway.Close)

	originalGateway := setting.OkpayGatewayUrl
	originalMerchantID := setting.OkpayMerchantId
	originalToken := setting.OkpayMerchantToken
	originalExchangeRate := setting.OkpayExchangeRate
	originalAutoRate := setting.OkpayAutoExchangeEnabled
	originalFallbackRate := setting.OkpayUsdtCnyRate
	originalCoin := setting.OkpayCoin
	setting.OkpayGatewayUrl = gateway.URL
	setting.OkpayMerchantId = "merchant-test"
	setting.OkpayMerchantToken = "secret-test"
	setting.OkpayExchangeRate = 7.2
	setting.OkpayAutoExchangeEnabled = false
	setting.OkpayUsdtCnyRate = 7.2
	setting.OkpayCoin = "USDT"
	resetOkpayRateCacheForTest()
	t.Cleanup(func() {
		setting.OkpayGatewayUrl = originalGateway
		setting.OkpayMerchantId = originalMerchantID
		setting.OkpayMerchantToken = originalToken
		setting.OkpayExchangeRate = originalExchangeRate
		setting.OkpayAutoExchangeEnabled = originalAutoRate
		setting.OkpayUsdtCnyRate = originalFallbackRate
		setting.OkpayCoin = originalCoin
		resetOkpayRateCacheForTest()
	})

	ctx, recorder := newSubscriptionPaymentContext(t, SubscriptionOkpayPayRequest{PlanId: plan.Id}, 901)
	SubscriptionRequestOkpayPay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	assert.Contains(t, recorder.Body.String(), `"payment_url":"https://pay.example.test/order"`)
	require.NotNil(t, capturedForm)
	assert.Equal(t, "10.00000000", capturedForm.Get("amount"))
	assert.Equal(t, "USDT", capturedForm.Get("coin"))

	var order model.SubscriptionOrder
	require.NoError(t, db.Where("payment_provider = ?", model.PaymentProviderOkpay).First(&order).Error)
	assert.Equal(t, model.PaymentMethodOkpay, order.PaymentMethod)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.InDelta(t, 72, order.Money, 0.000001)
	assert.Equal(t, "okpay-provider-order", order.ProviderOrderId)
	assert.Equal(t, "10.00000000", order.ProviderAmount)
	assert.Equal(t, "USDT", order.ProviderCurrency)
}

func buildSignedOkpayJSONCallbackForTest(tradeNo string, providerOrderId string, amount string, coin string, merchantId string, merchantToken string) string {
	parts := []string{
		"code=200",
		"data[order_id]=" + providerOrderId,
	}
	if tradeNo != "" {
		parts = append(parts, "data[unique_id]="+tradeNo)
	}
	parts = append(parts,
		"data[amount]="+amount,
		"data[coin]="+coin,
		"data[status]=1",
		"data[type]=deposit",
		"id="+merchantId,
		"status=success",
	)
	unsigned := strings.Join(parts, "&")
	return fmt.Sprintf(
		`{"code":200,"data":{"order_id":%q,"unique_id":%q,"amount":%q,"coin":%q,"status":1,"type":"deposit"},"id":%q,"status":"success","sign":%q}`,
		providerOrderId,
		tradeNo,
		amount,
		coin,
		merchantId,
		okpaySignatureForTest(unsigned, merchantToken),
	)
}

func TestOkpayJSONWebhookCompletesSubscriptionWhenNewPaymentsDisabled(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	order := &model.SubscriptionOrder{
		UserId:           901,
		PlanId:           plan.Id,
		Money:            72,
		TradeNo:          "OKPAY_SUB_CALLBACK_TEST",
		PaymentMethod:    model.PaymentMethodOkpay,
		PaymentProvider:  model.PaymentProviderOkpay,
		ProviderOrderId:  "provider-order",
		ProviderAmount:   "10.00000000",
		ProviderCurrency: "USDT",
		CreateTime:       common.GetTimestamp(),
		Status:           common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	originalGateway := setting.OkpayGatewayUrl
	originalMerchantID := setting.OkpayMerchantId
	originalToken := setting.OkpayMerchantToken
	setting.OkpayGatewayUrl = ""
	setting.OkpayMerchantId = "merchant-callback"
	setting.OkpayMerchantToken = "callback-secret"
	t.Cleanup(func() {
		setting.OkpayGatewayUrl = originalGateway
		setting.OkpayMerchantId = originalMerchantID
		setting.OkpayMerchantToken = originalToken
	})

	body := buildSignedOkpayJSONCallbackForTest(order.TradeNo, "provider-order", "10.00000000", "USDT", setting.OkpayMerchantId, setting.OkpayMerchantToken)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/okpay/notify", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	OkpayNotify(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"status":"success"}`, recorder.Body.String())
	assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")

	var savedOrder model.SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", order.TradeNo).First(&savedOrder).Error)
	assert.Equal(t, common.TopUpStatusSuccess, savedOrder.Status)
	var subCount int64
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", 901, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 1, subCount)
}

func TestOkpayWebhookKeepsLegacyOrderWithoutGatewaySnapshotPayable(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	order := &model.SubscriptionOrder{
		UserId:          901,
		PlanId:          plan.Id,
		Money:           72,
		TradeNo:         "OKPAY_LEGACY_CALLBACK_TEST",
		PaymentMethod:   model.PaymentMethodOkpay,
		PaymentProvider: model.PaymentProviderOkpay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	originalMerchantID := setting.OkpayMerchantId
	originalToken := setting.OkpayMerchantToken
	setting.OkpayMerchantId = "merchant-legacy"
	setting.OkpayMerchantToken = "legacy-secret"
	t.Cleanup(func() {
		setting.OkpayMerchantId = originalMerchantID
		setting.OkpayMerchantToken = originalToken
	})

	body := buildSignedOkpayJSONCallbackForTest(order.TradeNo, "legacy-provider-order", "10.00000000", "USDT", setting.OkpayMerchantId, setting.OkpayMerchantToken)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/okpay/notify", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	OkpayNotify(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"status":"success"}`, recorder.Body.String())
	var savedOrder model.SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", order.TradeNo).First(&savedOrder).Error)
	assert.Equal(t, common.TopUpStatusSuccess, savedOrder.Status)
}

func TestOkpayWebhookStrictlyValidatesStoredGatewaySnapshot(t *testing.T) {
	testCases := []struct {
		name                    string
		callbackTradeNo         bool
		callbackProviderOrderId string
		callbackAmount          string
		callbackCurrency        string
		expectSuccess           bool
	}{
		{name: "matching snapshot", callbackTradeNo: true, callbackProviderOrderId: "provider-strict", callbackAmount: "10.00000000", callbackCurrency: "USDT", expectSuccess: true},
		{name: "provider order fallback", callbackTradeNo: false, callbackProviderOrderId: "provider-strict", callbackAmount: "10.00000000", callbackCurrency: "USDT", expectSuccess: true},
		{name: "amount mismatch", callbackTradeNo: true, callbackProviderOrderId: "provider-strict", callbackAmount: "9.99999999", callbackCurrency: "USDT", expectSuccess: false},
		{name: "currency mismatch", callbackTradeNo: true, callbackProviderOrderId: "provider-strict", callbackAmount: "10.00000000", callbackCurrency: "TRX", expectSuccess: false},
		{name: "provider order mismatch", callbackTradeNo: true, callbackProviderOrderId: "provider-other", callbackAmount: "10.00000000", callbackCurrency: "USDT", expectSuccess: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupSubscriptionPaymentControllerTestDB(t)
			plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
			order := &model.SubscriptionOrder{
				UserId:           901,
				PlanId:           plan.Id,
				Money:            72,
				TradeNo:          "OKPAY_STRICT_" + strings.ReplaceAll(testCase.name, " ", "_"),
				PaymentMethod:    model.PaymentMethodOkpay,
				PaymentProvider:  model.PaymentProviderOkpay,
				ProviderOrderId:  "provider-strict",
				ProviderAmount:   "10.00000000",
				ProviderCurrency: "USDT",
				CreateTime:       common.GetTimestamp(),
				Status:           common.TopUpStatusPending,
			}
			require.NoError(t, order.Insert())

			originalMerchantID := setting.OkpayMerchantId
			originalToken := setting.OkpayMerchantToken
			setting.OkpayMerchantId = "merchant-strict"
			setting.OkpayMerchantToken = "strict-secret"
			t.Cleanup(func() {
				setting.OkpayMerchantId = originalMerchantID
				setting.OkpayMerchantToken = originalToken
			})

			callbackTradeNo := ""
			if testCase.callbackTradeNo {
				callbackTradeNo = order.TradeNo
			}
			body := buildSignedOkpayJSONCallbackForTest(
				callbackTradeNo,
				testCase.callbackProviderOrderId,
				testCase.callbackAmount,
				testCase.callbackCurrency,
				setting.OkpayMerchantId,
				setting.OkpayMerchantToken,
			)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/okpay/notify", strings.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			OkpayNotify(ctx)

			assert.Equal(t, http.StatusOK, recorder.Code)
			if testCase.expectSuccess {
				assert.JSONEq(t, `{"status":"success"}`, recorder.Body.String())
			} else {
				assert.JSONEq(t, `{"status":"fail"}`, recorder.Body.String())
			}

			var savedOrder model.SubscriptionOrder
			require.NoError(t, db.Where("trade_no = ?", order.TradeNo).First(&savedOrder).Error)
			var subCount int64
			require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", 901, plan.Id).Count(&subCount).Error)
			if testCase.expectSuccess {
				assert.Equal(t, common.TopUpStatusSuccess, savedOrder.Status)
				assert.EqualValues(t, 1, subCount)
			} else {
				assert.Equal(t, common.TopUpStatusPending, savedOrder.Status)
				assert.EqualValues(t, 0, subCount)
			}
		})
	}
}

func TestBepusdtWebhookRejectsMismatchedSubscriptionAmount(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	plan := seedSubscriptionPaymentUserAndPlan(t, db, nil)
	order := &model.SubscriptionOrder{
		UserId:          901,
		PlanId:          plan.Id,
		Money:           19.99,
		TradeNo:         "BEPUSDT_SUB_AMOUNT_MISMATCH",
		PaymentMethod:   model.PaymentMethodBepusdt,
		PaymentProvider: model.PaymentProviderBepusdt,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/bepusdt/notify", nil)
	handleBepusdtPaymentSuccess(ctx, &bepusdtNotifyPayload{
		OrderId: order.TradeNo,
		TradeId: "trade_amount_mismatch",
		Amount:  "9.99",
		Status:  2,
	})

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var savedOrder model.SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", order.TradeNo).First(&savedOrder).Error)
	assert.Equal(t, common.TopUpStatusPending, savedOrder.Status)
	var subCount int64
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", 901, plan.Id).Count(&subCount).Error)
	assert.EqualValues(t, 0, subCount)
}

func TestCryptoWebhookAvailabilityKeepsExistingOrdersReachable(t *testing.T) {
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalBepusdtToken := setting.BepusdtAuthToken
	originalBepusdtChains := setting.BepusdtChains
	originalOkpayGateway := setting.OkpayGatewayUrl
	originalOkpayMerchantID := setting.OkpayMerchantId
	originalOkpayToken := setting.OkpayMerchantToken
	paymentSetting.ComplianceConfirmed = false
	setting.BepusdtAuthToken = "existing-bepusdt-secret"
	setting.BepusdtChains = "[]"
	setting.OkpayGatewayUrl = ""
	setting.OkpayMerchantId = "existing-okpay-merchant"
	setting.OkpayMerchantToken = "existing-okpay-secret"
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		setting.BepusdtAuthToken = originalBepusdtToken
		setting.BepusdtChains = originalBepusdtChains
		setting.OkpayGatewayUrl = originalOkpayGateway
		setting.OkpayMerchantId = originalOkpayMerchantID
		setting.OkpayMerchantToken = originalOkpayToken
	})

	assert.True(t, isBepusdtWebhookEnabled())
	assert.True(t, isOkpayWebhookEnabled())
}
