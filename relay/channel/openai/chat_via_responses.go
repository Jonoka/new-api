package openai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func responsesStreamIndexKey(itemID string, idx *int) string {
	if itemID == "" {
		return ""
	}
	if idx == nil {
		return itemID
	}
	return fmt.Sprintf("%s:%d", itemID, *idx)
}

func stringDeltaFromPrefix(prev string, next string) string {
	if next == "" {
		return ""
	}
	if prev != "" && strings.HasPrefix(next, prev) {
		return next[len(prev):]
	}
	return next
}

func isResponsesSSEBody(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return bytes.HasPrefix(trimmed, []byte("data:")) ||
		bytes.HasPrefix(trimmed, []byte("event:")) ||
		bytes.HasPrefix(trimmed, []byte("id:")) ||
		bytes.HasPrefix(trimmed, []byte("retry:")) ||
		bytes.HasPrefix(trimmed, []byte(":"))
}

func normalizeResponsesStreamJSONData(data string) (string, bool) {
	data = strings.TrimSpace(data)
	for {
		switch {
		case data == "" || data == "[DONE]":
			return "", false
		case strings.HasPrefix(data, "data:"):
			data = strings.TrimSpace(strings.TrimPrefix(data, "data:"))
			continue
		case strings.HasPrefix(data, "event:"),
			strings.HasPrefix(data, "id:"),
			strings.HasPrefix(data, "retry:"),
			strings.HasPrefix(data, ":"):
			return "", false
		}
		break
	}
	if !strings.HasPrefix(data, "{") {
		return "", false
	}
	return data, true
}

func parseResponsesStreamEventData(data string) (dto.ResponsesStreamResponse, string, bool, error) {
	var streamResp dto.ResponsesStreamResponse
	jsonData, ok := normalizeResponsesStreamJSONData(data)
	if !ok {
		return streamResp, "", false, nil
	}
	if err := common.UnmarshalJsonStr(jsonData, &streamResp); err != nil {
		return streamResp, jsonData, true, err
	}
	return streamResp, jsonData, true, nil
}

func convertResponsesSSEToJSON(sseBody []byte) ([]byte, error) {
	lines := bytes.Split(sseBody, []byte("\n"))

	var (
		completedResp *dto.OpenAIResponsesResponse
		responseID    string
		model         string
		createdAt     int
		outputText    strings.Builder
		usage         *dto.Usage
	)

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}

		streamResp, _, ok, err := parseResponsesStreamEventData(string(data))
		if !ok {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal responses SSE event: %w", err)
		}

		if streamResp.Response != nil {
			if streamResp.Response.ID != "" {
				responseID = streamResp.Response.ID
			}
			if streamResp.Response.Model != "" {
				model = streamResp.Response.Model
			}
			if streamResp.Response.CreatedAt != 0 {
				createdAt = streamResp.Response.CreatedAt
			}
			if streamResp.Response.Usage != nil {
				usage = streamResp.Response.Usage
			}
		}

		switch streamResp.Type {
		case "response.output_text.delta":
			outputText.WriteString(streamResp.Delta)
		case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled":
			if streamResp.Response != nil {
				completedResp = streamResp.Response
			}
		case "response.error", "response.failed":
			if streamResp.Response != nil {
				completedResp = streamResp.Response
			}
		}
	}

	if completedResp == nil {
		completedResp = &dto.OpenAIResponsesResponse{
			ID:        responseID,
			Object:    "response",
			CreatedAt: createdAt,
			Model:     model,
			Usage:     usage,
		}
	}
	if completedResp.ID == "" {
		completedResp.ID = responseID
	}
	if completedResp.Object == "" {
		completedResp.Object = "response"
	}
	if completedResp.CreatedAt == 0 {
		completedResp.CreatedAt = createdAt
	}
	if completedResp.Model == "" {
		completedResp.Model = model
	}
	if completedResp.Usage == nil {
		completedResp.Usage = usage
	}
	if len(completedResp.Output) == 0 && outputText.Len() > 0 {
		completedResp.Output = []dto.ResponsesOutput{
			{
				Type: "message",
				Role: "assistant",
				Content: []dto.ResponsesOutputContent{
					{
						Type: "output_text",
						Text: outputText.String(),
					},
				},
			},
		}
	}

	return common.Marshal(completedResp)
}

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if isResponsesSSEBody(body) {
		converted, convErr := convertResponsesSSEToJSON(body)
		if convErr != nil {
			return nil, types.NewOpenAIError(convErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		body = converted
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	service.BindOpenAIResponsesContinuationResponseIDFromInfo(info, responsesResp.ID)

	chatId := helper.GetResponseID(c)
	chatResp, usage, err := service.ResponsesResponseToChatCompletionsResponse(&responsesResp, chatId)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	var responseBody []byte
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(chatResp, info)
		responseBody, err = common.Marshal(claudeResp)
	case types.RelayFormatGemini:
		geminiResp := service.ResponseOpenAI2Gemini(chatResp, info)
		responseBody, err = common.Marshal(geminiResp)
	default:
		responseBody, err = common.Marshal(chatResp)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	state := openaicompat.NewResponsesToChatStreamState(info.UpstreamModelName, false)
	state.ID = responseId
	state.Created = createAt
	streamErr := (*types.NewAPIError)(nil)

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendChatChunk := func(chunk dto.ChatCompletionsStreamResponse) bool {
		if len(chunk.Choices) == 0 && chunk.Usage == nil {
			return true
		}
		if info.RelayFormat == types.RelayFormatOpenAI {
			if err := helper.ObjectData(c, &chunk); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		}

		chunkData, err := common.Marshal(&chunk)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := HandleStreamFormat(c, info, string(chunkData), false, false); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		streamResp, _, ok, err := parseResponsesStreamEventData(data)
		if !ok {
			return
		}
		if err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			sr.Error(err)
			return
		}
		if streamResp.Response != nil {
			service.BindOpenAIResponsesContinuationResponseIDFromInfo(info, streamResp.Response.ID)
		}

		if streamResp.Type == "response.error" || streamResp.Type == "response.failed" {
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					sr.Stop(streamErr)
					return
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}

		chunks, err := openaicompat.ResponsesStreamEventToChatChunks(&streamResp, state)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, chunk := range chunks {
			if !sendChatChunk(chunk) {
				sr.Stop(streamErr)
				return
			}
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}

	usage := state.Usage
	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.Usage = usage
	}

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
		info.ClaudeConvertInfo.Usage = usage
	}
	for _, chunk := range openaicompat.FinalizeResponsesToChatStream(state) {
		if !sendChatChunk(chunk) {
			return nil, streamErr
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, state.Created, state.Model, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return usage, nil
}
