package dto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageRequestTokenMetaKeepsCountAsIndependentBillingRatio(t *testing.T) {
	n := uint(3)
	request := ImageRequest{
		Model:   "dall-e-3",
		Prompt:  "draw",
		N:       &n,
		Size:    "1024x1792",
		Quality: "hd",
	}

	meta := request.GetTokenCountMeta()

	require.Equal(t, float64(3), meta.ImagePriceRatio)
	require.Equal(t, float64(3), meta.BillingRatios["n"])
}
