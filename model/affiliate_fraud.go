package model

import (
	"errors"
	"strings"

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

type FraudAlertQuery struct {
	Status   string
	IP       string
	Keyword  string
	Page     int
	PageSize int
}

func fraudDetectionSinceTimestamp(days int) int64 {
	if days <= 0 {
		return 0
	}
	return common.GetTimestamp() - int64(days)*86400
}

func DetectFraudForInviter(inviterId int, days int) (int, error) {
	var inviteeIds []int
	if err := DB.Model(&User{}).Where("inviter_id = ?", inviterId).Pluck("id", &inviteeIds).Error; err != nil {
		return 0, err
	}
	if len(inviteeIds) == 0 {
		return 0, nil
	}

	overlaps, err := GetIPOverlapBatch(inviterId, inviteeIds, fraudDetectionSinceTimestamp(days))
	if err != nil {
		return 0, err
	}
	if overlaps == nil {
		overlaps = make(map[int][]string)
	}
	for _, inviteeId := range inviteeIds {
		if _, ok := overlaps[inviteeId]; !ok {
			overlaps[inviteeId] = nil
		}
	}

	newAlerts := 0
	for inviteeId, sharedIPs := range overlaps {
		created, err := upsertFraudAlertForPair(inviterId, inviteeId, sharedIPs)
		if err != nil {
			continue
		}
		if created {
			newAlerts++
		}
	}
	_ = refreshDetectedFraudAlertsForInviter(inviterId, overlaps)
	return newAlerts, nil
}

func DetectFraudBulk(days int) (int, error) {
	var inviterIds []int
	if err := DB.Model(&User{}).
		Where("aff_count > 0").
		Pluck("id", &inviterIds).Error; err != nil {
		return 0, err
	}

	totalNew := 0
	for _, inviterId := range inviterIds {
		n, err := DetectFraudForInviter(inviterId, days)
		if err != nil {
			continue
		}
		totalNew += n
	}
	return totalNew, nil
}

// DetectFraudDeep scans the logs table (LOG_DB) for IP overlaps between
// inviters and their invitees. This catches historical activity that happened
// before the user_ip_records table was introduced.
func DetectFraudDeep(days int) (int, error) {
	var inviterIds []int
	if err := DB.Model(&User{}).
		Where("aff_count > 0").
		Pluck("id", &inviterIds).Error; err != nil {
		return 0, err
	}

	totalNew := 0
	for _, inviterId := range inviterIds {
		n, err := detectFraudDeepForInviter(inviterId, days)
		if err != nil {
			continue
		}
		totalNew += n
	}
	return totalNew, nil
}

func detectFraudDeepForInviter(inviterId int, days int) (int, error) {
	var inviteeIds []int
	if err := DB.Model(&User{}).Where("inviter_id = ?", inviterId).Pluck("id", &inviteeIds).Error; err != nil {
		return 0, err
	}
	if len(inviteeIds) == 0 {
		return 0, nil
	}

	// Get inviter's distinct IPs from logs
	var inviterIPs []string
	sinceTimestamp := fraudDetectionSinceTimestamp(days)
	inviterLogQuery := LOG_DB.Model(&Log{}).
		Where("user_id = ? AND ip != '' AND type != ?", inviterId, LogTypeTopup).
		Distinct("ip")
	if sinceTimestamp > 0 {
		inviterLogQuery = inviterLogQuery.Where("created_at >= ?", sinceTimestamp)
	}
	if err := inviterLogQuery.Pluck("ip", &inviterIPs).Error; err != nil {
		return 0, err
	}
	inviterIPs = filterAffiliateFraudIPs(inviterIPs)

	// For each invitee, check IP overlap from logs
	newAlerts := 0
	currentOverlaps := make(map[int][]string)
	for _, inviteeId := range inviteeIds {
		var sharedIPs []string
		if len(inviterIPs) > 0 {
			inviteeLogQuery := LOG_DB.Model(&Log{}).
				Where("user_id = ? AND ip IN ? AND ip != '' AND type != ?", inviteeId, inviterIPs, LogTypeTopup).
				Distinct("ip")
			if sinceTimestamp > 0 {
				inviteeLogQuery = inviteeLogQuery.Where("created_at >= ?", sinceTimestamp)
			}
			if err := inviteeLogQuery.Pluck("ip", &sharedIPs).Error; err != nil {
				continue
			}
		}

		// Also merge with user_ip_records overlaps
		ipRecordOverlaps, _ := GetIPOverlap(inviterId, inviteeId, sinceTimestamp)
		allShared := mergeUniqueStrings(sharedIPs, ipRecordOverlaps)
		currentOverlaps[inviteeId] = allShared
		if len(allShared) == 0 {
			continue
		}

		created, err := upsertFraudAlertForPair(inviterId, inviteeId, allShared)
		if err != nil {
			continue
		}
		if created {
			newAlerts++
		}
	}
	_ = refreshDetectedFraudAlertsForInviter(inviterId, currentOverlaps)
	return newAlerts, nil
}

func mergeUniqueStrings(a, b []string) []string {
	return filterAffiliateFraudIPs(append(a, b...))
}

func upsertFraudAlertForPair(inviterId, inviteeId int, sharedIPs []string) (bool, error) {
	sharedIPs = filterAffiliateFraudIPs(sharedIPs)
	if len(sharedIPs) == 0 {
		return false, deleteDetectedFraudAlertForPair(inviterId, inviteeId)
	}

	ipsJSON, _ := common.Marshal(sharedIPs)
	var alert AffiliateFraudAlert
	err := DB.Where("inviter_id = ? AND invitee_id = ? AND status = ?", inviterId, inviteeId, FraudAlertStatusDetected).
		First(&alert).Error
	if err == nil {
		return false, DB.Model(&alert).Updates(map[string]interface{}{
			"shared_ips":      string(ipsJSON),
			"shared_ip_count": len(sharedIPs),
			"resolved_action": "",
			"resolved_at":     0,
			"admin_remark":    "",
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}

	alert = AffiliateFraudAlert{
		InviterId:     inviterId,
		InviteeId:     inviteeId,
		SharedIps:     string(ipsJSON),
		SharedIpCount: len(sharedIPs),
		Status:        FraudAlertStatusDetected,
		DetectedAt:    common.GetTimestamp(),
	}
	if err := DB.Create(&alert).Error; err != nil {
		return false, err
	}
	return true, nil
}

func deleteDetectedFraudAlertForPair(inviterId, inviteeId int) error {
	return DB.Where("inviter_id = ? AND invitee_id = ? AND status = ?", inviterId, inviteeId, FraudAlertStatusDetected).
		Delete(&AffiliateFraudAlert{}).Error
}

func refreshDetectedFraudAlertsForInviter(inviterId int, overlaps map[int][]string) error {
	var alerts []AffiliateFraudAlert
	if err := DB.Where("inviter_id = ? AND status = ?", inviterId, FraudAlertStatusDetected).Find(&alerts).Error; err != nil {
		return err
	}
	for _, alert := range alerts {
		sharedIPs := filterAffiliateFraudIPs(overlaps[alert.InviteeId])
		if len(sharedIPs) == 0 {
			if err := deleteDetectedFraudAlertForPair(alert.InviterId, alert.InviteeId); err != nil {
				return err
			}
			continue
		}
		ipsJSON, _ := common.Marshal(sharedIPs)
		if err := DB.Model(&alert).Updates(map[string]interface{}{
			"shared_ips":      string(ipsJSON),
			"shared_ip_count": len(sharedIPs),
		}).Error; err != nil {
			return err
		}
	}
	return nil
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
		if err := tx.Model(&User{}).Where("id = ? AND aff_count > 0", alert.InviterId).
			Update("aff_count", gorm.Expr("aff_count - ?", 1)).Error; err != nil {
			return err
		}

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

func DeleteFraudAlert(alertId int) error {
	if alertId <= 0 {
		return errors.New("invalid alert ID")
	}
	result := DB.Delete(&AffiliateFraudAlert{}, alertId)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("alert not found")
	}
	return nil
}

func GetFraudAlerts(status string, page, pageSize int) ([]FraudAlertWithUsers, int64, error) {
	return SearchFraudAlerts(FraudAlertQuery{Status: status, Page: page, PageSize: pageSize})
}

func SearchFraudAlerts(params FraudAlertQuery) ([]FraudAlertWithUsers, int64, error) {
	var total int64
	var alerts []AffiliateFraudAlert

	query := DB.Model(&AffiliateFraudAlert{})
	if status := strings.TrimSpace(params.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if ip := strings.TrimSpace(params.IP); ip != "" {
		pattern, err := sanitizeLikePattern(ip)
		if err != nil {
			return nil, 0, err
		}
		if !strings.Contains(pattern, "%") {
			pattern = "%" + pattern + "%"
		}
		query = query.Where("shared_ips LIKE ? ESCAPE '!'", pattern)
	}
	if keyword := strings.TrimSpace(params.Keyword); keyword != "" {
		userIds, err := findAffiliateAdminMatchedUserIds(keyword)
		if err != nil {
			return nil, 0, err
		}
		if len(userIds) == 0 {
			return []FraudAlertWithUsers{}, 0, nil
		}
		query = query.Where("inviter_id IN ? OR invitee_id IN ?", userIds, userIds)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 50
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
