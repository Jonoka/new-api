package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCompletionRatioInfoGPT56UsesOfficialLockedRatio(t *testing.T) {
	for _, modelName := range []string{
		"gpt-5.6-luna",
		"gpt-5.6-terra",
		"gpt-5.6-sol",
	} {
		t.Run(modelName, func(t *testing.T) {
			info := GetCompletionRatioInfo(modelName)
			require.Equal(t, 6.0, info.Ratio)
			require.True(t, info.Locked)
		})
	}
}

func TestGetCompletionRatioInfoGPT55RemainsSix(t *testing.T) {
	info := GetCompletionRatioInfo("gpt-5.5")
	require.Equal(t, 6.0, info.Ratio)
	require.True(t, info.Locked)
}
