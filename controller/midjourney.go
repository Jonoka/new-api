package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

func UpdateMidjourneyTaskBulk() {
	//imageModel := "midjourney"
	ctx := context.TODO()
	for {
		time.Sleep(time.Duration(15) * time.Second)

		tasks := model.GetAllUnFinishTasks()
		if len(tasks) == 0 {
			continue
		}

		logger.LogInfo(ctx, fmt.Sprintf("检测到未完成的任务数有: %v", len(tasks)))
		taskChannelM := make(map[int][]string)
		taskChannelSeen := make(map[int]map[string]struct{})
		taskM := make(map[string][]*model.Midjourney)
		nullTaskIds := make([]int, 0)
		for _, task := range tasks {
			projected, err := projectCompletedMidjourneyAccounting(ctx, task)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("project Midjourney accounting task %d: %v", task.Id, err))
				continue
			}
			if projected {
				continue
			}
			if task.MjId == "" {
				if task.TaskRowID != nil {
					logger.LogError(ctx, fmt.Sprintf("linked Midjourney task %d has no upstream task id", task.Id))
					continue
				}
				// 统计失败的未完成任务
				nullTaskIds = append(nullTaskIds, task.Id)
				continue
			}
			key := midjourneyPollingKey(task.ChannelId, task.MjId)
			taskM[key] = append(taskM[key], task)
			if taskChannelSeen[task.ChannelId] == nil {
				taskChannelSeen[task.ChannelId] = make(map[string]struct{})
			}
			if _, exists := taskChannelSeen[task.ChannelId][task.MjId]; !exists {
				taskChannelSeen[task.ChannelId][task.MjId] = struct{}{}
				taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], task.MjId)
			}
		}
		if len(nullTaskIds) > 0 {
			err := model.MjBulkUpdateByTaskIds(nullTaskIds, map[string]any{
				"status":   "FAILURE",
				"progress": "100%",
			})
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Fix null mj_id task error: %v", err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("Fix null mj_id task success: %v", nullTaskIds))
			}
		}
		if len(taskChannelM) == 0 {
			continue
		}

		for channelId, taskIds := range taskChannelM {
			logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelId, len(taskIds)))
			if len(taskIds) == 0 {
				continue
			}
			midjourneyChannel, err := model.CacheGetChannel(channelId)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("CacheGetChannel: %v", err))
				legacyTaskRowIDs := make([]int, 0, len(taskIds))
				failureReason := fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId)
				for _, taskID := range taskIds {
					for _, task := range taskM[midjourneyPollingKey(channelId, taskID)] {
						if task.TaskRowID == nil {
							legacyTaskRowIDs = append(legacyTaskRowIDs, task.Id)
							continue
						}
						if err := failLinkedMidjourneyTask(ctx, task, failureReason); err != nil {
							logger.LogError(ctx, fmt.Sprintf("fail linked Midjourney task %d: %v", task.Id, err))
						}
					}
				}
				if len(legacyTaskRowIDs) > 0 {
					err := model.MjBulkUpdateByTaskIds(legacyTaskRowIDs, map[string]any{
						"fail_reason": failureReason,
						"status":      "FAILURE",
						"progress":    "100%",
					})
					if err != nil {
						logger.LogInfo(ctx, fmt.Sprintf("UpdateMidjourneyTask error: %v", err))
					}
				}
				continue
			}
			requestUrl := fmt.Sprintf("%s/mj/task/list-by-condition", *midjourneyChannel.BaseURL)

			body, _ := common.Marshal(map[string]any{
				"ids": taskIds,
			})
			req, err := http.NewRequest("POST", requestUrl, bytes.NewBuffer(body))
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Get Task error: %v", err))
				continue
			}
			// 设置超时时间
			timeout := time.Second * 15
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			// 使用带有超时的 context 创建新的请求
			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("mj-api-secret", midjourneyChannel.Key)
			resp, err := service.GetHttpClient().Do(req)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Get Task Do req error: %v", err))
				continue
			}
			if resp.StatusCode != http.StatusOK {
				logger.LogError(ctx, fmt.Sprintf("Get Task status code: %d", resp.StatusCode))
				continue
			}
			responseBody, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Get Mjp Task parse body error: %v", err))
				continue
			}
			var responseItems []dto.MidjourneyDto
			err = common.Unmarshal(responseBody, &responseItems)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Get Mjp Task parse body error2: %v, body: %s", err, string(responseBody)))
				continue
			}
			resp.Body.Close()
			req.Body.Close()
			cancel()

			for _, responseItem := range responseItems {
				tasksForResponse := taskM[midjourneyPollingKey(channelId, responseItem.MjId)]
				if len(tasksForResponse) == 0 {
					continue
				}
				for _, task := range tasksForResponse {
					item := responseItem
					useTime := (time.Now().UnixNano() / int64(time.Millisecond)) - task.SubmitTime
					// 如果时间超过一小时，且进度不是100%，则认为任务失败
					if useTime > 3600000 && task.Progress != "100%" {
						item.FailReason = "上游任务超时（超过1小时）"
						item.Status = "FAILURE"
					}
					if !checkMjTaskNeedUpdate(task, item) {
						continue
					}
					preStatus := task.Status
					task.Code = 1
					task.Progress = item.Progress
					task.PromptEn = item.PromptEn
					task.State = item.State
					task.SubmitTime = item.SubmitTime
					task.StartTime = item.StartTime
					task.FinishTime = item.FinishTime
					task.ImageUrl = item.ImageUrl
					task.Status = item.Status
					task.FailReason = item.FailReason
					service.ApplyMidjourneyTaskProjection(task, item)

					shouldReturnQuota := false
					if (task.Progress != "100%" && item.FailReason != "") || (task.Progress == "100%" && task.Status == "FAILURE") {
						logger.LogInfo(ctx, task.MjId+" 构建失败，"+task.FailReason)
						task.Progress = "100%"
						if task.Quota != 0 {
							shouldReturnQuota = true
						}
					}
					if task.TaskRowID != nil && (task.Status == string(model.TaskStatusSuccess) || task.Status == string(model.TaskStatusFailure)) {
						accountingTask, err := model.GetTaskByRowID(*task.TaskRowID)
						if err != nil {
							logger.LogError(ctx, fmt.Sprintf("load Midjourney accounting task %d: %v", *task.TaskRowID, err))
							continue
						}
						reason := "midjourney task completed"
						if shouldReturnQuota {
							reason = "构图失败"
						}
						terminalProjection := item
						terminalProjection.Status = task.Status
						terminalProjection.Progress = task.Progress
						terminalProjection.FailReason = task.FailReason
						if _, err := service.FinalizeMidjourneyTaskAccounting(ctx, accountingTask, terminalProjection, task.Quota, reason); err != nil {
							logger.LogError(ctx, "finalize Midjourney task accounting: "+err.Error())
							continue
						}
						projection, err := service.MidjourneyTaskProjection(accountingTask)
						if err != nil {
							logger.LogError(ctx, "load Midjourney task projection: "+err.Error())
							continue
						}
						service.ApplyMidjourneyTaskProjection(task, projection)
					}
					won, err := updateMidjourneyTaskWithLegacyRefund(ctx, task, preStatus, shouldReturnQuota)
					if err != nil {
						logger.LogError(ctx, "UpdateMidjourneyTask task error: "+err.Error())
					} else if !won {
						logger.LogInfo(ctx, fmt.Sprintf("Midjourney task %d was updated by another poller", task.Id))
					}
				}
			}
		}
	}
}

