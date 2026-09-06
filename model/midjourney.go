package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

type Midjourney struct {
	Id          int    `json:"id"`
	Code        int    `json:"code"`
	UserId      int    `json:"user_id" gorm:"index"`
	Action      string `json:"action" gorm:"type:varchar(40);index"`
	MjId        string `json:"mj_id" gorm:"index"`
	Prompt      string `json:"prompt"`
	PromptEn    string `json:"prompt_en"`
	Description string `json:"description"`
	State       string `json:"state"`
	SubmitTime  int64  `json:"submit_time" gorm:"index"`
	StartTime   int64  `json:"start_time" gorm:"index"`
	FinishTime  int64  `json:"finish_time" gorm:"index"`
	ImageUrl    string `json:"image_url"`
	VideoUrl    string `json:"video_url"`
	VideoUrls   string `json:"video_urls"`
	Status      string `json:"status" gorm:"type:varchar(20);index"`
	Progress    string `json:"progress" gorm:"type:varchar(30);index"`
	FailReason  string `json:"fail_reason"`
	ChannelId   int    `json:"channel_id"`
	Quota       int    `json:"quota"`
	Buttons     string `json:"buttons"`
	Properties  string `json:"properties"`
	TaskRowID   *int64 `json:"-" gorm:"index"`
}

// TaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type TaskQueryParams struct {
	ChannelID      string
	MjID           string
	StartTimestamp string
	EndTimestamp   string
}

func GetAllUserTask(userId int, startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB.Where("user_id = ?", userId)

	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		// 假设您已将前端传来的时间戳转换为数据库所需的时间格式，并处理了时间戳的验证和解析
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllTasks(startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB

	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllUnFinishTasks() []*Midjourney {
	var tasks []*Midjourney
	var err error
	// get all tasks progress is not 100%
	err = DB.Where("progress != ?", "100%").Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func GetByOnlyMJId(mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("mj_id = ?", mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJId(userId int, mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("user_id = ? and mj_id = ?", userId, mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJIds(userId int, mjIds []string) []*Midjourney {
	var mj []*Midjourney
	var err error
	err = DB.Where("user_id = ? and mj_id in (?)", userId, mjIds).Find(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetMjByuId(id int) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("id = ?", id).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func UpdateProgress(id int, progress string) error {
	return DB.Model(&Midjourney{}).Where("id = ?", id).Update("progress", progress).Error
}

func (midjourney *Midjourney) Insert() error {
	var err error
	err = DB.Create(midjourney).Error
	return err
}

func (midjourney *Midjourney) Update() error {
	var err error
	err = DB.Save(midjourney).Error
	return err
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Uses Model().Select("*").Updates() to avoid GORM Save()'s INSERT fallback.
func (midjourney *Midjourney) UpdateWithStatus(fromStatus string) (bool, error) {
	result := DB.Model(midjourney).Where("status = ?", fromStatus).Select("*").Updates(midjourney)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// PersistMidjourneySubmissionLink creates the internal task and legacy public
// row, then attaches both to the already-funded submission journal. Accounting
// ownership is transferred by service.HandoffTaskBilling afterward.
func PersistMidjourneySubmissionLink(ctx context.Context, submissionID, leaseToken string, midjourney *Midjourney, task *Task) error {
	if submissionID == "" || leaseToken == "" || midjourney == nil || task == nil || task.UserId <= 0 || task.UserId != midjourney.UserId {
		return errors.New("invalid midjourney submission link")
	}
	if task.Platform != constant.TaskPlatformMidjourney || task.TaskID == "" || midjourney.MjId == "" ||
		task.PrivateData.UpstreamTaskID != midjourney.MjId || task.ChannelId != midjourney.ChannelId || task.Action != midjourney.Action {
		return errors.New("midjourney submission identity mismatch")
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := getDBTimestampTx(tx)
		task.CreatedAt = now
		task.UpdatedAt = now
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		taskRowID := task.ID
		midjourney.TaskRowID = &taskRowID
		if err := tx.Create(midjourney).Error; err != nil {
			return err
		}
		result := tx.Model(&TaskSubmission{}).
			Where("submission_id = ? AND state = ? AND lease_token = ? AND user_id = ? AND task_row_id IS NULL", submissionID, TaskSubmissionStateActive, leaseToken, task.UserId).
			Updates(map[string]any{
				"task_row_id":      task.ID,
				"lease_expires_at": now + taskSubmissionLeaseSeconds(0),
				"updated_at":       now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTaskSubmissionConflict
		}
		return nil
	})
	if err == nil {
		return nil
	}
	if verifyErr := verifyMidjourneySubmissionLink(ctx, submissionID, leaseToken, midjourney, task); verifyErr != nil {
		return fmt.Errorf("persist midjourney submission link failed (%v); durable result unresolved: %w", err, verifyErr)
	}
	return nil
}

func verifyMidjourneySubmissionLink(ctx context.Context, submissionID, leaseToken string, expectedMidjourney *Midjourney, expectedTask *Task) error {
	if expectedMidjourney.Id <= 0 || expectedTask.ID <= 0 {
		return errors.New("midjourney submission link identity is missing")
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var midjourney Midjourney
		if err := tx.First(&midjourney, expectedMidjourney.Id).Error; err != nil {
			return err
		}
		if midjourney.TaskRowID == nil || *midjourney.TaskRowID != expectedTask.ID ||
			midjourney.UserId != expectedMidjourney.UserId || midjourney.MjId != expectedMidjourney.MjId ||
			midjourney.ChannelId != expectedMidjourney.ChannelId || midjourney.Action != expectedMidjourney.Action ||
			midjourney.Quota != expectedMidjourney.Quota {
			return errors.New("midjourney submission link mismatch")
		}
		var task Task
		if err := tx.First(&task, expectedTask.ID).Error; err != nil {
			return err
		}
		if task.TaskID != expectedTask.TaskID || task.UserId != expectedTask.UserId || task.Platform != expectedTask.Platform ||
			task.ChannelId != expectedTask.ChannelId || task.Action != expectedTask.Action ||
			task.PrivateData.UpstreamTaskID != expectedTask.PrivateData.UpstreamTaskID {
			return errors.New("midjourney accounting task mismatch")
		}
		var submission TaskSubmission
		if err := tx.Where("submission_id = ?", submissionID).First(&submission).Error; err != nil {
			return err
		}
		if submission.State != TaskSubmissionStateActive || submission.LeaseToken != leaseToken || submission.UserID != expectedTask.UserId || submission.TaskRowID == nil || *submission.TaskRowID != expectedTask.ID {
			return errors.New("midjourney task submission link mismatch")
		}
		return nil
	})
}

func MjBulkUpdate(mjIds []string, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("mj_id in (?)", mjIds).
		Updates(params).Error
}

func MjBulkUpdateByTaskIds(taskIDs []int, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("id in (?)", taskIDs).
		Updates(params).Error
}

// CountAllTasks returns total midjourney tasks for admin query
func CountAllTasks(queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// CountAllUserTask returns total midjourney tasks for user
func CountAllUserTask(userId int, queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{}).Where("user_id = ?", userId)
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}
