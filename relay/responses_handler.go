package relay

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func syncResponsesStreamStateFromBody(c *gin.Context, info *relaycommon.RelayInfo, jsonData []byte) {
	if info == nil || len(jsonData) == 0 {
		return
	}
	streamValue := gjson.GetBytes(jsonData, "stream")
	if streamValue.Type != gjson.True && streamValue.Type != gjson.False {
		return
	}
	info.IsStream = streamValue.Bool()
	common.SetContextKey(c, appconstant.ContextKeyIsStream, info.IsStream)
}

func responsesRequestFromRelayInput(request any) (*dto.OpenAIResponsesRequest, error) {
	switch req := request.(type) {
	case *dto.OpenAIResponsesRequest:
		return req, nil
	case *dto.OpenAIResponsesCompactionRequest:
		return &dto.OpenAIResponsesRequest{
			Model:                req.Model,
			Input:                req.Input,
			Instructions:         req.Instructions,
			PreviousResponseID:   req.PreviousResponseID,
			Tools:                req.Tools,
			ParallelToolCalls:    req.ParallelToolCalls,
			Reasoning:            req.Reasoning,
			ServiceTier:          req.ServiceTier,
			PromptCacheKey:       req.PromptCacheKey,
			PromptCacheOptions:   req.PromptCacheOptions,
			PromptCacheRetention: req.PromptCacheRetention,
			Text:                 req.Text,
		}, nil
	default:
		return nil, fmt.Errorf("invalid request type, expected dto.OpenAIResponsesRequest or dto.OpenAIResponsesCompactionRequest, got %T", request)
	}
}

func ResponsesHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		switch info.ApiType {
		case appconstant.APITypeOpenAI, appconstant.APITypeCodex:
		default:
			return types.NewErrorWithStatusCode(
				fmt.Errorf("unsupported endpoint %q for api type %d", "/v1/responses/compact", info.ApiType),
				types.ErrorCodeInvalidRequest,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	responsesReq, err := responsesRequestFromRelayInput(info.Request)
	if err != nil {
		return types.NewErrorWithStatusCode(
			err,
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	request, err := common.DeepCopy(responsesReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	attachedPreviousResponseID := false
	if info.RelayMode == relayconstant.RelayModeResponses {
		attachedPreviousResponseID = service.AttachOpenAIResponsesContinuation(info, request)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	var requestBody io.Reader
	var retryWithoutPreviousResponse func(statusCode int, message string) (*dto.Usage, *types.NewAPIError)
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.ReaderOnly(storage)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI Responses API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}
		if cleanedJSONData, removed := service.RemoveIncompleteOpenAIResponsesReasoningHistoryFromJSON(jsonData); removed {
			jsonData = cleanedJSONData
			attachedPreviousResponseID = false
			service.DropOpenAIResponsesPreviousResponseID(request)
		}
		relaycommon.MergeOpenAISessionBridgeOverride(info, jsonData)
		syncResponsesStreamStateFromBody(c, info, jsonData)
		retryJSONData := append([]byte(nil), jsonData...)
		retryWithoutPreviousResponse = func(statusCode int, message string) (*dto.Usage, *types.NewAPIError) {
			if info.RelayMode != relayconstant.RelayModeResponses || !attachedPreviousResponseID || !service.IsOpenAIResponsesPreviousResponseRetryable(statusCode, message) {
				return nil, nil
			}
			service.DeleteOpenAIResponsesContinuationResponseID(info, request)
			service.DropOpenAIResponsesPreviousResponseID(request)
			retryJSONData = service.RemoveOpenAIResponsesPreviousResponseIDFromJSON(retryJSONData)
			attachedPreviousResponseID = false

			body, size, closer, bodyErr := relaycommon.NewOutboundJSONBody(retryJSONData)
			if bodyErr != nil {
				return nil, types.NewError(bodyErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			defer closer.Close()
			info.UpstreamRequestBodySize = size

			statusCodeMappingStr := c.GetString("status_code_mapping")
			respRetry, doErr := adaptor.DoRequest(c, info, body)
			if doErr != nil {
				return nil, types.NewOpenAIError(doErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
			}
			if respRetry == nil {
				return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			}
			httpRespRetry := respRetry.(*http.Response)
			markActualStreamFromResponse(c, info, httpRespRetry)
			if httpRespRetry.StatusCode != http.StatusOK {
				newAPIError := service.RelayErrorHandler(c.Request.Context(), httpRespRetry, false)
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return nil, newAPIError
			}
			usage, newAPIError := adaptor.DoResponse(c, httpRespRetry, info)
			if newAPIError != nil {
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return nil, newAPIError
			}
			usageDto, _ := usage.(*dto.Usage)
			return usageDto, nil
		}

		logger.LogDebug(c, "requestBody: %s", jsonData)
		body, size, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		info.UpstreamRequestBodySize = size
		requestBody = body
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)

		markActualStreamFromResponse(c, info, httpResp)

		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			if retryWithoutPreviousResponse != nil {
				if retryUsage, retryErr := retryWithoutPreviousResponse(newAPIError.StatusCode, newAPIError.ErrorWithStatusCode()); retryErr != nil || retryUsage != nil {
					if retryErr != nil {
						return retryErr
					}
					if retryUsage != nil {
						usageDto := retryUsage
						if info.RelayMode == relayconstant.RelayModeResponsesCompact {
							originModelName := info.OriginModelName
							originPriceData := info.PriceData
							_, err := helper.ModelPriceHelper(c, info, info.GetEstimatePromptTokens(), &types.TokenCountMeta{})
							if err != nil {
								info.OriginModelName = originModelName
								info.PriceData = originPriceData
								return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusBadRequest))
							}
							service.PostTextConsumeQuota(c, info, usageDto, nil)
							info.OriginModelName = originModelName
							info.PriceData = originPriceData
							return nil
						}
						if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
							service.PostAudioConsumeQuota(c, info, usageDto, "")
						} else {
							service.PostTextConsumeQuota(c, info, usageDto, nil)
						}
						return nil
					}
				}
			}
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		if retryWithoutPreviousResponse != nil {
			if retryUsage, retryErr := retryWithoutPreviousResponse(newAPIError.StatusCode, newAPIError.ErrorWithStatusCode()); retryErr != nil || retryUsage != nil {
				if retryErr != nil {
					return retryErr
				}
				if retryUsage != nil {
					usage = retryUsage
					newAPIError = nil
				}
			}
		}
	}
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	usageDto := usage.(*dto.Usage)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		originModelName := info.OriginModelName
		originPriceData := info.PriceData

		_, err := helper.ModelPriceHelper(c, info, info.GetEstimatePromptTokens(), &types.TokenCountMeta{})
		if err != nil {
			info.OriginModelName = originModelName
			info.PriceData = originPriceData
			return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusBadRequest))
		}
		service.PostTextConsumeQuota(c, info, usageDto, nil)

		info.OriginModelName = originModelName
		info.PriceData = originPriceData
		return nil
	}

	if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
		service.PostAudioConsumeQuota(c, info, usageDto, "")
	} else {
		service.PostTextConsumeQuota(c, info, usageDto, nil)
	}
	return nil
}
