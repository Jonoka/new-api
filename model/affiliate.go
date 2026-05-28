package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	AffiliateSourceTopUp        = "topup"
	AffiliateSourceSubscription = "subscription"
)

const (
	AffiliateRecordStatusPending   = "pending"
	AffiliateRecordStatusAvailable = "available"
)

const (
	AffiliatePayoutMethodUSDT   = "usdt"
	AffiliatePayoutMethodAlipay = "alipay"
	AffiliatePayoutMethodWechat = "wechat"
)

const (
	AffiliateWithdrawalStatusPending  = "pending"
	AffiliateWithdrawalStatusApproved = "approved"
	AffiliateWithdrawalStatusPaid     = "paid"
	AffiliateWithdrawalStatusRejected = "rejected"
)

type AffiliateRecord struct {
	Id            int    `json:"id"`
	UserId        int    `json:"user_id" gorm:"index;uniqueIndex:idx_affiliate_record_source,priority:3"`
	InviteeId     int    `json:"invitee_id" gorm:"index"`
	Level         int    `json:"level" gorm:"index;uniqueIndex:idx_affiliate_record_source,priority:4"`
	SourceType    string `json:"source_type" gorm:"type:varchar(32);index;uniqueIndex:idx_affiliate_record_source,priority:1"`
	SourceId      string `json:"source_id" gorm:"type:varchar(255);index;uniqueIndex:idx_affiliate_record_source,priority:2"`
	SourceQuota   int    `json:"source_quota"`
	RewardQuota   int    `json:"reward_quota"`
	Ratio         int    `json:"ratio"`
	Status        string `json:"status" gorm:"type:varchar(32);index"`
	AvailableTime int64  `json:"available_time" gorm:"index"`
	SettledTime   int64  `json:"settled_time"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type AffiliateBalance struct {
	Id               int   `json:"id"`
	UserId           int   `json:"user_id" gorm:"uniqueIndex"`
	PendingQuota     int   `json:"pending_quota"`
	AvailableQuota   int   `json:"available_quota"`
	FrozenQuota      int   `json:"frozen_quota"`
	WithdrawnQuota   int   `json:"withdrawn_quota"`
	TransferredQuota int   `json:"transferred_quota"`
	TotalQuota       int   `json:"total_quota"`
	CreatedAt        int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

type AffiliatePayoutAccount struct {
	Id            int    `json:"id"`
	UserId        int    `json:"user_id" gorm:"uniqueIndex"`
	UsdtAddress   string `json:"usdt_address" gorm:"type:varchar(255)"`
	UsdtChain     string `json:"usdt_chain" gorm:"type:varchar(32)"`
	AlipayAccount string `json:"alipay_account" gorm:"type:varchar(255)"`
	AlipayName    string `json:"alipay_name" gorm:"type:varchar(255)"`
	AlipayQrPath  string `json:"alipay_qr_path" gorm:"type:varchar(255)"`
	WechatAccount string `json:"wechat_account" gorm:"type:varchar(255)"`
	WechatName    string `json:"wechat_name" gorm:"type:varchar(255)"`
	WechatQrPath  string `json:"wechat_qr_path" gorm:"type:varchar(255)"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type AffiliateWithdrawal struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Quota           int     `json:"quota"`
	DisplayAmount   float64 `json:"display_amount"`
	DisplayCurrency string  `json:"display_currency" gorm:"type:varchar(32)"`
	Method          string  `json:"method" gorm:"type:varchar(32);index"`
	PayoutSnapshot  string  `json:"payout_snapshot" gorm:"type:text"`
	Status          string  `json:"status" gorm:"type:varchar(32);index"`
	AdminId         int     `json:"admin_id"`
	AdminRemark     string  `json:"admin_remark" gorm:"type:varchar(500)"`
	ApprovedTime    int64   `json:"approved_time"`
	PaidTime        int64   `json:"paid_time"`
	RejectedTime    int64   `json:"rejected_time"`
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

type AffiliateLeaderboardItem struct {
	Rank            int    `json:"rank"`
	UserId          int    `json:"user_id"`
	Username        string `json:"username"`
	DisplayName     string `json:"display_name"`
	InviteCount     int    `json:"invite_count"`
	CommissionQuota int    `json:"commission_quota"`
}

func isAffiliateSourceEnabled(sourceType string) bool {
	affiliateSetting := setting.GetAffiliateSetting()
	switch sourceType {
	case AffiliateSourceTopUp:
		return affiliateSetting.TriggerTopupEnabled
	case AffiliateSourceSubscription:
		return affiliateSetting.TriggerSubscriptionEnabled
	default:
		return false
	}
}

func normalizeAffiliatePayoutMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case AffiliatePayoutMethodUSDT:
		return AffiliatePayoutMethodUSDT
	case AffiliatePayoutMethodAlipay:
		return AffiliatePayoutMethodAlipay
	case AffiliatePayoutMethodWechat:
		return AffiliatePayoutMethodWechat
	default:
		return ""
	}
}

