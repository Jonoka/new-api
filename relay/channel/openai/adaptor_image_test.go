package openai

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertImageGenerationRequestPreservesGPTImage2Size(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "生成一张方形图片",
		Size:   "1024x1024",
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(ctx, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}, request)
	require.NoError(t, err)

	body, err := common.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-image-2","prompt":"生成一张方形图片","size":"1024x1024"}`, string(body))
}
