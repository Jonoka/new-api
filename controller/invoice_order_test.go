package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupInvoiceOrderControllerTest(t *testing.T) {
	t.Helper()
	db := setupSubscriptionPaymentControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.InvoiceRecord{}, &model.InvoiceOrderLink{}))

	originalEnabled := model.InvoiceEnabled
	originalRules := model.InvoiceFeeRules
	originalTypes := model.InvoiceTypes
	originalPrice := operation_setting.Price
	originalQuotaPerUnit := common.QuotaPerUnit
	model.InvoiceEnabled = true
	model.InvoiceFeeRules = `[{"min":0,"type":"percent","value":10}]`
	model.InvoiceTypes = `["personal","company"]`
	operation_setting.Price = 7
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		model.InvoiceEnabled = originalEnabled
		model.InvoiceFeeRules = originalRules
		model.InvoiceTypes = originalTypes
		operation_setting.Price = originalPrice
		common.QuotaPerUnit = originalQuotaPerUnit
	})
}

func invoiceOrderControllerContext(t *testing.T, body string, userId int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/invoice/request", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userId)
	return ctx, recorder
}

func TestApplyInvoiceOrdersUsesAuthenticatedUserAndForcesRequired(t *testing.T) {
	setupInvoiceOrderControllerTest(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 1201, Username: "invoice-controller", Quota: 1_000_000}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          1201,
		TradeNo:         "TOP-CONTROLLER",
		Money:           70,
		ActualMoney:     70,
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 30,
		CompleteTime:    now - 20,
	}).Error)

	ctx, recorder := invoiceOrderControllerContext(t, `{
		"orders":[{"source_type":"topup","source_id":"TOP-CONTROLLER"}],
		"invoice":{"type":"company","title":"控制器测试公司","tax_no":"91310000CTRL"}
	}`, 1201)
	ApplyInvoiceOrders(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			SourceType  string  `json:"source_type"`
			BaseAmount  float64 `json:"base_amount"`
			FeeAmount   float64 `json:"fee_amount"`
			TotalAmount float64 `json:"total_amount"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, model.InvoiceSourceCombined, response.Data.SourceType)
	assert.Equal(t, 70.0, response.Data.BaseAmount)
	assert.Equal(t, 7.0, response.Data.FeeAmount)
	assert.Equal(t, 77.0, response.Data.TotalAmount)

	var saved model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", "TOP-CONTROLLER").First(&saved).Error)
	assert.True(t, saved.InvoiceRequired)
}

func TestPreviewInvoiceOrdersReturnsFrontendFieldNames(t *testing.T) {
	setupInvoiceOrderControllerTest(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 1202, Username: "invoice-preview", Quota: 1_000_000}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          1202,
		TradeNo:         "TOP-PREVIEW",
		Money:           70,
		ActualMoney:     70,
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 30,
		CompleteTime:    now - 20,
	}).Error)

	ctx, recorder := invoiceOrderControllerContext(t, `{
		"orders":[{"source_type":"topup","source_id":"TOP-PREVIEW"}],
		"invoice":{"required":true,"type":"personal"}
	}`, 1202)
	PreviewInvoiceOrders(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"order_amount":70`)
	assert.Contains(t, recorder.Body.String(), `"invoice_fee":7`)
	assert.Contains(t, recorder.Body.String(), `"invoice_total_amount":77`)
}
