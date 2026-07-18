package relay

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func TestRecalcQuotaFromRatiosSkipsSecondsForPerRequestPrice(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota:          560,
			UsePrice:       true,
			ModelPriceUnit: types.ModelPriceUnitRequest,
			OtherRatios: map[string]float64{
				"seconds":    8,
				"resolution": 1.4,
			},
		},
	}

	got := recalcQuotaFromRatios(info, map[string]float64{
		"seconds":    10,
		"resolution": 1.4,
	})
	if got != 560 {
		t.Fatalf("recalcQuotaFromRatios() = %d, want 560", got)
	}
}

func TestRecalcQuotaFromRatiosUsesSecondsForPerSecondPrice(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota:          4480,
			UsePrice:       true,
			ModelPriceUnit: types.ModelPriceUnitSecond,
			OtherRatios: map[string]float64{
				"seconds":    8,
				"resolution": 1.4,
			},
		},
	}

	got := recalcQuotaFromRatios(info, map[string]float64{
		"seconds":    10,
		"resolution": 1.4,
	})
	if got != 5600 {
		t.Fatalf("recalcQuotaFromRatios() = %d, want 5600", got)
	}
}
