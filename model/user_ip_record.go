package model

import (
	"net"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
)

type UserIPRecord struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"index:idx_user_ip,priority:1;index"`
	Ip        string `json:"ip" gorm:"type:varchar(45);index:idx_user_ip,priority:2;index"`
	Action    string `json:"action" gorm:"type:varchar(32);index"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

func (UserIPRecord) TableName() string {
	return "user_ip_records"
}

func RecordUserIP(userId int, ip string, action string) {
	if userId <= 0 || ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return
	}
	gopool.Go(func() {
		recordUserIPSync(userId, ip, action)
	})
}

func recordUserIPSync(userId int, ip string, action string) {
	now := common.GetTimestamp()
	oneHourAgo := now - 3600

	var count int64
	DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip = ? AND action = ? AND created_at > ?", userId, ip, action, oneHourAgo).
		Count(&count)
	if count > 0 {
		return
	}

	record := &UserIPRecord{
		UserId: userId,
		Ip:     ip,
		Action: action,
	}
	DB.Create(record)
}

func GetDistinctIPsByUserId(userId int) ([]string, error) {
	var ips []string
	err := DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip != ''", userId).
		Distinct("ip").
		Pluck("ip", &ips).Error
	return ips, err
}

func normalizeAffiliateFraudIP(ip string) (string, bool) {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsLoopback() || parsed.To4() == nil {
		return "", false
	}
	return parsed.String(), true
}

func filterAffiliateFraudIPs(ips []string) []string {
	seen := make(map[string]bool, len(ips))
	filtered := make([]string, 0, len(ips))
	for _, rawIP := range ips {
		ip, ok := normalizeAffiliateFraudIP(rawIP)
		if !ok || seen[ip] {
			continue
		}
		seen[ip] = true
		filtered = append(filtered, ip)
	}
	return filtered
}

func GetIPOverlap(userIdA, userIdB int, sinceTimestamp int64) ([]string, error) {
	var ipsA []string
	inviterQuery := DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip != ''", userIdA).
		Distinct("ip")
	if sinceTimestamp > 0 {
		inviterQuery = inviterQuery.Where("created_at >= ?", sinceTimestamp)
	}
	if err := inviterQuery.Pluck("ip", &ipsA).Error; err != nil {
		return nil, err
	}
	ipsA = filterAffiliateFraudIPs(ipsA)
	if len(ipsA) == 0 {
		return nil, nil
	}

	var shared []string
	inviteeQuery := DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip IN ?", userIdB, ipsA).
		Distinct("ip")
	if sinceTimestamp > 0 {
		inviteeQuery = inviteeQuery.Where("created_at >= ?", sinceTimestamp)
	}
	err := inviteeQuery.Pluck("ip", &shared).Error
	return filterAffiliateFraudIPs(shared), err
}

func GetIPOverlapBatch(inviterId int, inviteeIds []int, sinceTimestamp int64) (map[int][]string, error) {
	if len(inviteeIds) == 0 {
		return nil, nil
	}

	var inviterIPs []string
	inviterQuery := DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip != ''", inviterId).
		Distinct("ip")
	if sinceTimestamp > 0 {
		inviterQuery = inviterQuery.Where("created_at >= ?", sinceTimestamp)
	}
	if err := inviterQuery.Pluck("ip", &inviterIPs).Error; err != nil {
		return nil, err
	}
	inviterIPs = filterAffiliateFraudIPs(inviterIPs)
	if len(inviterIPs) == 0 {
		return nil, nil
	}

	type ipUserRow struct {
		UserId int
		Ip     string
	}
	var rows []ipUserRow
	inviteeQuery := DB.Model(&UserIPRecord{}).
		Select("DISTINCT user_id, ip").
		Where("user_id IN ? AND ip IN ?", inviteeIds, inviterIPs)
	if sinceTimestamp > 0 {
		inviteeQuery = inviteeQuery.Where("created_at >= ?", sinceTimestamp)
	}
	err := inviteeQuery.Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int][]string)
	for _, r := range rows {
		if ip, ok := normalizeAffiliateFraudIP(r.Ip); ok {
			result[r.UserId] = append(result[r.UserId], ip)
		}
	}
	return result, nil
}

func CleanOldIPRecords(beforeTimestamp int64) (int64, error) {
	result := DB.Where("created_at < ?", beforeTimestamp).Delete(&UserIPRecord{})
	return result.RowsAffected, result.Error
}

func GetUserIPRecordCount(userId int) (int64, error) {
	var count int64
	err := DB.Model(&UserIPRecord{}).Where("user_id = ?", userId).Count(&count).Error
	return count, err
}

func GetRecentIPsByUserId(userId int, limit int) ([]UserIPRecord, error) {
	var records []UserIPRecord
	err := DB.Where("user_id = ?", userId).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}
