package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	AffiliateRiskStatusActive  = "active"
	AffiliateRiskStatusRemoved = "removed"
)

const (
	AffiliateRiskEventApply           = "apply"
	AffiliateRiskEventRemove          = "remove"
	AffiliateRiskEventDetachInvitees  = "detach_invitees"
	AffiliateRiskEventRestoreInvitees = "restore_invitees"
	AffiliateRiskEventClearAssets     = "clear_assets"
)

type AffiliateRiskUser struct {
	Id               int    `json:"id" gorm:"primaryKey"`
	UserId           int    `json:"user_id" gorm:"uniqueIndex;index"`
	Status           string `json:"status" gorm:"type:varchar(32);index;default:active"`
	FreezeAssets     bool   `json:"freeze_assets" gorm:"index"`
	BlockInviteCode  bool   `json:"block_invite_code" gorm:"index"`
	DetachedInvitees bool   `json:"detached_invitees"`
	ClearedQuota     int    `json:"cleared_quota"`
	Reason           string `json:"reason" gorm:"type:varchar(500)"`
	AdminId          int    `json:"admin_id"`
	RemovedBy        int    `json:"removed_by"`
	RemoveRemark     string `json:"remove_remark" gorm:"type:varchar(500)"`
	RemovedAt        int64  `json:"removed_at"`
	CreatedAt        int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AffiliateRiskUser) TableName() string {
	return "affiliate_risk_users"
}