func getAffiliateBalanceForUpdateTx(tx *gorm.DB, userId int) (*AffiliateBalance, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	balance := &AffiliateBalance{}
	err := tx.Set("gorm:query_option", "FOR UPDATE").Where("user_id = ?", userId).First(balance).Error
	if err == nil {
		return balance, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	balance.UserId = userId
	if err := tx.Create(balance).Error; err != nil {
		return nil, err
	}
	return balance, nil
}

func GetAffiliateBalance(userId int) (*AffiliateBalance, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	balance := &AffiliateBalance{}
	err := DB.Where("user_id = ?", userId).First(balance).Error
	if err == nil {
		return balance, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	balance.UserId = userId
	if err := DB.Create(balance).Error; err != nil {
		return nil, err
	}
	return balance, nil
}

func CreateAffiliateRewardsForPayment(inviteeId int, sourceType string, sourceId string, sourceQuota int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return createAffiliateRewardsForPaymentTx(tx, inviteeId, sourceType, sourceId, sourceQuota)
	})
}

func createAffiliateRewardsForPaymentTx(tx *gorm.DB, inviteeId int, sourceType string, sourceId string, sourceQuota int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	sourceId = strings.TrimSpace(sourceId)
	if inviteeId <= 0 || sourceId == "" || sourceQuota <= 0 {
		return nil
	}
	if !isAffiliateSourceEnabled(sourceType) {
		return nil
	}

	var invitee User
	if err := tx.Select("id", "inviter_id").Where("id = ?", inviteeId).First(&invitee).Error; err != nil {
		return err
	}
	if invitee.InviterId <= 0 {
		return nil
	}

	affiliateSetting := setting.GetAffiliateSetting()
	if affiliateSetting.FirstLevelEnabled && affiliateSetting.FirstLevelRatio > 0 {
		if err := createAffiliateRewardRecordTx(tx, invitee.InviterId, inviteeId, 1, sourceType, sourceId, sourceQuota, affiliateSetting.FirstLevelRatio); err != nil {
			return err
		}
	}

	if !affiliateSetting.SecondLevelEnabled || affiliateSetting.SecondLevelRatio <= 0 {
		return nil
	}

	var parent User
	if err := tx.Select("id", "inviter_id").Where("id = ?", invitee.InviterId).First(&parent).Error; err != nil {
		return err
	}
	if parent.InviterId <= 0 {
		return nil
	}
	return createAffiliateRewardRecordTx(tx, parent.InviterId, inviteeId, 2, sourceType, sourceId, sourceQuota, affiliateSetting.SecondLevelRatio)
}

func createAffiliateRewardRecordTx(tx *gorm.DB, userId int, inviteeId int, level int, sourceType string, sourceId string, sourceQuota int, ratio int) error {
	if userId <= 0 || rewardRatioInvalid(ratio) {
		return nil
	}
	rewardQuota := sourceQuota * ratio / 100
	if rewardQuota <= 0 {
		return nil
	}

	var existing AffiliateRecord
	err := tx.Where("source_type = ? AND source_id = ? AND user_id = ? AND level = ?", sourceType, sourceId, userId, level).
		First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	now := common.GetTimestamp()
	record := &AffiliateRecord{
		UserId:        userId,
		InviteeId:     inviteeId,
		Level:         level,
		SourceType:    sourceType,
		SourceId:      sourceId,
		SourceQuota:   sourceQuota,
		RewardQuota:   rewardQuota,
		Ratio:         ratio,
		Status:        AffiliateRecordStatusPending,
		AvailableTime: now + setting.GetAffiliateSetting().SettlementDelaySeconds,
	}
	if err := tx.Create(record).Error; err != nil {
		return err
	}

	balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
	if err != nil {
		return err
	}
	balance.PendingQuota += rewardQuota
	balance.TotalQuota += rewardQuota
	return tx.Save(balance).Error
}

func rewardRatioInvalid(ratio int) bool {
	return ratio <= 0 || ratio > 100
}

func SettleMatureAffiliateRecords(userId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return settleMatureAffiliateRecordsTx(tx, userId)
	})
}

