package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func alphaSearchRequestContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	return c
}

func TestGetAndValidateAlphaSearchRequestPreservesRawJSON(t *testing.T) {
	raw := "{\n  \"model\": \"gpt-5.6\", \"stream\": false, \"large\": 9007199254740993, \"nil\": null, \"zero\": 0, \"flag\": false\n}"
	request, err := GetAndValidateRequest(alphaSearchRequestContext(t, raw), types.RelayFormatOpenAIAlphaSearch)
	require.NoError(t, err)
	alpha, ok := request.(*dto.AlphaSearchRequest)
	require.True(t, ok)
	require.Equal(t, "gpt-5.6", alpha.Model)
	require.NotNil(t, alpha.Stream)
	require.False(t, *alpha.Stream)
	require.Equal(t, raw, string(alpha.RawBody))
}

func TestGetAndValidateAlphaSearchRequestRejectsInvalidShapes(t *testing.T) {
	tests := map[string]string{
		"malformed":           `{"model":`,
		"null body":           `null`,
		"array body":          `[{"model":"gpt-5.6"}]`,
		"missing model":       `{}`,
		"empty model":         `{"model":"   "}`,
		"numeric model":       `{"model":57}`,
		"null model":          `{"model":null}`,
		"object model":        `{"model":{}}`,
		"stream true":         `{"model":"gpt-5.6","stream":true}`,
		"stream string":       `{"model":"gpt-5.6","stream":"false"}`,
		"stream number":       `{"model":"gpt-5.6","stream":0}`,
		"stream null":         `{"model":"gpt-5.6","stream":null}`,
		"stream object":       `{"model":"gpt-5.6","stream":{}}`,
		"trailing JSON value": `{"model":"gpt-5.6"}{}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := GetAndValidateRequest(alphaSearchRequestContext(t, body), types.RelayFormatOpenAIAlphaSearch)
			require.Error(t, err)
		})
	}
}
