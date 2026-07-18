package taskcommon

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestNormalizeVideoResolution(t *testing.T) {
	tests := map[string]string{
		"SD":        "480p",
		"1280x720":  "720p",
		"720x1280":  "720p",
		"full_hd":   "1080p",
		"2560x1440": "2k",
		"4K":        "4k",
		"16:9":      "",
	}
	for input, want := range tests {
		require.Equal(t, want, NormalizeVideoResolution(input), input)
	}
}

func TestAdjustPerSecondBillingOnCompleteUsesActualDurationAndFrozenRatios(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	task := &model.Task{PrivateData: model.TaskPrivateData{
		BillingContext: &model.TaskBillingContext{
			ModelPrice:     0.06,
			ModelPriceUnit: types.ModelPriceUnitSecond,
			GroupRatio:     2,
			OtherRatios: map[string]float64{
				"seconds":          5,
				"resolution-1080p": 1.5,
			},
		},
	}}

	quota := AdjustPerSecondBillingOnComplete(task, &relaycommon.TaskInfo{DurationSeconds: 8})
	require.Equal(t, 720_000, quota)
	require.Equal(t, float64(8), task.PrivateData.BillingContext.OtherRatios["seconds"])
}

func TestAdjustPerSecondBillingOnCompleteSkipsPerRequestPrice(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{
		BillingContext: &model.TaskBillingContext{
			ModelPrice:     0.06,
			ModelPriceUnit: types.ModelPriceUnitRequest,
		},
	}}
	require.Zero(t, AdjustPerSecondBillingOnComplete(task, &relaycommon.TaskInfo{DurationSeconds: 8}))
}