func settleMatureAffiliateRecordsTx(tx *gorm.DB, userId int) error {
	now := common.GetTimestamp()
	query := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("status = ? AND available_time <= ?", AffiliateRecordStatusPending, now)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}

	var records []AffiliateRecord
	if err := query.Find(&records).Error; err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	quotaByUser := make(map[int]int)
	recordIds := make([]int, 0, len(records))
	for _, record := range records {
		quotaByUser[record.UserId] += record.RewardQuota
		recordIds = append(recordIds, record.Id)
	}

	if err := tx.Model(&AffiliateRecord{}).Where("id IN ?", recordIds).Updates(map[string]interface{}{
		"status":       AffiliateRecordStatusAvailable,
		"settled_time": now,
	}).Error; err != nil {
		return err
	}

	for uid, quota := range quotaByUser {
		balance, err := getAffiliateBalanceForUpdateTx(tx, uid)
		if err != nil {
			return err
		}
		balance.PendingQuota -= quota
		if balance.PendingQuota < 0 {
			balance.PendingQuota = 0
		}
		balance.AvailableQuota += quota
		if err := tx.Save(balance).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetAffiliateRecords(userId int, status string, pageInfo *common.PageInfo) ([]*AffiliateRecord, int64, error) {
	if err := SettleMatureAffiliateRecords(userId); err != nil {
		return nil, 0, err
	}
	query := DB.Model(&AffiliateRecord{}).Where("user_id = ?", userId)
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []*AffiliateRecord
	err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&records).Error
	return records, total, err
}

func GetAffiliateWithdrawals(userId int, pageInfo *common.PageInfo) ([]*AffiliateWithdrawal, int64, error) {
	query := DB.Model(&AffiliateWithdrawal{}).Where("user_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var withdrawals []*AffiliateWithdrawal
	err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&withdrawals).Error
	return withdrawals, total, err
}

func GetAllAffiliateWithdrawals(status string, pageInfo *common.PageInfo) ([]*AffiliateWithdrawal, int64, error) {
	query := DB.Model(&AffiliateWithdrawal{})
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var withdrawals []*AffiliateWithdrawal
	err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&withdrawals).Error
	return withdrawals, total, err
}

func GetAffiliatePayoutAccount(userId int) (*AffiliatePayoutAccount, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	account := &AffiliatePayoutAccount{}
	err := DB.Where("user_id = ?", userId).First(account).Error
	if err == nil {
		if account.UsdtChain == "" {
			account.UsdtChain = setting.GetAffiliateSetting().UsdtChain
		}
		return account, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &AffiliatePayoutAccount{
		UserId:    userId,
		UsdtChain: setting.GetAffiliateSetting().UsdtChain,
	}, nil
}

func SaveAffiliatePayoutAccount(account *AffiliatePayoutAccount) error {
	if account == nil || account.UserId <= 0 {
		return errors.New("invalid payout account")
	}
	account.UsdtAddress = strings.TrimSpace(account.UsdtAddress)
	account.AlipayAccount = strings.TrimSpace(account.AlipayAccount)
	account.AlipayName = strings.TrimSpace(account.AlipayName)
	account.AlipayQrPath = strings.TrimSpace(account.AlipayQrPath)
	account.WechatAccount = strings.TrimSpace(account.WechatAccount)
	account.WechatName = strings.TrimSpace(account.WechatName)
	account.WechatQrPath = strings.TrimSpace(account.WechatQrPath)
	account.UsdtChain = strings.TrimSpace(account.UsdtChain)
	if account.UsdtChain == "" {
		account.UsdtChain = setting.GetAffiliateSetting().UsdtChain
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		existing := &AffiliatePayoutAccount{}
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where("user_id = ?", account.UserId).First(existing).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return tx.Create(account).Error
			}
			return err
		}
		account.Id = existing.Id
		account.CreatedAt = existing.CreatedAt
		return tx.Save(account).Error
	})
}

