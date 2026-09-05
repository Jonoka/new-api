package relay

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildAlphaSearchRequestBodyPreservesOriginalBytesWithoutMapping(t *testing.T) {
	raw := []byte("{\n \"model\" : \"gpt-5.6\", \"input\":\"keep\\\\n\", \"large\":9007199254740993, \"nil\":null, \"zero\":0, \"flag\":false\n}")
	out, err := buildAlphaSearchRequestBody(raw, "gpt-5.6")
	require.NoError(t, err)
	require.Equal(t, raw, out)
}

func TestBuildAlphaSearchRequestBodyChangesOnlyModel(t *testing.T) {
	raw := []byte(`{"model":"public","input":"keep","large":9007199254740993,"nil":null,"zero":0,"flag":false,"store":true,"future":{"escaped":"a\\tb"}}`)
	original := append([]byte(nil), raw...)
	out, err := buildAlphaSearchRequestBody(raw, "upstream")
	require.NoError(t, err)
	require.Contains(t, string(out), `"model":"upstream"`)
	require.Contains(t, string(out), `"large":9007199254740993`)
	require.Contains(t, string(out), `"nil":null`)
	require.Contains(t, string(out), `"zero":0`)
	require.Contains(t, string(out), `"flag":false`)
	require.Contains(t, string(out), `"store":true`)
	require.Contains(t, string(out), `"escaped":"a\\tb"`)
	require.Equal(t, original, raw)
}

func TestAlphaSearchMappingRebuildsEachAttemptFromOriginalBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	request := &dto.AlphaSearchRequest{
		Model:   "public",
		RawBody: []byte(`{"model":"public","attempt":"original","large":9007199254740993}`),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "public",
		RelayMode:       relayconstant.RelayModeAlphaSearch,
		Request:         request,
	}

	c.Set("model_mapping", `{"public":"mapped-a"}`)
	info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "public"}
	require.NoError(t, helper.ModelMappedHelper(c, info, request))
	first, err := buildAlphaSearchRequestBody(request.RawBody, info.UpstreamModelName)
	require.NoError(t, err)
	require.Contains(t, string(first), `"model":"mapped-a"`)

	c.Set("model_mapping", `{"public":"mapped-b"}`)
	info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "public"}
	require.NoError(t, helper.ModelMappedHelper(c, info, request))
	second, err := buildAlphaSearchRequestBody(request.RawBody, info.UpstreamModelName)
	require.NoError(t, err)
	require.Contains(t, string(second), `"model":"mapped-b"`)
	require.NotContains(t, string(second), "mapped-a")
	require.Contains(t, string(second), `"attempt":"original"`)
	require.Contains(t, string(second), `"large":9007199254740993`)
}

func TestValidateAlphaSearchMappedModelRejectsConflictingOverride(t *testing.T) {
	require.NoError(t, validateAlphaSearchMappedModel([]byte(`{"model":"mapped","future":true}`), "mapped"))
	require.NoError(t, validateAlphaSearchMappedModel([]byte(`{"model":"mapped","stream":false}`), "mapped"))
	for _, body := range []string{
		`{"model":"other"}`,
		`{"model":57}`,
		`{"future":true}`,
		`{"model":null}`,
		`{"model":"mapped","stream":true}`,
		`{"model":"mapped","stream":"false"}`,
		`{"model":"mapped","stream":null}`,
	} {
		require.Error(t, validateAlphaSearchMappedModel([]byte(body), "mapped"))
	}
}

func TestValidateAlphaSearchResponseContract(t *testing.T) {
	valid := []string{
		`{"output":""}`,
		`{"output":"result","encrypted_output":"cipher"}`,
		`{"output":"result","encrypted_output":"","future":{"large":9007199254740993}}`,
		`{"output":"result","error":null}`,
		`{"output":"result","error":false}`,
		`{"output":"result","error":[]}`,
		`{"output":"result","error":"future field"}`,
	}
	for _, body := range valid {
		require.NoError(t, validateAlphaSearchResponse([]byte(body)), body)
	}

	invalid := []string{
		``,
		`<html>ok</html>`,
		`[]`,
		`{"output":`,
		`{"result":"missing output"}`,
		`{"output":null}`,
		`{"output":57}`,
		`{"output":"ok","encrypted_output":null}`,
		`{"output":"ok","encrypted_output":57}`,
		`{"error":{"message":"failed"}}`,
		`{"output":"ok","error":{"message":"failed"}}`,
	}
	for _, body := range invalid {
		require.Error(t, validateAlphaSearchResponse([]byte(body)), body)
	}
}

func TestReadBoundedAlphaSearchResponse(t *testing.T) {
	want := []byte(`{"output":"ok","future":true}`)
	got, err := readBoundedAlphaSearchResponse(bytes.NewReader(want))
	require.NoError(t, err)
	require.Equal(t, want, got)

	_, err = readBoundedAlphaSearchResponse(strings.NewReader(strings.Repeat("x", int(maxAlphaSearchResponseBytes)+1)))
	require.Error(t, err)

	_, err = readBoundedAlphaSearchResponse(errorReader{})
	require.Error(t, err)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
