package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// Alpha results are short search artifacts. The 8 MiB cap is intentionally
// much smaller than the existing 128 MiB default request limit while leaving
// ample room for the pinned Codex client's string response fields.
const maxAlphaSearchResponseBytes int64 = 8 << 20

func AlphaSearchHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	info.InitChannelMeta(c)
	if info.ChannelType != constant.ChannelTypeOpenAI && info.ChannelType != constant.ChannelTypeCodex {
		return types.NewErrorWithStatusCode(
			errors.New("channel does not support /v1/alpha/search"),
			types.ErrorCodeInvalidRequest,
			http.StatusBadGateway,
		)
	}

	request, ok := info.Request.(*dto.AlphaSearchRequest)
	if !ok {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected *dto.AlphaSearchRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	jsonData, err := buildAlphaSearchRequestBody(request.RawBody, info.UpstreamModelName)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return newAPIErrorFromParamOverride(err)
		}
	}
	if err := validateAlphaSearchMappedModel(jsonData, info.UpstreamModelName); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeChannelParamOverrideInvalid, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	body, size, factory, owner, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer owner.Close()
	info.UpstreamRequestBodySize = size
	info.UpstreamRequestBodyFactory = factory

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	response, err := adaptor.DoRequest(c, info, body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	httpResponse, ok := response.(*http.Response)
	if !ok || httpResponse == nil {
		return types.NewOpenAIError(errors.New("invalid upstream response"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	if httpResponse.Body == nil {
		return types.NewOpenAIError(errors.New("alpha search upstream response body is missing"), types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	defer httpResponse.Body.Close()
	responseBody, err := readBoundedAlphaSearchResponse(httpResponse.Body)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}

	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		httpResponse.Body = io.NopCloser(bytes.NewReader(responseBody))
		upstreamErr := service.RelayErrorHandler(c.Request.Context(), httpResponse, false)
		service.ResetStatusCode(upstreamErr, c.GetString("status_code_mapping"))
		if httpResponse.StatusCode >= http.StatusMultipleChoices && httpResponse.StatusCode < http.StatusBadRequest {
			return types.NewError(upstreamErr, upstreamErr.GetErrorCode(), types.ErrOptionWithSkipRetry())
		}
		return upstreamErr
	}

	if err := validateAlphaSearchResponse(responseBody); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	if info.IsChannelTest {
		service.AttachChannelMetricUsage(c, service.ChannelMetricUsage{})
	} else {
		if err := service.SettleAlphaSearchBilling(c, info); err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeUpdateDataError, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		}
	}

	contentType := httpResponse.Header.Get("Content-Type")
	mediaType, _, contentTypeErr := mime.ParseMediaType(contentType)
	if contentTypeErr != nil || (mediaType != "application/json" && !(strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))) {
		contentType = "application/json"
	}
	c.Writer.Header().Set("Content-Type", contentType)
	c.Writer.Header().Set("Content-Length", strconv.Itoa(len(responseBody)))
	c.Writer.WriteHeader(httpResponse.StatusCode)
	if _, err := c.Writer.Write(responseBody); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	return nil
}

func buildAlphaSearchRequestBody(rawBody []byte, upstreamModel string) ([]byte, error) {
	if len(bytes.TrimSpace(rawBody)) == 0 {
		return nil, errors.New("empty alpha search request body")
	}
	if upstreamModel == "" {
		return nil, errors.New("empty alpha search upstream model")
	}
	var current struct {
		Model string `json:"model"`
	}
	if err := common.Unmarshal(rawBody, &current); err != nil {
		return nil, err
	}
	if current.Model == upstreamModel {
		return append([]byte(nil), rawBody...), nil
	}
	return sjson.SetBytes(rawBody, "model", upstreamModel)
}

func validateAlphaSearchMappedModel(body []byte, expectedModel string) error {
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil {
		return fmt.Errorf("invalid alpha search request after parameter override: %w", err)
	}
	rawModel, ok := fields["model"]
	if !ok || common.GetJsonType(rawModel) != "string" {
		return errors.New("alpha search parameter override must preserve the mapped model")
	}
	var model string
	if err := common.Unmarshal(rawModel, &model); err != nil || model != expectedModel {
		return errors.New("alpha search parameter override conflicts with the mapped model")
	}
	if rawStream, ok := fields["stream"]; ok {
		var stream bool
		if common.GetJsonType(rawStream) != "boolean" || common.Unmarshal(rawStream, &stream) != nil || stream {
			return errors.New("alpha search parameter override must preserve synchronous JSON")
		}
	}
	return nil
}

func readBoundedAlphaSearchResponse(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxAlphaSearchResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read alpha search response: %w", err)
	}
	if int64(len(data)) > maxAlphaSearchResponseBytes {
		return nil, fmt.Errorf("alpha search response exceeds %d bytes", maxAlphaSearchResponseBytes)
	}
	return data, nil
}

func validateAlphaSearchResponse(body []byte) error {
	if common.GetJsonType(body) != "object" {
		return errors.New("alpha search response must be a JSON object")
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil {
		return fmt.Errorf("invalid alpha search response: %w", err)
	}
	if rawError, ok := fields["error"]; ok && common.GetJsonType(rawError) == "object" {
		return errors.New("alpha search upstream returned an error object")
	}
	output, ok := fields["output"]
	if !ok || common.GetJsonType(output) != "string" {
		return errors.New("alpha search response output must be a string")
	}
	if encrypted, ok := fields["encrypted_output"]; ok && common.GetJsonType(encrypted) != "string" {
		return errors.New("alpha search response encrypted_output must be a string")
	}
	return nil
}
