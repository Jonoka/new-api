package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestAlphaSearchChannelTestUsesValidatedSyncTransportWithoutDebit(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeCodex} {
		t.Run(constant.GetChannelTypeName(channelType), func(t *testing.T) {
			db := setupAlphaSearchRelayDB(t)
			user, _ := seedFinalGroupRelayFunding(t, db, 1, 100000)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if channelType == constant.ChannelTypeCodex {
					require.Equal(t, "/backend-api/codex/alpha/search", r.URL.Path)
					require.Equal(t, "Bearer test-access", r.Header.Get("Authorization"))
					require.Equal(t, "test-account", r.Header.Get("Chatgpt-Account-Id"))
				} else {
					require.Equal(t, "/v1/alpha/search", r.URL.Path)
					require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				}
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Contains(t, string(body), `"model":"probe-mapped"`)
				require.Contains(t, string(body), `"search_query"`)
				require.NotContains(t, string(body), `"instructions"`)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"output":""}`)
			}))
			t.Cleanup(server.Close)
			channel := seedFinalGroupRelayChannel(t, db, "alpha-probe", "default", server.URL, 100)
			channel.Type = channelType
			channel.ModelMapping = common.GetPointer(`{"fg-relay-model":"probe-mapped"}`)
			if channelType == constant.ChannelTypeCodex {
				channel.Key = `{"access_token":"test-access","account_id":"test-account"}`
			}
			result := testChannel(channel, user.Id, finalGroupRelayModel, string(constant.EndpointTypeOpenAIAlphaSearch), false)
			require.NoError(t, result.localErr)
			require.Nil(t, result.newAPIError)
			require.EqualValues(t, 1, calls.Load())
			var stored model.User
			require.NoError(t, db.First(&stored, user.Id).Error)
			require.Equal(t, 100000, stored.Quota)
			require.Zero(t, stored.UsedQuota)
			require.Zero(t, stored.RequestCount)
			result = testChannel(channel, user.Id, finalGroupRelayModel, string(constant.EndpointTypeOpenAIAlphaSearch), true)
			require.Error(t, result.localErr)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
