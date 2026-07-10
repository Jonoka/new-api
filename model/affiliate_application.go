package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
)

const (
	AffiliateAppStatusPending  = "pending"
	AffiliateAppStatusApproved = "approved"
	AffiliateAppStatusRejected = "rejected"
)

const (
	AffiliateGateStatusNotRequired = "not_required"
	AffiliateGateStatusNone        = "none"
)

type AffiliateApplication struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	UserId         int    `json:"user_id" gorm:"uniqueIndex"`
	Status         string `json:"status" gorm:"type:varchar(32);index;default:pending"`
	AgreedAt       int64  `json:"agreed_at"`
	AgreementHash  string `json:"agreement_hash" gorm:"type:varchar(64)"`
	AdminId        int    `json:"admin_id"`
	AdminRemark    string `json:"admin_remark" gorm:"type:varchar(500)"`
	RejectedReason string `json:"rejected_reason" gorm:"type:varchar(500)"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AffiliateApplication) TableName() string {
	return "affiliate_applications"
}

type AffiliateApplicationWithUser struct {
	AffiliateApplication
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type AffiliateGrantResult struct {
	UserId      int    `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Status      string `json:"status"`
	Updated     bool   `json:"updated"`
}

func HashAgreementText(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}

func affiliateAgreementHashForSetting(s *setting.AffiliateSetting) string {
	if s == nil || !s.AgreementEnabled {
		return ""
	}
	return HashAgreementText(s.AgreementText)
}

func affiliateApplicationSatisfiesAgreement(app *AffiliateApplication, s *setting.AffiliateSetting) bool {
	if s == nil || !s.AgreementEnabled {
		return true
	}
	if app == nil || app.AgreementHash == "" {
		return false
	}
	return app.AgreementHash == affiliateAgreementHashForSetting(s)
}

func AffiliateApplicationSatisfiesAgreement(app *AffiliateApplication, s *setting.AffiliateSetting) bool {
	return affiliateApplicationSatisfiesAgreement(app, s)
}

func AffiliateAccessRequired(s *setting.AffiliateSetting) bool {
	return s != nil && (s.ReviewEnabled || s.AgreementEnabled)
}

func AffiliateUserCanInvite(userId int, s *setting.AffiliateSetting) bool {
	return affiliateUserCanInviteWithDB(DB, userId, s)
}

func affiliateUserCanInviteWithDB(db *gorm.DB, userId int, s *setting.AffiliateSetting) bool {
	if !AffiliateAccessRequired(s) {
		return true
	}
	if db == nil {
		return false
	}
	var app AffiliateApplication
	if err := db.Where("user_id = ?", userId).First(&app).Error; err != nil {
		return false
	}
	if !affiliateApplicationSatisfiesAgreement(&app, s) {
		return false
	}
	if s.ReviewEnabled {
		return app.Status == AffiliateAppStatusApproved
	}
	return app.Status == AffiliateAppStatusApproved || app.Status == AffiliateAppStatusPending
}

func CreateAffiliateApplication(userId int, agreementText string) error {
	affiliateSetting := setting.GetAffiliateSetting()
	status := AffiliateAppStatusPending
	if !affiliateSetting.ReviewEnabled {
		status = AffiliateAppStatusApproved
	}

	if err := checkInviterEligibility(userId, affiliateSetting); err != nil {
		return err
	}

	var existing AffiliateApplication
	err := DB.Where("user_id = ?", userId).First(&existing).Error
	if err == nil {
		currentHash := affiliateAgreementHashForSetting(affiliateSetting)
		if existing.Status == AffiliateAppStatusPending && existing.AgreementHash == currentHash {
			return errors.New("application already pending")
		}
		if existing.Status == AffiliateAppStatusApproved && existing.AgreementHash == currentHash {
			return errors.New("already approved")
		}
		DB.Delete(&existing)
	}

	app := &AffiliateApplication{
		UserId:        userId,
		Status:        status,
		AgreedAt:      common.GetTimestamp(),
		AgreementHash: HashAgreementText(agreementText),
	}
	return DB.Create(app).Error
}

