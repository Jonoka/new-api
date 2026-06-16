package model

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
)

const (
	AffiliateAppStatusPending  = "pending"
	AffiliateAppStatusApproved = "approved"
	AffiliateAppStatusRejected = "rejected"
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

func HashAgreementText(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}

func CreateAffiliateApplication(userId int, agreementText string) error {
	affiliateSetting := setting.GetAffiliateSetting()

	if err := checkInviterEligibility(userId, affiliateSetting); err != nil {
		return err
	}

	var existing AffiliateApplication
	err := DB.Where("user_id = ?", userId).First(&existing).Error
	if err == nil {
		if existing.Status == AffiliateAppStatusPending {
			return errors.New("application already pending")
		}
		if existing.Status == AffiliateAppStatusApproved {
			return errors.New("already approved")
		}
		DB.Delete(&existing)
	}

	app := &AffiliateApplication{
		UserId:        userId,
		Status:        AffiliateAppStatusPending,
		AgreedAt:      common.GetTimestamp(),
		AgreementHash: HashAgreementText(agreementText),
	}
	return DB.Create(app).Error
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
		var totalRecharge int64
		DB.Model(&TopUp{}).
			Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).
			Select("COALESCE(SUM(quota), 0)").
			Scan(&totalRecharge)
		if totalRecharge < int64(s.InviterMinRechargeAmount) {
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