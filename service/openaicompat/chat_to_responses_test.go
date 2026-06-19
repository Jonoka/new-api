package openaicompat

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsRequestToResponsesRequestPreservesStream(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(strconv.FormatBool(stream), func(t *testing.T) {
			req := &dto.GeneralOpenAIRequest{
				Model:  "test-model",
				Stream: common.GetPointer(stream),
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: "hello",
					},
				},
			}

			respReq, err := ChatCompletionsRequestToResponsesRequest(req)
			require.NoError(t, err)
			require.NotNil(t, respReq.Stream)
			require.Equal(t, stream, *respReq.Stream)
		})
	}
}
