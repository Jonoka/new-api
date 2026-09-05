package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAlphaSearchRelayDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupFinalGroupRelayDB(t)
	require.NoError(t, db.AutoMigrate(&model.TaskAccountingEvent{}, &model.TaskAccountingLogReceipt{}))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"tool_price_setting.prices": `{"web_search_preview":10}`,
	}))
	return db
}

func setAlphaSearchRelayBody(c *gin.Context, body string) {
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
}

type alphaSearchCommitWriter struct {
	gin.ResponseWriter
	beforeWrite func()
}

func (w *alphaSearchCommitWriter) Write(body []byte) (int, error) {
	if w.Status() >= 200 && w.Status() < 300 {
		w.beforeWrite()
	}
	return w.ResponseWriter.Write(body)
}

func TestAlphaSearchRelayFinalGroupAccounting(t *testing.T) {
	for _, tc := range []struct {
		name, firstGroup, secondGroup                        string
		firstStatus, secondStatus, quota, finalQuota, status int
		firstCalls, secondCalls                              int32
	}{
		{"tiered_model_success", "fg-paid-b", "", 200, 0, 100000, 5000, 200, 1, 0},
		{"free_to_paid", "fg-free-a", "fg-paid-b", 503, 200, 100000, 5000, 200, 1, 1},
		{"paid_to_free", "fg-paid-a", "fg-free-b", 503, 200, 100000, 0, 200, 1, 1},
		{"paid_to_paid", "fg-paid-b", "fg-paid-a", 503, 200, 100000, 10000, 200, 1, 1},
		{"all_failed", "fg-paid-b", "fg-paid-a", 503, 503, 100000, 0, 503, 1, 1},
		{"insufficient", "fg-paid-b", "", 200, 0, 100, 0, 403, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAlphaSearchRelayDB(t)
			user, token := seedFinalGroupRelayFunding(t, db, 1, tc.quota)
			var firstCalls, secondCalls atomic.Int32
			response := `{"output":"","encrypted_output":"opaque","unknown":{"large":9007199254740993}}`
			upstream := func(status int, counter *atomic.Int32) *httptest.Server {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					counter.Add(1)
					require.Equal(t, "/v1/alpha/search", r.URL.Path)
					require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.Contains(t, string(body), `9007199254740993`)
					require.NotContains(t, string(body), `"instructions"`)
					w.Header().Set("Content-Type", "text/html")
					w.WriteHeader(status)
					if status == 200 {
						_, _ = io.WriteString(w, response)
					} else {
						_, _ = io.WriteString(w, `{"error":{"message":"capacity fixture","type":"server_error"}}`)
					}
				}))
				t.Cleanup(server.Close)
				return server
			}
			firstServer := upstream(tc.firstStatus, &firstCalls)
			first := seedFinalGroupRelayChannel(t, db, "alpha-first", tc.firstGroup, firstServer.URL, 100)
			finalChannel := first
			tokenGroup := tc.firstGroup
			if tc.secondGroup != "" {
				secondServer := upstream(tc.secondStatus, &secondCalls)
				finalChannel = seedFinalGroupRelayChannel(t, db, "alpha-second", tc.secondGroup, secondServer.URL+"/v1", 100)
				t.Cleanup(func() { model.ReleaseChannelConcurrency(finalChannel.Id) })
				tokenGroup += "," + tc.secondGroup
			}
			requestID := "alpha-final-" + tc.name
			c, recorder := finalGroupRelayContext(t, user, token, first, tc.firstGroup, tokenGroup, requestID)
			setAlphaSearchRelayBody(c, `{"model":"fg-relay-model","input":"query","opaque":{"large":9007199254740993,"flag":false,"zero":0,"null":null}}`)
			published := false
			c.Writer = &alphaSearchCommitWriter{ResponseWriter: c.Writer, beforeWrite: func() {
				published = true
				var current model.User
				require.NoError(t, db.First(&current, user.Id).Error)
				require.Equal(t, tc.quota-tc.finalQuota, current.Quota)
				require.Equal(t, 1, current.RequestCount, "success must follow atomic accounting")
			}}
			Relay(c, types.RelayFormatOpenAIAlphaSearch)
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			require.Equal(t, tc.firstCalls, firstCalls.Load())
			require.Equal(t, tc.secondCalls, secondCalls.Load())
			gotUser, gotToken, gotFirst, gotFinal := readFinalGroupRelayState(t, db, user.Id, token.Id, first.Id, finalChannel.Id)
			require.Equal(t, tc.quota-tc.finalQuota, gotUser.Quota)
			require.Equal(t, tc.quota-tc.finalQuota, gotToken.RemainQuota)
			require.Equal(t, tc.finalQuota, gotUser.UsedQuota)
			require.Equal(t, tc.finalQuota, gotToken.UsedQuota)
			require.EqualValues(t, tc.finalQuota, gotFinal.UsedQuota)
			if finalChannel.Id != first.Id {
				require.Zero(t, gotFirst.UsedQuota)
			}
			var logs []model.Log
			require.NoError(t, db.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			if tc.status != 200 {
				require.False(t, published)
				require.Zero(t, gotUser.RequestCount)
				require.Empty(t, logs)
				return
			}
			require.True(t, published)
			require.Equal(t, response, recorder.Body.String())
			require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
			require.Equal(t, 1, gotUser.RequestCount)
			require.Len(t, logs, 1)
			require.Equal(t, finalChannel.Id, logs[0].ChannelId)
			require.Equal(t, tc.finalQuota, logs[0].Quota)
			require.Equal(t, finalGroupRelayModel, logs[0].ModelName)
			require.Zero(t, logs[0].PromptTokens)
			require.Zero(t, logs[0].CompletionTokens)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			require.Equal(t, float64(1), other["web_search_call_count"])
			require.Equal(t, float64(10), other["web_search_price"])
			require.NotContains(t, other, "expr_b64")
		})
	}
}

