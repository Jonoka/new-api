package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupContextForSelectedChannelReleasesConcurrencyWhenKeySelectionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	limit := 1
	channel := &model.Channel{
		Id:               990001,
		Key:              "disabled-key",
		ConcurrencyLimit: &limit,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled},
			MultiKeyMode:       constant.MultiKeyModeRandom,
		},
	}
	t.Cleanup(func() {
		model.ReleaseChannelConcurrency(channel.Id)
	})

	err := SetupContextForSelectedChannel(c, channel, "test-model")

	require.NotNil(t, err)
	require.True(t, model.IsChannelConcurrencyAvailable(channel))
}
