package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type realtimeReservationRecorder struct {
	targets []int
	failure error
}

func (r *realtimeReservationRecorder) Reserve(target int) error {
	r.targets = append(r.targets, target)
	if len(r.targets) == 2 {
		return r.failure
	}
	return nil
}
func (*realtimeReservationRecorder) Settle(int) error         { return nil }
func (*realtimeReservationRecorder) Refund(*gin.Context)      {}
func (*realtimeReservationRecorder) NeedsRefund() bool        { return false }
func (*realtimeReservationRecorder) GetPreConsumedQuota() int { return 0 }

func TestRealtimeResponseCyclesReserveCumulativeUsageAndPropagateFailure(t *testing.T) {
	for _, failIncrease := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "increase_rejected"}[failIncrease], func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
			billing := &realtimeReservationRecorder{}
			if failIncrease {
				billing.failure = errors.New("insufficient cumulative reservation")
			}
			info := &relaycommon.RelayInfo{OriginModelName: "realtime-cumulative-fixture", UsingGroup: "default", Billing: billing,
				PriceData: types.PriceData{ModelRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}}
			total := &dto.RealtimeUsage{}
			first := &dto.RealtimeUsage{TotalTokens: 10, InputTokens: 10}
			first.InputTokenDetails.TextTokens = 10
			require.NoError(t, preConsumeUsage(c, info, first, total))
			second := &dto.RealtimeUsage{TotalTokens: 20, InputTokens: 20}
			second.InputTokenDetails.TextTokens = 20
			err := preConsumeUsage(c, info, second, total)
			if failIncrease {
				require.ErrorIs(t, err, billing.failure)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, []int{10, 30}, billing.targets)
			require.Equal(t, 30, total.InputTokenDetails.TextTokens, "observed usage remains available to final settlement even when an increase fails")
		})
	}
}