func CreateAffiliateWithdrawal(userId int, method string, quota int) (*AffiliateWithdrawal, error) {
	if quota <= 0 {
		return nil, errors.New("提现额度必须大于 0")
	}
	if minAmount := setting.GetAffiliateSetting().MinWithdrawalAmount; minAmount > 0 {
		minQuota := affiliateDisplayAmountToQuota(minAmount)
		if minQuota > 0 && quota < minQuota {
			return nil, fmt.Errorf("提现金额不能小于 %d", minAmount)
		}
	}
	method = normalizeAffiliatePayoutMethod(method)
	if method == "" {
		return nil, errors.New("无效的提现方式")
	}
	if err := SettleMatureAffiliateRecords(userId); err != nil {
		return nil, err
	}

	var withdrawal *AffiliateWithdrawal
	err := DB.Transaction(func(tx *gorm.DB) error {
		balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
		if err != nil {
			return err
		}
		if balance.AvailableQuota < quota {
			return errors.New("可提现额度不足")
		}

		snapshot, err := buildAffiliatePayoutSnapshotTx(tx, userId, method)
		if err != nil {
			return err
		}
		withdrawal = &AffiliateWithdrawal{
			UserId:          userId,
			Quota:           quota,
			DisplayAmount:   float64(quota) / common.QuotaPerUnit,
			DisplayCurrency: "USD",
			Method:          method,
			PayoutSnapshot:  snapshot,
			Status:          AffiliateWithdrawalStatusPending,
		}
		if err := tx.Create(withdrawal).Error; err != nil {
			return err
		}

		balance.AvailableQuota -= quota
		balance.FrozenQuota += quota
		return tx.Save(balance).Error
	})
	return withdrawal, err
}

func affiliateDisplayAmountToQuota(amount int) int {
	if amount <= 0 {
		return 0
	}
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return amount
	}
	rate := operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
	if rate <= 0 {
		rate = 1
	}
	return int(decimal.NewFromInt(int64(amount)).
		Div(decimal.NewFromFloat(rate)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		IntPart())
}

func buildAffiliatePayoutSnapshotTx(tx *gorm.DB, userId int, method string) (string, error) {
	var account AffiliatePayoutAccount
	err := tx.Where("user_id = ?", userId).First(&account).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if account.UsdtChain == "" {
		account.UsdtChain = setting.GetAffiliateSetting().UsdtChain
	}
	snapshot := map[string]interface{}{
		"method": method,
	}
	switch method {
	case AffiliatePayoutMethodUSDT:
		if strings.TrimSpace(account.UsdtAddress) == "" {
			return "", errors.New("请先填写 USDT 收款地址")
		}
		snapshot["usdt_address"] = account.UsdtAddress
		snapshot["usdt_chain"] = account.UsdtChain
	case AffiliatePayoutMethodAlipay:
		if strings.TrimSpace(account.AlipayAccount) == "" && strings.TrimSpace(account.AlipayQrPath) == "" {
			return "", errors.New("请先填写支付宝账号或上传支付宝收款码")
		}
		snapshot["alipay_account"] = account.AlipayAccount
		snapshot["alipay_name"] = account.AlipayName
		snapshot["alipay_qr_path"] = account.AlipayQrPath
	case AffiliatePayoutMethodWechat:
		if strings.TrimSpace(account.WechatAccount) == "" && strings.TrimSpace(account.WechatQrPath) == "" {
			return "", errors.New("请先填写微信账号或上传微信收款码")
		}
		snapshot["wechat_account"] = account.WechatAccount
		snapshot["wechat_name"] = account.WechatName
		snapshot["wechat_qr_path"] = account.WechatQrPath
	}
	bytes, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func ApproveAffiliateWithdrawal(withdrawalId int, adminId int, remark string) error {
	return updateAffiliateWithdrawalStatus(withdrawalId, adminId, remark, AffiliateWithdrawalStatusApproved)
}

func RejectAffiliateWithdrawal(withdrawalId int, adminId int, remark string) error {
	return updateAffiliateWithdrawalStatus(withdrawalId, adminId, remark, AffiliateWithdrawalStatusRejected)
}

func MarkAffiliateWithdrawalPaid(withdrawalId int, adminId int, remark string) error {
	return updateAffiliateWithdrawalStatus(withdrawalId, adminId, remark, AffiliateWithdrawalStatusPaid)
}

func updateAffiliateWithdrawalStatus(withdrawalId int, adminId int, remark string, targetStatus string) error {
	if withdrawalId <= 0 {
		return errors.New("invalid withdrawal id")
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		withdrawal := &AffiliateWithdrawal{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", withdrawalId).First(withdrawal).Error; err != nil {
			return err
		}
		if withdrawal.Status == targetStatus {
			return nil
		}
		if withdrawal.Status == AffiliateWithdrawalStatusPaid || withdrawal.Status == AffiliateWithdrawalStatusRejected {
			return errors.New("提现申请已完结")
		}

		balance, err := getAffiliateBalanceForUpdateTx(tx, withdrawal.UserId)
		if err != nil {
			return err
		}
		withdrawal.AdminId = adminId
		withdrawal.AdminRemark = strings.TrimSpace(remark)
		withdrawal.Status = targetStatus

		switch targetStatus {
		case AffiliateWithdrawalStatusApproved:
			withdrawal.ApprovedTime = now
		case AffiliateWithdrawalStatusRejected:
			withdrawal.RejectedTime = now
			balance.FrozenQuota -= withdrawal.Quota
			if balance.FrozenQuota < 0 {
				balance.FrozenQuota = 0
			}
			balance.AvailableQuota += withdrawal.Quota
		case AffiliateWithdrawalStatusPaid:
			withdrawal.PaidTime = now
			balance.FrozenQuota -= withdrawal.Quota
			if balance.FrozenQuota < 0 {
				balance.FrozenQuota = 0
			}
			balance.WithdrawnQuota += withdrawal.Quota
		default:
			return errors.New("无效的提现状态")
		}

		if err := tx.Save(balance).Error; err != nil {
			return err
		}
		return tx.Save(withdrawal).Error
	})
}

