package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func signedBepusdtNotifyValues(params map[string]string, authToken string) url.Values {
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	values.Set("signature", generateBepusdtSignature(params, authToken))
	return values
}

func newBepusdtNotifyContext(method string, target string, body string, contentType string) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		ctx.Request.Header.Set("Content-Type", contentType)
	}
	return ctx
}

func TestParseBepusdtNotifyPayloadSupportsGetQuery(t *testing.T) {
	authToken := "bepusdt-token"
	values := signedBepusdtNotifyValues(map[string]string{
		"trade_id":      "trade_123",
		"order_id":      "USR1NOabc",
		"amount":        "10.00000000",
		"actual_amount": "10.00000000",
		"token":         "USDT",
		"status":        "2",
	}, authToken)
	ctx := newBepusdtNotifyContext(http.MethodGet, "/api/bepusdt/notify?"+values.Encode(), "", "")

	parsed, err := parseBepusdtNotifyPayload(ctx)

	require.NoError(t, err)
	require.Equal(t, "USR1NOabc", parsed.Payload.OrderId)
	require.Equal(t, 2, parsed.Payload.Status)
	require.True(t, verifyBepusdtNotifyParamsSignature(parsed.Params, authToken))
}

func TestParseBepusdtNotifyPayloadSupportsPostForm(t *testing.T) {
	authToken := "bepusdt-token"
	values := signedBepusdtNotifyValues(map[string]string{
		"trade_id":             "trade_456",
		"order_id":             "USR2NOabc",
		"amount":               "9.99",
		"actual_amount":        "9.99",
		"block_transaction_id": "0xabc",
		"status":               "2",
	}, authToken)
	ctx := newBepusdtNotifyContext(http.MethodPost, "/api/bepusdt/notify", values.Encode(), "application/x-www-form-urlencoded")

	parsed, err := parseBepusdtNotifyPayload(ctx)

	require.NoError(t, err)
	require.Equal(t, "0xabc", parsed.Payload.BlockTransactionId)
	require.Equal(t, "9.99", parsed.Params["amount"])
	require.True(t, verifyBepusdtNotifyParamsSignature(parsed.Params, authToken))
}

func TestParseBepusdtNotifyPayloadKeepsJSONNumberTextForSignature(t *testing.T) {
	authToken := "bepusdt-token"
	params := map[string]string{
		"trade_id":      "trade_789",
		"order_id":      "USR3NOabc",
		"amount":        "10.00",
		"actual_amount": "10.00",
		"status":        "2",
	}
	signature := generateBepusdtSignature(params, authToken)
	body := `{"trade_id":"trade_789","order_id":"USR3NOabc","amount":10.00,"actual_amount":10.00,"status":2,"signature":"` + signature + `"}`
	ctx := newBepusdtNotifyContext(http.MethodPost, "/api/bepusdt/notify", body, "application/json")

	parsed, err := parseBepusdtNotifyPayload(ctx)

	require.NoError(t, err)
	require.Equal(t, "10.00", parsed.Params["amount"])
	require.Equal(t, "10.00", parsed.Params["actual_amount"])
	require.True(t, verifyBepusdtNotifyParamsSignature(parsed.Params, authToken))
}
