package jimeng

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestEstimateBillingUsesGeneratedFrameDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{Duration: 10})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "jimeng_v30_pro"},
		PriceData: types.PriceData{
			UsePrice:       true,
			ModelPriceUnit: types.ModelPriceUnitSecond,
		},
	}
	if got := (&TaskAdaptor{}).EstimateBilling(c, info)["seconds"]; got != 10 {
		t.Fatalf("seconds = %v, want 10", got)
	}
}
