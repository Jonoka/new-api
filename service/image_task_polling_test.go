package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestNormalizePolledTaskDataFlattensOpenAIImageTasksResult(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{TaskProtocol: model.TaskProtocolOpenAIImageTasks}}
	body := []byte(`{"task_id":"abc","status":"succeeded","result":{"created":123,"data":[{"url":"https://example/image.png"}]}}`)
	require.JSONEq(t, `{"created":123,"data":[{"url":"https://example/image.png"}]}`, string(normalizePolledTaskData(task, body)))
}

func TestTaskPollingKeySeparatesSameUpstreamIDAcrossChannels(t *testing.T) {
	require.NotEqual(t, taskPollingKey(11, "same-task"), taskPollingKey(22, "same-task"))
	tasks := map[string]*model.Task{
		taskPollingKey(11, "same-task"): {ChannelId: 11, TaskID: "public-a"},
		taskPollingKey(22, "same-task"): {ChannelId: 22, TaskID: "public-b"},
	}
	require.Equal(t, "public-a", tasks[taskPollingKey(11, "same-task")].TaskID)
	require.Equal(t, "public-b", tasks[taskPollingKey(22, "same-task")].TaskID)
}