func GrantAffiliateAccessByUser(userId int, userIdentifier string, adminId int, remark string) (*AffiliateGrantResult, error) {
	remark = strings.TrimSpace(remark)
	if remark == "" {
		remark = "管理员手动赋予返佣权限"
	}

	affiliateSetting := setting.GetAffiliateSetting()
	var result *AffiliateGrantResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		user, err := findAffiliateBindUserTx(tx, userId, userIdentifier)
		if err != nil {
			return err
		}

		now := common.GetTimestamp()
		app := &AffiliateApplication{}
		err = tx.Set("gorm:query_option", "FOR UPDATE").Where("user_id = ?", user.Id).First(app).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			app = &AffiliateApplication{
				UserId:        user.Id,
				Status:        AffiliateAppStatusApproved,
				AgreedAt:      now,
				AgreementHash: affiliateAgreementHashForSetting(affiliateSetting),
				AdminId:       adminId,
				AdminRemark:   remark,
			}
			if err := tx.Create(app).Error; err != nil {
				return err
			}
			result = &AffiliateGrantResult{
				UserId:      user.Id,
				Username:    user.Username,
				DisplayName: user.DisplayName,
				Email:       user.Email,
				Status:      app.Status,
				Updated:     true,
			}
			return nil
		}

		changed := app.Status != AffiliateAppStatusApproved ||
			app.AgreementHash != affiliateAgreementHashForSetting(affiliateSetting) ||
			app.AdminId != adminId ||
			app.AdminRemark != remark ||
			app.RejectedReason != ""
		updates := map[string]interface{}{
			"status":          AffiliateAppStatusApproved,
			"agreed_at":       now,
			"agreement_hash":  affiliateAgreementHashForSetting(affiliateSetting),
			"admin_id":        adminId,
			"admin_remark":    remark,
			"rejected_reason": "",
		}
		if err := tx.Model(app).Updates(updates).Error; err != nil {
			return err
		}

		result = &AffiliateGrantResult{
			UserId:      user.Id,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Email:       user.Email,
			Status:      AffiliateAppStatusApproved,
			Updated:     changed,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func checkInviterEligibility(userId int, s *setting.AffiliateSetting) error {
	if s.InviterMinAccountAgeDays <= 0 && s.InviterMinRechargeAmount <= 0 {
		return nil
	}

	var user User
	if err := DB.Select("id", "created_at").Where("id = ?", userId).First(&user).Error; err != nil {
		return errors.New("user not found")
	}

	if s.InviterMinAccountAgeDays > 0 {
		requiredAge := int64(s.InviterMinAccountAgeDays) * 86400
		if common.GetTimestamp()-user.CreatedAt < requiredAge {
			return fmt.Errorf("account must be at least %d days old", s.InviterMinAccountAgeDays)
		}
	}

	if s.InviterMinRechargeAmount > 0 {
		totalRecharge, err := GetUserTotalRechargeAmount(userId)
		if err != nil {
			return fmt.Errorf("failed to load recharge history: %w", err)
		}
		if totalRecharge < float64(s.InviterMinRechargeAmount) {
			return fmt.Errorf("total recharge must be at least %d", s.InviterMinRechargeAmount)
		}
	}

	return nil
}

func ApproveAffiliateApplication(appId, adminId int, remark string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var app AffiliateApplication
		if err := tx.Where("id = ? AND status = ?", appId, AffiliateAppStatusPending).First(&app).Error; err != nil {
			return errors.New("application not found or not pending")
		}

		if err := tx.Model(&app).Updates(map[string]interface{}{
			"status":       AffiliateAppStatusApproved,
			"admin_id":     adminId,
			"admin_remark": remark,
		}).Error; err != nil {
			return err
		}

		return nil
	})
}

func RejectAffiliateApplication(appId, adminId int, reason string) error {
	var app AffiliateApplication
	if err := DB.Where("id = ? AND status = ?", appId, AffiliateAppStatusPending).First(&app).Error; err != nil {
		return errors.New("application not found or not pending")
	}

	return DB.Model(&app).Updates(map[string]interface{}{
		"status":          AffiliateAppStatusRejected,
		"admin_id":        adminId,
		"rejected_reason": reason,
	}).Error
}

func RevokeAffiliateApplication(appId int) error {
	var app AffiliateApplication
	if err := DB.Where("id = ?", appId).First(&app).Error; err != nil {
		return errors.New("application not found")
	}
	return DB.Delete(&app).Error
}

func GetAffiliateApplicationByUserId(userId int) (*AffiliateApplication, error) {
	var app AffiliateApplication
	err := DB.Where("user_id = ?", userId).First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &app, err
}

func IsInviterApproved(userId int) bool {
	var count int64
	DB.Model(&AffiliateApplication{}).
		Where("user_id = ? AND status = ?", userId, AffiliateAppStatusApproved).
		Count(&count)
	return count > 0
}

func GetPendingApplications(page, pageSize int, status string) ([]AffiliateApplicationWithUser, int64, error) {
	var total int64
	var apps []AffiliateApplication

	query := DB.Model(&AffiliateApplication{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&apps).Error; err != nil {
		return nil, 0, err
	}

	result := make([]AffiliateApplicationWithUser, 0, len(apps))
	for _, app := range apps {
		item := AffiliateApplicationWithUser{AffiliateApplication: app}
		var user User
		if err := DB.Select("username", "display_name", "email").Where("id = ?", app.UserId).First(&user).Error; err == nil {
			item.Username = user.Username
			item.DisplayName = user.DisplayName
			item.Email = user.Email
		}
		result = append(result, item)
	}

	return result, total, nil
}

func AutoApproveMatureApplications() (int, error) {
	s := setting.GetAffiliateSetting()
	if !s.ReviewEnabled || s.AutoApproveAfterDays <= 0 {
		return 0, nil
	}

	cutoff := common.GetTimestamp() - int64(s.AutoApproveAfterDays)*86400
	result := DB.Model(&AffiliateApplication{}).
		Where("status = ? AND created_at <= ?", AffiliateAppStatusPending, cutoff).
		Updates(map[string]interface{}{
			"status":       AffiliateAppStatusApproved,
			"admin_remark": "auto-approved",
		})

	return int(result.RowsAffected), result.Error
}
