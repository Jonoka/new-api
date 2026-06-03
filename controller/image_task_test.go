package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// compile-time adapter for gin-specific Refund signature
type imageTaskTestBillingSettler struct {
	preConsumed int
}

func (s imageTaskTestBillingSettler) Settle(int) error         { return nil }
func (s imageTaskTestBillingSettler) Refund(*gin.Context)      {}
func (s imageTaskTestBillingSettler) NeedsRefund() bool        { return false }
func (s imageTaskTestBillingSettler) GetPreConsumedQuota() int { return s.preConsumed }
func (s imageTaskTestBillingSettler) Reserve(int) error        { return nil }

func TestParseImageTaskSubmitResponseBodyMarksImmediateURLResultSuccess(t *testing.T) {
	body := []byte(`{
		"created": 1780224045,
		"data": [{"url": "https://example.invalid/generated.png", "width": 1024, "height": 1024}],
		"task_id": "task_sync_url",
		"usage": {"total_cost": 8, "total_points": 0.08}
	}`)

	taskID, upstreamTaskID, status, progress := parseImageTaskSubmitResponseBody(body)

	assert.Equal(t, "task_sync_url", taskID)
	assert.Equal(t, "task_sync_url", upstreamTaskID)
	assert.Equal(t, string(model.TaskStatusSuccess), status)
	assert.Equal(t, "100%", progress)
}

func TestParseImageTaskSubmitResponseBodyMarksImmediateB64ResultSuccess(t *testing.T) {
	body := []byte(`{
		"id": "provider_task_123",
		"task_id": "task_sync_b64",
		"data": [{"b64_json": "iVBORw0KGgo="}]
	}`)

	taskID, upstreamTaskID, status, progress := parseImageTaskSubmitResponseBody(body)

	assert.Equal(t, "task_sync_b64", taskID)
	assert.Equal(t, "provider_task_123", upstreamTaskID)
	assert.Equal(t, string(model.TaskStatusSuccess), status)
	assert.Equal(t, "100%", progress)
}

func TestParseImageTaskSubmitResponseBodyPreservesQueuedTaskWithoutResult(t *testing.T) {
	body := []byte(`{
		"id": "provider_task_queued",
		"task_id": "task_queued",
		"status": "queued",
		"progress": 0
	}`)

	taskID, upstreamTaskID, status, progress := parseImageTaskSubmitResponseBody(body)

	assert.Equal(t, "task_queued", taskID)
	assert.Equal(t, "provider_task_queued", upstreamTaskID)
	assert.Equal(t, string(model.TaskStatusQueued), status)
	assert.Equal(t, "0%", progress)
}

func TestParseImageTaskSubmitResponseBodyCompletedStatusProgress(t *testing.T) {
	body := []byte(`{
		"id": "provider_task_done",
		"task_id": "task_done",
		"status": "completed",
		"progress": 100
	}`)

	_, _, status, progress := parseImageTaskSubmitResponseBody(body)

	assert.Equal(t, string(model.TaskStatusSuccess), status)
	assert.Equal(t, "100%", progress)
}

func TestImageTaskPersistedQuotaPrefersFinalQuota(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota:             125000,
			QuotaToPreConsume: 100000,
		},
		Billing: imageTaskTestBillingSettler{preConsumed: 90000},
	}

	assert.Equal(t, 125000, imageTaskPersistedQuota(relayInfo))
}

func TestImageTaskPersistedQuotaFallsBackToPreConsume(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			QuotaToPreConsume: 100000,
		},
		Billing: imageTaskTestBillingSettler{preConsumed: 90000},
	}

	assert.Equal(t, 100000, imageTaskPersistedQuota(relayInfo))
}

func TestImageTaskPersistedQuotaFallsBackToBillingSession(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		Billing: imageTaskTestBillingSettler{preConsumed: 90000},
	}

	assert.Equal(t, 90000, imageTaskPersistedQuota(relayInfo))
}

func TestNormalizeOpenAIImageGenerationQuality(t *testing.T) {
	cases := map[string]string{
		"":         "auto",
		"auto":     "auto",
		"LOW":      "low",
		" medium ": "medium",
		"high":     "high",
		"4k":       "auto",
		"ultra":    "auto",
	}

	for input, want := range cases {
		assert.Equal(t, want, dto.NormalizeOpenAIImageGenerationQuality(input), input)
	}
}
