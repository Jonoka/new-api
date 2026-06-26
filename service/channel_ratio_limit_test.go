package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCheckTokenGroupRatioLimitRejectsRatioAboveLimit(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenGroupRatioLimits, map[string]float64{
		"default": 0.5,
	})

	err := CheckTokenGroupRatioLimit(ctx, "default", "default")

	require.Error(t, err)
	require.Contains(t, err.Error(), "已超过令牌倍率保护")
}

func TestCheckTokenGroupRatioLimitAllowsRatioWithinLimit(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenGroupRatioLimits, map[string]float64{
		"default": 1,
	})

	err := CheckTokenGroupRatioLimit(ctx, "default", "default")

	require.NoError(t, err)
}
