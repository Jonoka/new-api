package kling

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestEstimateBillingUsesEffectiveDurationAndQuality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{Duration: 10, Mode: "PRO"})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "kling-v2-master"},
		PriceData: types.PriceData{
			UsePrice:       true,
			ModelPriceUnit: types.ModelPriceUnitSecond,
		},
	}
	adaptor := &TaskAdaptor{}
	if got := adaptor.EstimateBilling(c, info)["seconds"]; got != 10 {
		t.Fatalf("seconds = %v, want 10", got)
	}
	if got := adaptor.EstimateTaskBillingSpec(c, info).Dimensions["quality"]; got != "pro" {
		t.Fatalf("quality = %q, want pro", got)
	}
}
