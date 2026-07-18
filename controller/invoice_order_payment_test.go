package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func invoiceConfigResponseForTest(t *testing.T) struct {
	Success bool `json:"success"`
	Data    struct {
		PayMethods    []map[string]string    `json:"pay_methods"`
		BepusdtChains []setting.BepusdtChain `json:"bepusdt_chains"`
	} `json:"data"`
} {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/invoice/config", nil)
	GetInvoiceConfig(ctx)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			PayMethods    []map[string]string    `json:"pay_methods"`
			BepusdtChains []setting.BepusdtChain `json:"bepusdt_chains"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func invoicePaymentMethodTypes(methods []map[string]string) []string {
	types := make([]string, 0, len(methods))
	for _, method := range methods {
		types = append(types, method["type"])
	}
	return types
}

func TestGetInvoiceConfigFiltersExternalPaymentMethodsByComplianceAndAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTerms := paymentSetting.ComplianceTermsVersion
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalMethods := operation_setting.PayMethods
	originalBepURL := setting.BepusdtApiUrl
	originalBepToken := setting.BepusdtAuthToken
	originalBepChains := setting.BepusdtChains
	originalOKURL := setting.OkpayGatewayUrl
	originalOKID := setting.OkpayMerchantId
	originalOKToken := setting.OkpayMerchantToken
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTerms
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalMethods
		setting.BepusdtApiUrl = originalBepURL
		setting.BepusdtAuthToken = originalBepToken
		setting.BepusdtChains = originalBepChains
		setting.OkpayGatewayUrl = originalOKURL
		setting.OkpayMerchantId = originalOKID
		setting.OkpayMerchantToken = originalOKToken
	})

	operation_setting.PayAddress = "https://epay.example.com"
	operation_setting.EpayId = "epay-id"
	operation_setting.EpayKey = "epay-key"
	operation_setting.PayMethods = []map[string]string{
		{"name": "支付宝", "type": "alipay", "color": "blue"},
		{"name": "微信", "type": "wxpay", "color": "green"},
	}
	setting.BepusdtApiUrl = "https://bep.example.com"
	setting.BepusdtAuthToken = "bep-token"
	setting.BepusdtChains = `[{"name":"TRC20","trade_type":"usdt.trc20"}]`
	setting.OkpayGatewayUrl = "https://okpay.example.com"
	setting.OkpayMerchantId = "okpay-id"
	setting.OkpayMerchantToken = "okpay-token"

	paymentSetting.ComplianceConfirmed = false
	paymentSetting.ComplianceTermsVersion = ""
	response := invoiceConfigResponseForTest(t)
	require.True(t, response.Success)
	assert.Equal(t, []string{model.PaymentMethodBalance}, invoicePaymentMethodTypes(response.Data.PayMethods))
	assert.Empty(t, response.Data.BepusdtChains)

	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	response = invoiceConfigResponseForTest(t)
	require.True(t, response.Success)
	assert.ElementsMatch(t, []string{
		model.PaymentMethodBalance, "alipay", "wxpay", model.PaymentMethodBepusdt, model.PaymentMethodOkpay,
	}, invoicePaymentMethodTypes(response.Data.PayMethods))
	require.Len(t, response.Data.BepusdtChains, 1)
	assert.Equal(t, "usdt.trc20", response.Data.BepusdtChains[0].TradeType)
	for _, method := range response.Data.PayMethods {
		assert.NotEmpty(t, method["name"])
		assert.NotEmpty(t, method["type"])
		assert.NotEmpty(t, method["provider"])
		assert.NotEmpty(t, method["color"])
	}

	operation_setting.EpayKey = ""
	setting.BepusdtAuthToken = ""
	setting.OkpayMerchantToken = ""
	response = invoiceConfigResponseForTest(t)
	assert.Equal(t, []string{model.PaymentMethodBalance}, invoicePaymentMethodTypes(response.Data.PayMethods))
	assert.Empty(t, response.Data.BepusdtChains)
}
