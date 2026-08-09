package hailuo

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestEstimateBillingUsesEffectiveDurationAndResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{Duration: 10, Size: "1920x1080"})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "MiniMax-Hailuo-02"},
		PriceData: types.PriceData{
			UsePrice:       true,
			ModelPriceUnit: types.ModelPriceUnitSecond,
		},
	}
	adaptor := &TaskAdaptor{}
	if got := adaptor.EstimateBilling(c, info)["seconds"]; got != 10 {
		t.Fatalf("seconds = %v, want 10", got)
	}
	if got := adaptor.EstimateTaskBillingSpec(c, info).Dimensions["resolution"]; got != "1080p" {
		t.Fatalf("resolution = %q, want 1080p", got)
	}
}
