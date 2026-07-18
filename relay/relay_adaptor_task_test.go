package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetTaskAdaptorSupportsXAI(t *testing.T) {
	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeXai))
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		t.Fatal("GetTaskAdaptor(xAI) = nil")
	}
	if adaptor.GetChannelName() != "xai" {
		t.Fatalf("channel name = %q, want xai", adaptor.GetChannelName())
	}
}