func TestAlphaSearchRelayRejectsInvalidSuccessWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		truncated  bool
	}{
		{"html", "<html>error</html>", 200, false},
		{"error_object", `{"error":{"message":"error"}}`, 200, false},
		{"empty_204", "", 204, false},
		{"truncated", `{"output":"partial`, 200, true},
		{"redirect", "", 307, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAlphaSearchRelayDB(t)
			user, token := seedFinalGroupRelayFunding(t, db, 1, 100000)
			var firstCalls, nextCalls atomic.Int32
			next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalls.Add(1)
				_, _ = io.WriteString(w, `{"output":"must not retry"}`)
			}))
			t.Cleanup(next.Close)
			firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				firstCalls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				if tc.truncated {
					w.Header().Set("Content-Length", "10000")
				}
				if tc.status == 307 {
					w.Header().Set("Location", next.URL+"/v1/alpha/search")
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(firstServer.Close)
			first := seedFinalGroupRelayChannel(t, db, "alpha-invalid", "fg-paid-b", firstServer.URL, 100)
			second := seedFinalGroupRelayChannel(t, db, "alpha-no-retry", "fg-paid-a", next.URL, 100)
			t.Cleanup(func() { model.ReleaseChannelConcurrency(second.Id) })
			c, recorder := finalGroupRelayContext(t, user, token, first, "fg-paid-b", "fg-paid-b,fg-paid-a", "alpha-invalid-"+tc.name)
			setAlphaSearchRelayBody(c, fmt.Sprintf(`{"model":%q}`, finalGroupRelayModel))
			Relay(c, types.RelayFormatOpenAIAlphaSearch)
			require.GreaterOrEqual(t, recorder.Code, 300, recorder.Body.String())
			require.EqualValues(t, 1, firstCalls.Load())
			require.Zero(t, nextCalls.Load(), "ambiguous execution must not be repeated")
			gotUser, gotToken, _, _ := readFinalGroupRelayState(t, db, user.Id, token.Id, first.Id, second.Id)
			require.Equal(t, 100000, gotUser.Quota)
			require.Equal(t, 100000, gotToken.RemainQuota)
			require.Zero(t, gotUser.RequestCount)
		})
	}
}

type alphaSearchDisconnectWriter struct {
	gin.ResponseWriter
	writes int
}

func (w *alphaSearchDisconnectWriter) Write(body []byte) (int, error) {
	w.writes++
	n := len(body) / 2
	_, _ = w.ResponseWriter.Write(body[:n])
	return n, errors.New("client disconnected")
}

func TestAlphaSearchRelaySettlementAndPublicationFailures(t *testing.T) {
	for _, failSettlement := range []bool{false, true} {
		t.Run(fmt.Sprintf("settlement_failure_%t", failSettlement), func(t *testing.T) {
			db := setupAlphaSearchRelayDB(t)
			user, token := seedFinalGroupRelayFunding(t, db, 1, 100000)
			var calls atomic.Int32
			response := `{"output":"validated search"}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, response)
			}))
			t.Cleanup(server.Close)
			channel := seedFinalGroupRelayChannel(t, db, "alpha-publish", "fg-paid-b", server.URL, 100)
			c, recorder := finalGroupRelayContext(t, user, token, channel, "fg-paid-b", "fg-paid-b", "alpha-publish")
			setAlphaSearchRelayBody(c, `{"model":"fg-relay-model"}`)
			writer := &alphaSearchDisconnectWriter{ResponseWriter: c.Writer}
			if failSettlement {
				callback := "test:alpha-publication-settlement"
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "task_accounting_events" {
						tx.AddError(errors.New("injected settlement error"))
					}
				}))
				t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })
			} else {
				c.Writer = writer
			}
			Relay(c, types.RelayFormatOpenAIAlphaSearch)
			require.EqualValues(t, 1, calls.Load())
			gotUser, gotToken, _, gotChannel := readFinalGroupRelayState(t, db, user.Id, token.Id, channel.Id, channel.Id)
			if failSettlement {
				require.Equal(t, 500, recorder.Code)
				require.NotContains(t, recorder.Body.String(), "validated search")
				require.Equal(t, 100000, gotUser.Quota)
				require.Equal(t, 100000, gotToken.RemainQuota)
				require.Zero(t, gotUser.RequestCount)
				require.Zero(t, gotChannel.UsedQuota)
				return
			}
			require.Equal(t, 1, writer.writes, "no JSON error may be appended to partial output")
			require.Equal(t, response[:len(response)/2], recorder.Body.String())
			require.Equal(t, 95000, gotUser.Quota)
			require.Equal(t, 95000, gotToken.RemainQuota)
			require.Equal(t, 1, gotUser.RequestCount)
			require.EqualValues(t, 5000, gotChannel.UsedQuota)
		})
	}
}