type AffiliateRiskEvent struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	RiskUserId int    `json:"risk_user_id" gorm:"index"`
	UserId     int    `json:"user_id" gorm:"index"`
	Action     string `json:"action" gorm:"type:varchar(64);index"`
	AdminId    int    `json:"admin_id"`
	Detail     string `json:"detail" gorm:"type:text"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AffiliateRiskEvent) TableName() string {
	return "affiliate_risk_events"
}

type AffiliateRiskDetachedInvitee struct {
	Id         int   `json:"id" gorm:"primaryKey"`
	RiskUserId int   `json:"risk_user_id" gorm:"index;uniqueIndex:idx_risk_detached_pair,priority:1"`
	UserId     int   `json:"user_id" gorm:"index"`
	InviteeId  int   `json:"invitee_id" gorm:"index;uniqueIndex:idx_risk_detached_pair,priority:2"`
	Restored   bool  `json:"restored" gorm:"index"`
	RestoredAt int64 `json:"restored_at"`
	CreatedAt  int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AffiliateRiskDetachedInvitee) TableName() string {
	return "affiliate_risk_detached_invitees"
}

type AffiliateRiskUserWithDetail struct {
	AffiliateRiskUser
	User                   AffiliateAdminUserInfo `json:"user"`
	Balance                AffiliateBalance       `json:"balance"`
	DirectInviteeCount     int                    `json:"direct_invitee_count"`
	RestorableInviteeCount int                    `json:"restorable_invitee_count"`
}

type AffiliateRiskPreview struct {
	User                   AffiliateAdminUserInfo `json:"user"`
	Balance                AffiliateBalance       `json:"balance"`
	ActiveRisk             *AffiliateRiskUser     `json:"active_risk,omitempty"`
	DirectInviteeCount     int                    `json:"direct_invitee_count"`
	RestorableInviteeCount int                    `json:"restorable_invitee_count"`
	ClearableQuota         int                    `json:"clearable_quota"`
}

type AffiliateRiskApplyRequest struct {
	FreezeAssets    bool   `json:"freeze_assets"`
	BlockInviteCode bool   `json:"block_invite_code"`
	DetachInvitees  bool   `json:"detach_invitees"`
	ClearAssets     bool   `json:"clear_assets"`
	Reason          string `json:"reason"`
}

type AffiliateRiskRemoveRequest struct {
	RestoreDetachedInvitees bool   `json:"restore_detached_invitees"`
	Remark                  string `json:"remark"`
}

type AffiliateRiskApplyResult struct {
	RiskUser            AffiliateRiskUser `json:"risk_user"`
	FrozenQuota         int               `json:"frozen_quota"`
	DetachedCount       int               `json:"detached_count"`
	ClearedQuota        int               `json:"cleared_quota"`
	RejectedWithdrawals int               `json:"rejected_withdrawals"`
}

type AffiliateRiskRemoveResult struct {
	RestoredInvitees int `json:"restored_invitees"`
	UnfrozenQuota    int `json:"unfrozen_quota"`
}

func affiliateBalanceSnapshotQuota(balance *AffiliateBalance) int {
	if balance == nil {
		return 0
	}
	return balance.PendingQuota + balance.AvailableQuota + balance.FrozenQuota + balance.RiskFrozenQuota
}

func IsAffiliateUserInviteCodeBlocked(userId int) bool {
	return isAffiliateUserInviteCodeBlockedWithDB(DB, userId)
}

func isAffiliateUserInviteCodeBlockedWithDB(db *gorm.DB, userId int) bool {
	if db == nil || userId <= 0 {
		return false
	}
	var count int64
	_ = db.Model(&AffiliateRiskUser{}).
		Where("user_id = ? AND status = ? AND block_invite_code = ?", userId, AffiliateRiskStatusActive, true).
		Count(&count).Error
	return count > 0
}

func IsAffiliateUserAssetsFrozenTx(tx *gorm.DB, userId int) bool {
	if tx == nil || userId <= 0 {
		return false
	}
	var count int64
	_ = tx.Model(&AffiliateRiskUser{}).
		Where("user_id = ? AND status = ? AND freeze_assets = ?", userId, AffiliateRiskStatusActive, true).
		Count(&count).Error
	return count > 0
}

func ListAffiliateRiskUsers(keyword string, status string, pageInfo *common.PageInfo) ([]AffiliateRiskUserWithDetail, int64, error) {
	query := DB.Model(&AffiliateRiskUser{})
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status = ?", AffiliateRiskStatusActive)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		userIds, err := findAffiliateAdminMatchedUserIds(keyword)
		if err != nil {
			return nil, 0, err
		}
		if len(userIds) == 0 {
			return []AffiliateRiskUserWithDetail{}, 0, nil
		}
		query = query.Where("user_id IN ?", userIds)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var risks []AffiliateRiskUser
	if err := query.Order("id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&risks).Error; err != nil {
		return nil, 0, err
	}
	if len(risks) == 0 {
		return []AffiliateRiskUserWithDetail{}, total, nil
	}

	userIds := make([]int, 0, len(risks))
	riskIds := make([]int, 0, len(risks))
	for _, risk := range risks {
		userIds = append(userIds, risk.UserId)
		riskIds = append(riskIds, risk.Id)
	}
	usersById, err := getAffiliateAdminUsersByIds(userIds)
	if err != nil {
		return nil, 0, err
	}
	balancesByUser, err := getAffiliateBalancesByUserIds(userIds)
	if err != nil {
		return nil, 0, err
	}
	inviteeCounts, err := getDirectInviteeCounts(userIds)
	if err != nil {
		return nil, 0, err
	}
	restorableCounts, err := getRestorableDetachedInviteeCounts(riskIds)
	if err != nil {
		return nil, 0, err
	}

	items := make([]AffiliateRiskUserWithDetail, 0, len(risks))
	for _, risk := range risks {
		items = append(items, AffiliateRiskUserWithDetail{
			AffiliateRiskUser:      risk,
			User:                   usersById[risk.UserId],
			Balance:                balancesByUser[risk.UserId],
			DirectInviteeCount:     inviteeCounts[risk.UserId],
			RestorableInviteeCount: restorableCounts[risk.Id],
		})
	}
	return items, total, nil
}

func GetAffiliateRiskPreview(userId int) (*AffiliateRiskPreview, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	usersById, err := getAffiliateAdminUsersByIds([]int{userId})
	if err != nil {
		return nil, err
	}
	user, ok := usersById[userId]
	if !ok || user.Id == 0 {
		return nil, errors.New("user not found")
	}
	balance, err := GetAffiliateBalance(userId)
	if err != nil {
		return nil, err
	}
	activeRisk, err := GetActiveAffiliateRiskUser(userId)
	if err != nil {
		return nil, err
	}
	inviteeCounts, err := getDirectInviteeCounts([]int{userId})
	if err != nil {
		return nil, err
	}
	restorable := 0
	if activeRisk != nil {
		counts, err := getRestorableDetachedInviteeCounts([]int{activeRisk.Id})
		if err != nil {
			return nil, err
		}
		restorable = counts[activeRisk.Id]
	}
	return &AffiliateRiskPreview{
		User:                   user,
		Balance:                *balance,
		ActiveRisk:             activeRisk,
		DirectInviteeCount:     inviteeCounts[userId],
		RestorableInviteeCount: restorable,
		ClearableQuota:         balance.PendingQuota + balance.AvailableQuota + balance.FrozenQuota + balance.RiskFrozenQuota,
	}, nil
}

func GetActiveAffiliateRiskUser(userId int) (*AffiliateRiskUser, error) {
	var risk AffiliateRiskUser
	err := DB.Where("user_id = ? AND status = ?", userId, AffiliateRiskStatusActive).First(&risk).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &risk, nil
}

func ApplyAffiliateRiskAction(userId int, adminId int, req AffiliateRiskApplyRequest) (*AffiliateRiskApplyResult, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if err := ensureAffiliateRiskActionAllowed(req); err != nil {
		return nil, err
	}
	if !req.FreezeAssets && !req.BlockInviteCode && !req.DetachInvitees && !req.ClearAssets {
		return nil, errors.New("至少选择一个处置项")
	}
	result := &AffiliateRiskApplyResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Select("id").Where("id = ?", userId).First(&user).Error; err != nil {
			return errors.New("user not found")
		}

		risk, err := getOrCreateActiveAffiliateRiskUserTx(tx, userId)
		if err != nil {
			return err
		}
		risk.FreezeAssets = risk.FreezeAssets || req.FreezeAssets
		risk.BlockInviteCode = risk.BlockInviteCode || req.BlockInviteCode
		risk.Reason = strings.TrimSpace(req.Reason)
		risk.AdminId = adminId
		if err := tx.Save(risk).Error; err != nil {
			return err
		}

		if req.FreezeAssets {
			frozen, err := freezeAffiliateAssetsTx(tx, userId)
			if err != nil {
				return err
			}
			result.FrozenQuota = frozen
		}
		if req.DetachInvitees {
			detached, err := detachAffiliateInviteesTx(tx, risk.Id, userId)
			if err != nil {
				return err
			}
			if detached > 0 {
				risk.DetachedInvitees = true
				if err := tx.Model(risk).Update("detached_invitees", true).Error; err != nil {
					return err
				}
			}
			result.DetachedCount = detached
		}
		if req.ClearAssets {
			clearResult, err := clearAffiliateAssetsTx(tx, userId, adminId)
			if err != nil {
				return err
			}
			risk.ClearedQuota += clearResult.ClearedQuota
			if err := tx.Model(risk).Update("cleared_quota", risk.ClearedQuota).Error; err != nil {
				return err
			}
			result.ClearedQuota = clearResult.ClearedQuota
			result.RejectedWithdrawals = clearResult.RejectedWithdrawals
		}

		if err := createAffiliateRiskEventTx(tx, risk.Id, userId, adminId, AffiliateRiskEventApply, map[string]interface{}{
			"freeze_assets":        req.FreezeAssets,
			"block_invite_code":    req.BlockInviteCode,
			"detach_invitees":      req.DetachInvitees,
			"clear_assets":         req.ClearAssets,
			"reason":               risk.Reason,
			"frozen_quota":         result.FrozenQuota,
			"detached_count":       result.DetachedCount,
			"cleared_quota":        result.ClearedQuota,
			"rejected_withdrawals": result.RejectedWithdrawals,
		}); err != nil {
			return err
		}
		result.RiskUser = *risk
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = invalidateUserCache(userId)
	return result, nil
}

func RemoveAffiliateRiskAction(userId int, adminId int, req AffiliateRiskRemoveRequest) (*AffiliateRiskRemoveResult, error) {
	result := &AffiliateRiskRemoveResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var risk AffiliateRiskUser
		if err := lockForUpdate(tx).
			Where("user_id = ? AND status = ?", userId, AffiliateRiskStatusActive).
			First(&risk).Error; err != nil {
			return errors.New("active risk user not found")
		}

		if risk.FreezeAssets {
			unfrozen, err := unfreezeAffiliateAssetsTx(tx, userId)
			if err != nil {
				return err
			}
			result.UnfrozenQuota = unfrozen
		}
		if req.RestoreDetachedInvitees {
			restored, err := restoreAffiliateDetachedInviteesTx(tx, risk.Id, userId)
			if err != nil {
				return err
			}
			result.RestoredInvitees = restored
		}

		now := common.GetTimestamp()
		if err := tx.Model(&risk).Updates(map[string]interface{}{
			"status":            AffiliateRiskStatusRemoved,
			"removed_by":        adminId,
			"remove_remark":     strings.TrimSpace(req.Remark),
			"removed_at":        now,
			"freeze_assets":     false,
			"block_invite_code": false,
		}).Error; err != nil {
			return err
		}
		if err := createAffiliateRiskEventTx(tx, risk.Id, userId, adminId, AffiliateRiskEventRemove, map[string]interface{}{
			"restore_detached_invitees": req.RestoreDetachedInvitees,
			"restored_invitees":         result.RestoredInvitees,
			"unfrozen_quota":            result.UnfrozenQuota,
			"remark":                    strings.TrimSpace(req.Remark),
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = invalidateUserCache(userId)
	return result, nil
}

func getOrCreateActiveAffiliateRiskUserTx(tx *gorm.DB, userId int) (*AffiliateRiskUser, error) {
	var risk AffiliateRiskUser
	err := lockForUpdate(tx).
		Where("user_id = ? AND status = ?", userId, AffiliateRiskStatusActive).
		First(&risk).Error
	if err == nil {
		return &risk, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	err = lockForUpdate(tx).
		Where("user_id = ?", userId).
		First(&risk).Error
	if err == nil {
		now := common.GetTimestamp()
		if err := tx.Model(&risk).Updates(map[string]interface{}{
			"status":            AffiliateRiskStatusActive,
			"freeze_assets":     false,
			"block_invite_code": false,
			"removed_by":        0,
			"remove_remark":     "",
			"removed_at":        0,
			"updated_at":        now,
		}).Error; err != nil {
			return nil, err
		}
		risk.Status = AffiliateRiskStatusActive
		risk.FreezeAssets = false
		risk.BlockInviteCode = false
		risk.RemovedBy = 0
		risk.RemoveRemark = ""
		risk.RemovedAt = 0
		return &risk, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	risk = AffiliateRiskUser{
		UserId: userId,
		Status: AffiliateRiskStatusActive,
	}
	if err := tx.Create(&risk).Error; err != nil {
		return nil, err
	}
	return &risk, nil
}

func freezeAffiliateAssetsTx(tx *gorm.DB, userId int) (int, error) {
	balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
	if err != nil {
		return 0, err
	}
	if balance.AvailableQuota <= 0 {
		return 0, tx.Save(balance).Error
	}
	frozen := balance.AvailableQuota
	balance.AvailableQuota = 0
	balance.RiskFrozenQuota += frozen
	return frozen, tx.Save(balance).Error
}

func unfreezeAffiliateAssetsTx(tx *gorm.DB, userId int) (int, error) {
	balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
	if err != nil {
		return 0, err
	}
	unfrozen := balance.RiskFrozenQuota
	if unfrozen <= 0 {
		return 0, nil
	}
	balance.RiskFrozenQuota = 0
	balance.AvailableQuota += unfrozen
	return unfrozen, tx.Save(balance).Error
}

func detachAffiliateInviteesTx(tx *gorm.DB, riskUserId int, userId int) (int, error) {
	var invitees []User
	if err := lockForUpdate(tx).
		Select("id").
		Where("inviter_id = ?", userId).
		Find(&invitees).Error; err != nil {
		return 0, err
	}
	if len(invitees) == 0 {
		return 0, nil
	}
	inviteeIds := make([]int, 0, len(invitees))
	for _, invitee := range invitees {
		inviteeIds = append(inviteeIds, invitee.Id)
		snapshot := AffiliateRiskDetachedInvitee{
			RiskUserId: riskUserId,
			UserId:     userId,
			InviteeId:  invitee.Id,
		}
		if err := tx.Where("risk_user_id = ? AND invitee_id = ?", riskUserId, invitee.Id).FirstOrCreate(&snapshot).Error; err != nil {
			return 0, err
		}
		if snapshot.Restored {
			if err := tx.Model(&snapshot).Updates(map[string]interface{}{
				"restored":    false,
				"restored_at": 0,
			}).Error; err != nil {
				return 0, err
			}
		}
	}
	if err := tx.Model(&User{}).Where("id IN ?", inviteeIds).Update("inviter_id", 0).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&User{}).Where("id = ?", userId).Update("aff_count", gorm.Expr("CASE WHEN aff_count >= ? THEN aff_count - ? ELSE 0 END", len(inviteeIds), len(inviteeIds))).Error; err != nil {
		return 0, err
	}
	if err := createAffiliateRiskEventTx(tx, riskUserId, userId, 0, AffiliateRiskEventDetachInvitees, map[string]interface{}{
		"detached_count": len(inviteeIds),
	}); err != nil {
		return 0, err
	}
	return len(inviteeIds), nil
}

func restoreAffiliateDetachedInviteesTx(tx *gorm.DB, riskUserId int, userId int) (int, error) {
	var rows []AffiliateRiskDetachedInvitee
	if err := lockForUpdate(tx).
		Where("risk_user_id = ? AND user_id = ? AND restored = ?", riskUserId, userId, false).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	restored := 0
	now := common.GetTimestamp()
	for _, row := range rows {
		result := tx.Model(&User{}).
			Where("id = ? AND inviter_id = 0", row.InviteeId).
			Update("inviter_id", userId)
		if result.Error != nil {
			return restored, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		restored++
		if err := tx.Model(&row).Updates(map[string]interface{}{
			"restored":    true,
			"restored_at": now,
		}).Error; err != nil {
			return restored, err
		}
	}
	if restored > 0 {
		if err := tx.Model(&User{}).Where("id = ?", userId).Update("aff_count", gorm.Expr("aff_count + ?", restored)).Error; err != nil {
			return restored, err
		}
	}
	if err := createAffiliateRiskEventTx(tx, riskUserId, userId, 0, AffiliateRiskEventRestoreInvitees, map[string]interface{}{
		"restored_count": restored,
	}); err != nil {
		return restored, err
	}
	return restored, nil
}

type affiliateRiskClearResult struct {
	ClearedQuota        int
	RejectedWithdrawals int
}

func clearAffiliateAssetsTx(tx *gorm.DB, userId int, adminId int) (*affiliateRiskClearResult, error) {
	balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
	if err != nil {
		return nil, err
	}
	cleared := balance.PendingQuota + balance.AvailableQuota + balance.RiskFrozenQuota + balance.FrozenQuota
	result := &affiliateRiskClearResult{ClearedQuota: cleared}
	if cleared <= 0 {
		return result, nil
	}

	now := common.GetTimestamp()
	var withdrawals []AffiliateWithdrawal
	if err := lockForUpdate(tx).
		Where("user_id = ? AND status IN ?", userId, []string{AffiliateWithdrawalStatusPending, AffiliateWithdrawalStatusApproved}).
		Find(&withdrawals).Error; err != nil {
		return nil, err
	}
	for _, withdrawal := range withdrawals {
		withdrawal.Status = AffiliateWithdrawalStatusRejected
		withdrawal.AdminId = adminId
		withdrawal.AdminRemark = "返佣风控清空资产"
		withdrawal.RejectedTime = now
		if err := tx.Save(&withdrawal).Error; err != nil {
			return nil, err
		}
		result.RejectedWithdrawals++
	}

	if err := tx.Model(&AffiliateRecord{}).
		Where("user_id = ? AND status = ?", userId, AffiliateRecordStatusPending).
		Updates(map[string]interface{}{
			"status":       AffiliateRecordStatusConfiscated,
			"settled_time": now,
		}).Error; err != nil {
		return nil, err
	}

	balance.PendingQuota = 0
	balance.AvailableQuota = 0
	balance.RiskFrozenQuota = 0
	balance.FrozenQuota = 0
	balance.ConfiscatedQuota += cleared
	balance.TotalQuota -= cleared
	if balance.TotalQuota < 0 {
		balance.TotalQuota = 0
	}
	if err := tx.Save(balance).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func createAffiliateRiskEventTx(tx *gorm.DB, riskUserId int, userId int, adminId int, action string, detail map[string]interface{}) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	var detailText string
	if detail != nil {
		bytes, err := common.Marshal(detail)
		if err != nil {
			return err
		}
		detailText = string(bytes)
	}
	return tx.Create(&AffiliateRiskEvent{
		RiskUserId: riskUserId,
		UserId:     userId,
		Action:     action,
		AdminId:    adminId,
		Detail:     detailText,
	}).Error
}

func getAffiliateBalancesByUserIds(userIds []int) (map[int]AffiliateBalance, error) {
	result := make(map[int]AffiliateBalance)
	userIds = uniqueInts(userIds)
	if len(userIds) == 0 {
		return result, nil
	}
	var balances []AffiliateBalance
	if err := DB.Where("user_id IN ?", userIds).Find(&balances).Error; err != nil {
		return nil, err
	}
	for _, balance := range balances {
		result[balance.UserId] = balance
	}
	for _, userId := range userIds {
		if _, ok := result[userId]; !ok {
			result[userId] = AffiliateBalance{UserId: userId}
		}
	}
	return result, nil
}

func getDirectInviteeCounts(userIds []int) (map[int]int, error) {
	result := make(map[int]int)
	userIds = uniqueInts(userIds)
	if len(userIds) == 0 {
		return result, nil
	}
	type row struct {
		InviterId int
		Count     int
	}
	var rows []row
	if err := DB.Model(&User{}).
		Select("inviter_id, COUNT(*) AS count").
		Where("inviter_id IN ?", userIds).
		Group("inviter_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, item := range rows {
		result[item.InviterId] = item.Count
	}
	return result, nil
}

func getRestorableDetachedInviteeCounts(riskIds []int) (map[int]int, error) {
	result := make(map[int]int)
	riskIds = uniqueInts(riskIds)
	if len(riskIds) == 0 {
		return result, nil
	}
	type row struct {
		RiskUserId int
		Count      int
	}
	var rows []row
	if err := DB.Model(&AffiliateRiskDetachedInvitee{}).
		Select("risk_user_id, COUNT(*) AS count").
		Where("risk_user_id IN ? AND restored = ?", riskIds, false).
		Group("risk_user_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, item := range rows {
		result[item.RiskUserId] = item.Count
	}
	return result, nil
}

func ensureAffiliateRiskActionAllowed(req AffiliateRiskApplyRequest) error {
	if req.ClearAssets && strings.TrimSpace(req.Reason) == "" {
		return fmt.Errorf("清空资产必须填写原因")
	}
	return nil
}