func failLinkedMidjourneyTask(ctx context.Context, task *model.Midjourney, reason string) error {
	if task == nil || task.TaskRowID == nil {
		return nil
	}
	accountingTask, err := model.GetTaskByRowID(*task.TaskRowID)
	if err != nil {
		return err
	}
	terminal := dto.MidjourneyDto{
		MjId: task.MjId, Status: string(model.TaskStatusFailure), Progress: "100%",
		FailReason: reason, FinishTime: time.Now().UnixMilli(),
	}
	if _, err := service.FinalizeMidjourneyTaskAccounting(ctx, accountingTask, terminal, task.Quota, reason); err != nil {
		return err
	}
	projection, err := service.MidjourneyTaskProjection(accountingTask)
	if err != nil {
		return err
	}
	preStatus := task.Status
	service.ApplyMidjourneyTaskProjection(task, projection)
	_, err = task.UpdateWithStatus(preStatus)
	return err
}

func updateMidjourneyTaskWithLegacyRefund(ctx context.Context, task *model.Midjourney, preStatus string, shouldReturnQuota bool) (bool, error) {
	won, err := task.UpdateWithStatus(preStatus)
	if err != nil || !won || task.TaskRowID != nil || !shouldReturnQuota {
		return won, err
	}
	if err := model.IncreaseUserQuota(task.UserId, task.Quota, false); err != nil {
		return true, err
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: task.UserId, LogType: model.LogTypeRefund, ChannelId: task.ChannelId,
		ModelName: service.CovertMjpActionToModelName(task.Action), Quota: task.Quota,
		Other: map[string]interface{}{"task_id": task.MjId, "reason": "构图失败"},
	})
	return true, nil
}

