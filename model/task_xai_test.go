package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestInitTaskStoresSelectedXAIKeyForPolling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId: 1,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeXai,
			ChannelId:   2,
			ApiKey:      "selected-xai-key",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}

	task := InitTask(constant.TaskPlatform("48"), info)
	if task.PrivateData.Key != "selected-xai-key" {
		t.Fatalf("stored key = %q", task.PrivateData.Key)
	}
	if task.TaskID != "task_public" {
		t.Fatalf("task id = %q, want task_public", task.TaskID)
	}
}
