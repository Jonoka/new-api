package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// UpdateTaskBulk 薄入口，实际轮询逻辑在 service 层
func UpdateTaskBulk() {
	service.TaskPollingLoop()
}

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			userIds.Add(task.UserId)
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		item := relay.TaskModel2Dto(task)
		prepareImageTaskLog(item, task)
		result[i] = item
	}
	return result
}

// GetTaskImageContent 将任务日志中暂存的 Base64 图片转换为可直接预览的图片响应。
// 普通用户只能查看自己的任务；管理员跨用户查看时会再次校验数据库中的实时角色。
func GetTaskImageContent(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	index, err := strconv.Atoi(strings.TrimSpace(c.Param("index")))
	if taskID == "" || err != nil || index < 0 {
		abortCanvasRequest(c, http.StatusBadRequest, "invalid image content request")
		return
	}

	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists {
		viewer, viewerErr := model.GetUserById(c.GetInt("id"), false)
		if viewerErr != nil || viewer.Role < common.RoleAdminUser {
			abortCanvasRequest(c, http.StatusNotFound, "task not found")
			return
		}
		task, exists, err = model.GetByOnlyTaskId(taskID)
		if err != nil {
			abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
			return
		}
	}
	if !exists || task == nil || !constant.IsImageTaskPlatform(task.Platform) {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}

	writeImageTaskContent(c, task, index)
}

func prepareImageTaskLog(item *dto.TaskDto, task *model.Task) {
	if item == nil || task == nil || !constant.IsImageTaskPlatform(task.Platform) {
		return
	}

	// 图片正文可能很大，列表仅返回轻量预览地址，实际查看时再按需解码。
	item.Data = nil
	if task.Status != model.TaskStatusSuccess {
		return
	}
	if imageTaskDataExpired(task, time.Now().Unix()) || len(bytes.TrimSpace(task.Data)) == 0 {
		item.ResultExpired = true
		return
	}

	var payload struct {
		Data []struct {
			URL     string `json:"url,omitempty"`
			B64JSON string `json:"b64_json,omitempty"`
		} `json:"data"`
	}
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return
	}

	for index, image := range payload.Data {
		imageURL := strings.TrimSpace(image.URL)
		switch {
		case isCanvasImageDataURL(imageURL):
			item.ImageURLs = append(item.ImageURLs, fmt.Sprintf(
				"/api/task/%s/content/%d",
				url.PathEscape(task.TaskID),
				index,
			))
		case imageURL != "":
			item.ImageURLs = append(item.ImageURLs, imageURL)
		case strings.TrimSpace(image.B64JSON) != "":
			item.ImageURLs = append(item.ImageURLs, fmt.Sprintf(
				"/api/task/%s/content/%d",
				url.PathEscape(task.TaskID),
				index,
			))
		}
	}
}