func TransferAffiliateQuotaToBalance(userId int, quota int) error {
	if quota <= 0 {
		return errors.New("转入额度必须大于 0")
	}
	if err := SettleMatureAffiliateRecords(userId); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
		if err != nil {
			return err
		}
		if balance.AvailableQuota < quota {
			return errors.New("可转入额度不足")
		}
		balance.AvailableQuota -= quota
		balance.TransferredQuota += quota
		if err := tx.Save(balance).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", quota)).Error
	})
}

func affiliateLeaderboardPeriodStart(period string) int64 {
	now := time.Now()
	year, month, day := now.Date()
	location := now.Location()
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "day":
		return time.Date(year, month, day, 0, 0, 0, 0, location).Unix()
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := time.Date(year, month, day, 0, 0, 0, 0, location).
			AddDate(0, 0, -(weekday - 1))
		return start.Unix()
	default:
		return time.Date(year, month, 1, 0, 0, 0, 0, location).Unix()
	}
}

func GetAffiliateLeaderboard(period string, limit int) ([]AffiliateLeaderboardItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	startTime := affiliateLeaderboardPeriodStart(period)

	type inviteRow struct {
		UserId      int
		InviteCount int
	}
	type commissionRow struct {
		UserId          int
		CommissionQuota int
	}

	var inviteRows []inviteRow
	if err := DB.Model(&User{}).
		Select("inviter_id AS user_id, COUNT(*) AS invite_count").
		Where("inviter_id > 0 AND created_at >= ?", startTime).
		Group("inviter_id").
		Scan(&inviteRows).Error; err != nil {
		return nil, err
	}

	var commissionRows []commissionRow
	if err := DB.Model(&AffiliateRecord{}).
		Select("user_id, COALESCE(SUM(reward_quota), 0) AS commission_quota").
		Where("created_at >= ?", startTime).
		Group("user_id").
		Scan(&commissionRows).Error; err != nil {
		return nil, err
	}

	itemMap := make(map[int]*AffiliateLeaderboardItem)
	for _, row := range inviteRows {
		if row.UserId <= 0 {
			continue
		}
		item := itemMap[row.UserId]
		if item == nil {
			item = &AffiliateLeaderboardItem{UserId: row.UserId}
			itemMap[row.UserId] = item
		}
		item.InviteCount = row.InviteCount
	}
	for _, row := range commissionRows {
		if row.UserId <= 0 {
			continue
		}
		item := itemMap[row.UserId]
		if item == nil {
			item = &AffiliateLeaderboardItem{UserId: row.UserId}
			itemMap[row.UserId] = item
		}
		item.CommissionQuota = row.CommissionQuota
	}

	if len(itemMap) == 0 {
		return []AffiliateLeaderboardItem{}, nil
	}

	userIds := make([]int, 0, len(itemMap))
	for userId := range itemMap {
		userIds = append(userIds, userId)
	}
	var users []User
	if err := DB.Select("id", "username", "display_name").Where("id IN ?", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		if item := itemMap[user.Id]; item != nil {
			item.Username = user.Username
			item.DisplayName = user.DisplayName
		}
	}

	items := make([]AffiliateLeaderboardItem, 0, len(itemMap))
	for _, item := range itemMap {
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CommissionQuota != items[j].CommissionQuota {
			return items[i].CommissionQuota > items[j].CommissionQuota
		}
		if items[i].InviteCount != items[j].InviteCount {
			return items[i].InviteCount > items[j].InviteCount
		}
		return items[i].UserId < items[j].UserId
	})
	if len(items) > limit {
		items = items[:limit]
	}
	for i := range items {
		items[i].Rank = i + 1
	}
	return items, nil
}