func projectCompletedMidjourneyAccounting(ctx context.Context, task *model.Midjourney) (bool, error) {
	if task == nil || task.TaskRowID == nil {
		return false, nil
	}
	accountingTask, err := model.GetTaskByRowID(*task.TaskRowID)
	if err != nil {
		return false, err
	}
	if accountingTask.Status != model.TaskStatusSuccess && accountingTask.Status != model.TaskStatusFailure {
		return false, nil
	}
	projection, err := service.MidjourneyTaskProjection(accountingTask)
	if err != nil {
		return false, err
	}
	preStatus := task.Status
	service.ApplyMidjourneyTaskProjection(task, projection)
	_, err = task.UpdateWithStatus(preStatus)
	return err == nil, err
}

func midjourneyPollingKey(channelID int, taskID string) string {
	return fmt.Sprintf("%d\x00%s", channelID, taskID)
}

func checkMjTaskNeedUpdate(oldTask *model.Midjourney, newTask dto.MidjourneyDto) bool {
	if oldTask.Code != 1 {
		return true
	}
	if oldTask.Progress != newTask.Progress {
		return true
	}
	if oldTask.PromptEn != newTask.PromptEn {
		return true
	}
	if oldTask.State != newTask.State {
		return true
	}
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.ImageUrl != newTask.ImageUrl {
		return true
	}
	if oldTask.Status != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.Progress != "100%" && newTask.FailReason != "" {
		return true
	}
	// 检查 VideoUrl 是否需要更新
	if oldTask.VideoUrl != newTask.VideoUrl {
		return true
	}
	// 检查 VideoUrls 是否需要更新
	if newTask.VideoUrls != nil && len(newTask.VideoUrls) > 0 {
		newVideoUrlsStr, _ := common.Marshal(newTask.VideoUrls)
		if oldTask.VideoUrls != string(newVideoUrlsStr) {
			return true
		}
	} else if oldTask.VideoUrls != "" {
		// 如果新数据没有 VideoUrls 但旧数据有，需要更新（清空）
		return true
	}

	return false
}

func GetAllMidjourney(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	// 解析其他查询参数
	queryParams := model.TaskQueryParams{
		ChannelID:      c.Query("channel_id"),
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	items := model.GetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllTasks(queryParams)

	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func GetUserMidjourney(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	queryParams := model.TaskQueryParams{
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	items := model.GetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllUserTask(userId, queryParams)

	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}
