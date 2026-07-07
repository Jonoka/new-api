package model

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

type Redemption struct {
	Id             int            `json:"id"`
	UserId         int            `json:"user_id"`
	Key            string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status         int            `json:"status" gorm:"default:1"`
	Name           string         `json:"name" gorm:"index"`
	Quota          int            `json:"quota" gorm:"default:100"`
	CreatedTime    int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime   int64          `json:"redeemed_time" gorm:"bigint"`
	Count          int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId     int            `json:"used_user_id"`
	MaxRedeemCount int            `json:"max_redeem_count" gorm:"default:1"`
	RedeemedCount  int            `json:"redeemed_count" gorm:"default:0"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
	ExpiredTime    int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
}

type RedemptionUsage struct {
	Id           int   `json:"id"`
	RedemptionId int   `json:"redemption_id" gorm:"index;uniqueIndex:idx_redemption_usage_user,priority:1"`
	UserId       int   `json:"user_id" gorm:"index;uniqueIndex:idx_redemption_usage_user,priority:2"`
	CreatedTime  int64 `json:"created_time" gorm:"bigint"`
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Build query based on keyword type
	query := tx.Model(&Redemption{})

	// Only try to convert to ID if the string represents a valid integer
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
	} else {
		query = query.Where("name LIKE ?", keyword+"%")
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func Redeem(key string, userId int) (quota int, err error) {
	if key == "" {
		return 0, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return 0, errors.New("无效的 user id")
	}
	redemption := &Redemption{}

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("无效的兑换码")
		}
		if redemption.MaxRedeemCount <= 0 {
			redemption.MaxRedeemCount = 1
		}
		if redemption.RedeemedCount >= redemption.MaxRedeemCount {
			return errors.New("该兑换码已达兑换次数上限")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("该兑换码已被使用")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}
		var existingUsage int64
		if err := tx.Model(&RedemptionUsage{}).
			Where("redemption_id = ? AND user_id = ?", redemption.Id, userId).
			Count(&existingUsage).Error; err != nil {
			return err
		}
		if existingUsage > 0 {
			return errors.New("该兑换码已兑换过")
		}
		now := common.GetTimestamp()
		if err := tx.Create(&RedemptionUsage{
			RedemptionId: redemption.Id,
			UserId:       userId,
			CreatedTime:  now,
		}).Error; err != nil {
			return err
		}
		newStatus := common.RedemptionCodeStatusEnabled
		if redemption.RedeemedCount+1 >= redemption.MaxRedeemCount {
			newStatus = common.RedemptionCodeStatusUsed
		}
		result := tx.Model(&Redemption{}).
			Where("id = ? AND status = ? AND redeemed_count < ?", redemption.Id, common.RedemptionCodeStatusEnabled, redemption.MaxRedeemCount).
			Updates(map[string]interface{}{
				"redeemed_time":  now,
				"redeemed_count": gorm.Expr("redeemed_count + ?", 1),
				"status":         newStatus,
				"used_user_id":   userId,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("该兑换码已达兑换次数上限")
		}
		err = tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
		if err != nil {
			return err
		}
		sourceId := fmt.Sprintf("redemption-%d-user-%d", redemption.Id, userId)
		if err := createAffiliateRewardsForPaymentTx(tx, userId, AffiliateSourceRedemption, sourceId, redemption.Quota); err != nil {
			return err
		}
		redemption.RedeemedTime = now
		redemption.RedeemedCount++
		redemption.Status = newStatus
		redemption.UsedUserId = userId
		return nil
	})
	if err != nil {
		common.SysError("redemption failed: " + err.Error())
		return 0, err
	}
	RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id))
	return redemption.Quota, nil
}

func (redemption *Redemption) Insert() error {
	var err error
	if redemption.Status == 0 {
		redemption.Status = common.RedemptionCodeStatusEnabled
	}
	if redemption.MaxRedeemCount <= 0 {
		redemption.MaxRedeemCount = 1
	}
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	var err error
	if redemption.MaxRedeemCount <= 0 {
		redemption.MaxRedeemCount = 1
	}
	err = DB.Model(redemption).Select("name", "status", "quota", "redeemed_time", "expired_time", "max_redeem_count", "redeemed_count").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
