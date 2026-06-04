package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestTaskPollingPlatformFallsBackToChannelTypeWhenTaskPlatformMissing(t *testing.T) {
	got := taskPollingPlatform("", &model.Channel{Type: constant.ChannelTypeOpenAI})
	assert.Equal(t, constant.TaskPlatform("1"), got)
}

func TestTaskPollingPlatformPreservesExplicitPlatform(t *testing.T) {
	got := taskPollingPlatform(constant.TaskPlatformSuno, &model.Channel{Type: constant.ChannelTypeOpenAI})
	assert.Equal(t, constant.TaskPlatformSuno, got)
}
