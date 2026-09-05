package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGenRelayInfoAlphaSearchKeepsPublicIdentityAndSyncMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", nil)
	rootcommon.SetContextKey(c, rootconstant.ContextKeyOriginalModel, "public-model")
	request := &dto.AlphaSearchRequest{Model: "public-model", RawBody: []byte(`{"model":"public-model"}`)}

	info, err := GenRelayInfo(c, types.RelayFormatOpenAIAlphaSearch, request, nil)
	require.NoError(t, err)
	require.Equal(t, relayconstant.RelayModeAlphaSearch, info.RelayMode)
	require.Equal(t, types.RelayFormatOpenAIAlphaSearch, info.RelayFormat)
	require.Equal(t, "public-model", info.OriginModelName)
	require.False(t, info.IsStream)
	require.Equal(t, []types.RelayFormat{types.RelayFormatOpenAIAlphaSearch}, info.RequestConversionChain)
}
