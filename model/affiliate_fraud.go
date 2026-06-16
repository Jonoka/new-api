package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	FraudAlertStatusDetected  = "detected"
	FraudAlertStatusResolved  = "resolved"
	FraudAlertStatusDismissed = "dismissed"
)

const (
	FraudActionUnbind   = "unbind"
	FraudActionClawback = "clawback"
	FraudActionDismiss  = "dismiss"
)

type AffiliateFraudAlert struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	InviterId      int    `json:"inviter_id" gorm:"index"`
	InviteeId      int    `json:"invitee_id" gorm:"index"`
	SharedIps      string `json:"shared_ips" gorm:"type:text"`
	SharedIpCount  int    `json:"shared_ip_count"`
	Status         string `json:"status" gorm:"type:varchar(32);index;default:detected"`
	ResolvedAction string `json:"resolved_action" gorm:"type:varchar(32)"`
	ClawbackQuota  int    `json:"clawback_quota"`
	AdminId        int    `json:"admin_id"`
	AdminRemark    string `json:"admin_remark" gorm:"type:varchar(500)"`
	DetectedAt     int64  `json:"detected_at"`
	ResolvedAt     int64  `json:"resolved_at"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AffiliateFraudAlert) TableName() string {
	return "affiliate_fraud_alerts"
}

type FraudAlertWithUsers struct {
	AffiliateFraudAlert
	InviterUsername string `json:"inviter_username"`
	InviteeUsername string `json:"invitee_username"`
}

func DetectFraudForInviter(inviterId int) (int, error) {
	var inviteeIds []int
	if err := DB.Model(&User{}).Where("inviter_id = ?", inviterId).Pluck("id", &inviteeIds).Error; err != nil {
		return 0, err
	}
	if len(inviteeIds) == 0 {
		return 0, nil
	}

	overlaps, err := GetIPOverlapBatch(inviterId, inviteeIds)
	if err != nil {
		return 0, err
	}

	newAlerts := 0
	for inviteeId, sharedIPs := range overlaps {
		if len(sharedIPs) == 0 {
			continue
		}

		var existing int64
		DB.Model(&AffiliateFraudAlert{}).
			Where("inviter_id = ? AND invitee_id = ? AND status = ?", inviterId, inviteeId, FraudAlertStatusDetected).
			Count(&existing)
		if existing > 0 {
			continue
		}

		ipsJSON, _ := common.Marshal(sharedIPs)
		alert := &AffiliateFraudAlert{
			InviterId:     inviterId,
			InviteeId:     inviteeId,
			SharedIps:     string(ipsJSON),
			SharedIpCount: len(sharedIPs),
			Status:        FraudAlertStatusDetected,
			DetectedAt:    common.GetTimestamp(),
		}
		if err := DB.Create(alert).Error; err != nil {
			continue
		}
		newAlerts++
	}
	return newAlerts, nil
}

func DetectFraudBulk() (int, error) {
	var inviterIds []int
	if err := DB.Model(&User{}).
		Where("aff_count > 0").
		Pluck("id", &inviterIds).Error; err != nil {
		return 0, err
	}

	totalNew := 0
	for _, inviterId := range inviterIds {
		n, err := DetectFraudForInviter(inviterId)
		if err != nil {
			continue
		}
		totalNew += n
	}
	return totalNew, nil
}

func UnbindAffiliateRelationship(alertId, adminId int, doClawback bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var alert AffiliateFraudAlert
		if err := tx.Where("id = ? AND status = ?", alertId, FraudAlertStatusDetected).First(&alert).Error; err != nil {
			return errors.New("alert not found or already resolved")
		}

		clawbackAmount := 0
		if doClawback {
			amount, err := clawbackEarnings(tx, alert.InviterId, alert.InviteeId)
			if err != nil {
				return err
			}
			clawbackAmount = amount
		}

		if err := tx.Model(&User{}).Where("id = ?", alert.InviteeId).
			Update("inviter_id", 0).Error; err != nil {
			return err
		}
		tx.Model(&User{}).Where("id = ? AND aff_count > 0", alert.InviterId).
			UpdateColumn("aff_count", gorm.Expr("aff_count - 1"))

		action := FraudActionUnbind
		if doClawback {
			action = FraudActionClawback
		}

		return tx.Model(&alert).Updates(map[string]interface{}{
			"status":          FraudAlertStatusResolved,
			"resolved_action": action,
			"clawback_quota":  clawbackAmount,
			"admin_id":        adminId,
			"resolved_at":     common.GetTimestamp(),
		}).Error
	})
}

func clawbackEarnings(tx *gorm.DB, inviterId, inviteeId int) (int, error) {
	var records []AffiliateRecord
	if err := tx.Where("user_id = ? AND invitee_id = ?", inviterId, inviteeId).
		Find(&records).Error; err != nil {
		return 0, err
	}

	totalPending := 0
	totalAvailable := 0
	for _, r := range records {
		if r.Status == AffiliateRecordStatusPending {
			totalPending += r.RewardQuota
		} else {
			totalAvailable += r.RewardQuota
		}
	}

	if err := tx.Where("user_id = ? AND invitee_id = ?", inviterId, inviteeId).
		Delete(&AffiliateRecord{}).Error; err != nil {
		return 0, err
	}

	if totalPending > 0 || totalAvailable > 0 {
		var balance AffiliateBalance
		if err := tx.Where("user_id = ?", inviterId).First(&balance).Error; err == nil {
			updates := map[string]interface{}{}
			newPending := balance.PendingQuota - totalPending
			if newPending < 0 {
				newPending = 0
			}
			updates["pending_quota"] = newPending

			newAvailable := balance.AvailableQuota - totalAvailable
			if newAvailable < 0 {
				newAvailable = 0
			}
			updates["available_quota"] = newAvailable

			newTotal := balance.TotalQuota - totalPending - totalAvailable
			if newTotal < 0 {
				newTotal = 0
			}
			updates["total_quota"] = newTotal

			tx.Model(&balance).Updates(updates)
		}
	}

	return totalPending + totalAvailable, nil
}

func DismissFraudAlert(alertId, adminId int, remark string) error {
	var alert AffiliateFraudAlert
	if err := DB.Where("id = ? AND status = ?", alertId, FraudAlertStatusDetected).First(&alert).Error; err != nil {
		return errors.New("alert not found or already resolved")
	}

	return DB.Model(&alert).Updates(map[string]interface{}{
		"status":          FraudAlertStatusDismissed,
		"resolved_action": FraudActionDismiss,
		"admin_id":        adminId,
		"admin_remark":    remark,
		"resolved_at":     common.GetTimestamp(),
	}).Error
}

func GetFraudAlerts(status string, page, pageSize int) ([]FraudAlertWithUsers, int64, error) {
	var total int64
	var alerts []AffiliateFraudAlert

	query := DB.Model(&AffiliateFraudAlert{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("detected_at DESC").Offset(offset).Limit(pageSize).Find(&alerts).Error; err != nil {
		return nil, 0, err
	}

	result := make([]FraudAlertWithUsers, 0, len(alerts))
	for _, alert := range alerts {
		item := FraudAlertWithUsers{AffiliateFraudAlert: alert}
		var inviter, invitee User
		if DB.Select("username").Where("id = ?", alert.InviterId).First(&inviter).Error == nil {
			item.InviterUsername = inviter.Username
		}
		if DB.Select("username").Where("id = ?", alert.InviteeId).First(&invitee).Error == nil {
			item.InviteeUsername = invitee.Username
		}
		result = append(result, item)
	}

	return result, total, nil
}
