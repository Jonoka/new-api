package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminDeleteInvoicesReturnsActualDeletedCount(t *testing.T) {
	setupInvoiceOrderControllerTest(t)
	record := model.InvoiceRecord{
		UserId:        2201,
		SourceType:    model.InvoiceSourceTopUp,
		SourceId:      "TOP-CONTROLLER-INVOICE-DELETE",
		PaymentStatus: model.InvoicePaymentStatusSuccess,
		Status:        model.InvoiceStatusIssued,
	}
	require.NoError(t, model.DB.Create(&record).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/invoice/batch",
		bytes.NewBufferString(`{"ids":[`+jsonIntForTest(record.Id)+`,`+jsonIntForTest(record.Id+999)+`]}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	AdminDeleteInvoices(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"data":1`)
}

func jsonIntForTest(value int) string {
	return fmt.Sprintf("%d", value)
}
