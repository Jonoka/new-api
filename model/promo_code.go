package model

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	PromoCodeDiscountTypePercent = "percent"
	PromoCodeDiscountTypeFixed   = "fixed"
)

const (
	PromoCodeTargetTopUp        = "topup"
	PromoCodeTargetSubscription = "subscription"
)

const (
	promoCodeLegacyCodeIndex = "idx_promo_codes_code"
	promoCodeUniqueIndex     = "idx_promo_codes_code_deleted_id"
)

type PromoCode struct {
	Id                       int            `json:"id"`
	UserId                   int            `json:"user_id"`
	Name                     string         `json:"name" gorm:"index"`
	Code                     string         `json:"code" gorm:"type:varchar(64);uniqueIndex:idx_promo_codes_code_deleted_id,priority:1"`
	DeletedId                int            `json:"-" gorm:"not null;default:0;uniqueIndex:idx_promo_codes_code_deleted_id,priority:2"`
	Status                   int            `json:"status" gorm:"default:1"`
	DiscountType             string         `json:"discount_type" gorm:"type:varchar(16)"`
	DiscountValue            int64          `json:"discount_value" gorm:"type:bigint;not null;default:0"`
	AppliesToTopup           bool           `json:"applies_to_topup" gorm:"default:false"`
	AppliesToAllSubscription bool           `json:"applies_to_all_subscription" gorm:"default:false"`
	SubscriptionPlanIds      string         `json:"subscription_plan_ids" gorm:"type:text"`
	MaxRedeemCount           int            `json:"max_redeem_count" gorm:"default:0"`
	RedeemedCount            int            `json:"redeemed_count" gorm:"default:0"`
	CreatedTime              int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime              int64          `json:"updated_time" gorm:"bigint"`
	ExpiredTime              int64          `json:"expired_time" gorm:"bigint"`
	DeletedAt                gorm.DeletedAt `gorm:"index"`
}

type PromoCodeUsage struct {
	Id             int     `json:"id"`
	PromoCodeId    int     `json:"promo_code_id" gorm:"index;uniqueIndex:idx_promo_usage_order,priority:1"`
	UserId         int     `json:"user_id" gorm:"index"`
	OrderType      string  `json:"order_type" gorm:"type:varchar(32);index"`
	OrderNo        string  `json:"order_no" gorm:"type:varchar(255);index;uniqueIndex:idx_promo_usage_order,priority:2"`
	OriginalAmount float64 `json:"original_amount"`
	DiscountAmount float64 `json:"discount_amount"`
	PaidAmount     float64 `json:"paid_amount"`
	CreatedTime    int64   `json:"created_time" gorm:"bigint"`
}

type PromoCodeDiscountResult struct {
	PromoCodeId     int     `json:"promo_code_id"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	DiscountType    string  `json:"discount_type"`
	DiscountValue   int64   `json:"discount_value"`
	OriginalAmount  float64 `json:"original_amount"`
	DiscountAmount  float64 `json:"discount_amount"`
	PaidAmount      float64 `json:"paid_amount"`
	ActualPaidQuota int     `json:"actual_paid_quota"`
}

func normalizePromoCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func (promo *PromoCode) normalize() {
	promo.Code = normalizePromoCode(promo.Code)
	promo.Name = strings.TrimSpace(promo.Name)
	promo.DiscountType = strings.ToLower(strings.TrimSpace(promo.DiscountType))
	if promo.DiscountType == "" {
		promo.DiscountType = PromoCodeDiscountTypePercent
	}
	promo.SubscriptionPlanIds = normalizePromoSubscriptionPlanIds(promo.SubscriptionPlanIds)
	if promo.Status == 0 {
		promo.Status = common.RedemptionCodeStatusEnabled
	}
	now := common.GetTimestamp()
	if promo.CreatedTime == 0 {
		promo.CreatedTime = now
	}
	promo.UpdatedTime = now
}

func normalizePromoSubscriptionPlanIds(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	seen := map[int]struct{}{}
	ids := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, strconv.Itoa(id))
	}
	return strings.Join(ids, ",")
}

func promoPlanIdIncluded(raw string, planId int) bool {
	if planId <= 0 {
		return false
	}
	for _, item := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(item))
		if err == nil && id == planId {
			return true
		}
	}
	return false
}

func validatePromoCode(promo *PromoCode) error {
	if promo == nil {
		return errors.New("优惠码为空")
	}
	promo.normalize()
	if promo.Code == "" {
		return errors.New("优惠码不能为空")
	}
	if promo.Name == "" {
		return errors.New("优惠码名称不能为空")
	}
	if promo.DiscountType != PromoCodeDiscountTypePercent && promo.DiscountType != PromoCodeDiscountTypeFixed {
		return errors.New("无效的优惠类型")
	}
	if promo.DiscountValue <= 0 {
		return errors.New("优惠值必须大于 0")
	}
	if promo.DiscountType == PromoCodeDiscountTypePercent && promo.DiscountValue > 100 {
		return errors.New("优惠百分比不能超过 100")
	}
	if !promo.AppliesToTopup && !promo.AppliesToAllSubscription && promo.SubscriptionPlanIds == "" {
		return errors.New("优惠码必须至少指定一个适用范围")
	}
	if promo.MaxRedeemCount < 0 {
		return errors.New("最大使用次数不能为负数")
	}
	if promo.ExpiredTime != 0 && promo.ExpiredTime < common.GetTimestamp() {
		return errors.New("过期时间不能早于当前时间")
	}
	return nil
}

func migratePromoCodeDeletionKey(db *gorm.DB) error {
	if db == nil {
		return errors.New("数据库连接为空")
	}
	if !db.Migrator().HasColumn(&PromoCode{}, "DeletedId") {
		if err := db.Migrator().AddColumn(&PromoCode{}, "DeletedId"); err != nil {
			return fmt.Errorf("添加优惠码删除标识失败: %w", err)
		}
	}
	if err := db.Unscoped().Model(&PromoCode{}).
		Where("deleted_at IS NOT NULL AND deleted_id = ?", 0).
		UpdateColumn("deleted_id", gorm.Expr("id")).Error; err != nil {
		return fmt.Errorf("回填优惠码删除标识失败: %w", err)
	}
	if !db.Migrator().HasIndex(&PromoCode{}, promoCodeUniqueIndex) {
		if err := db.Migrator().CreateIndex(&PromoCode{}, promoCodeUniqueIndex); err != nil {
			return fmt.Errorf("创建优惠码组合唯一索引失败: %w", err)
		}
	}
	if db.Migrator().HasIndex(&PromoCode{}, promoCodeLegacyCodeIndex) {
		if err := db.Migrator().DropIndex(&PromoCode{}, promoCodeLegacyCodeIndex); err != nil {
			return fmt.Errorf("删除优惠码旧唯一索引失败: %w", err)
		}
	}
	return nil
}

// preparePromoCodeForWriteTx 兼容旧版本只写 deleted_at、deleted_id 仍为 0 的记录。
// 活动冲突仍被拒绝；历史软删除冲突则补齐 deleted_id，释放活动唯一键。
func preparePromoCodeForWriteTx(tx *gorm.DB, code string, currentId int) error {
	var existing PromoCode
	query := lockForUpdate(tx).Unscoped().Where("code = ? AND deleted_id = ?", code, 0)
	if currentId > 0 {
		query = query.Where("id <> ?", currentId)
	}
	err := query.First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !existing.DeletedAt.Valid {
		return errors.New("优惠码已存在")
	}
	return tx.Unscoped().Model(&PromoCode{}).
		Where("id = ? AND deleted_id = ?", existing.Id, 0).
		UpdateColumn("deleted_id", existing.Id).Error
}

func (promo *PromoCode) Insert() error {
	if err := validatePromoCode(promo); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := preparePromoCodeForWriteTx(tx, promo.Code, 0); err != nil {
			return err
		}
		return tx.Create(promo).Error
	})
}

func (promo *PromoCode) Update() error {
	if err := validatePromoCode(promo); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := preparePromoCodeForWriteTx(tx, promo.Code, promo.Id); err != nil {
			return err
		}
		return tx.Model(promo).Select(
			"name",
			"code",
			"status",
			"discount_type",
			"discount_value",
			"applies_to_topup",
			"applies_to_all_subscription",
			"subscription_plan_ids",
			"max_redeem_count",
			"redeemed_count",
			"updated_time",
			"expired_time",
		).Updates(promo).Error
	})
}

func GetPromoCodeById(id int) (*PromoCode, error) {
	if id <= 0 {
		return nil, errors.New("id 为空")
	}
	promo := &PromoCode{}
	return promo, DB.Where("id = ?", id).First(promo).Error
}

func GetAllPromoCodes(startIdx int, num int) (promoCodes []*PromoCode, total int64, err error) {
	tx := DB.Model(&PromoCode{})
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&promoCodes).Error
	return promoCodes, total, err
}

func SearchPromoCodes(keyword string, startIdx int, num int) (promoCodes []*PromoCode, total int64, err error) {
	keyword = strings.TrimSpace(keyword)
	query := DB.Model(&PromoCode{})
	if keyword != "" {
		normalized := normalizePromoCode(keyword)
		if id, convErr := strconv.Atoi(keyword); convErr == nil {
			query = query.Where("id = ? OR code LIKE ? OR name LIKE ?", id, normalized+"%", keyword+"%")
		} else {
			query = query.Where("code LIKE ? OR name LIKE ?", normalized+"%", keyword+"%")
		}
	}
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&promoCodes).Error
	return promoCodes, total, err
}

func DeletePromoCodeById(id int) error {
	if id <= 0 {
		return errors.New("id 为空")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var promo PromoCode
		err := lockForUpdate(tx).Where("id = ?", id).First(&promo).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&promo).UpdateColumn("deleted_id", promo.Id).Error; err != nil {
			return err
		}
		return tx.Delete(&promo).Error
	})
}

func promoAppliesToTarget(promo *PromoCode, target string, planId int) bool {
	switch target {
	case PromoCodeTargetTopUp:
		return promo.AppliesToTopup
	case PromoCodeTargetSubscription:
		return promo.AppliesToAllSubscription || promoPlanIdIncluded(promo.SubscriptionPlanIds, planId)
	default:
		return false
	}
}

func CalculatePromoCodeDiscount(code string, target string, planId int, originalAmount float64) (*PromoCodeDiscountResult, error) {
	return calculatePromoCodeDiscountTx(DB, code, target, planId, originalAmount)
}

func calculatePromoCodeDiscountTx(tx *gorm.DB, code string, target string, planId int, originalAmount float64) (*PromoCodeDiscountResult, error) {
	code = normalizePromoCode(code)
	if code == "" {
		return nil, nil
	}
	if tx == nil {
		tx = DB
	}
	if originalAmount < 0 {
		return nil, errors.New("原始金额不能为负数")
	}
	var promo PromoCode
	if err := tx.Where("code = ?", code).First(&promo).Error; err != nil {
		return nil, errors.New("无效的优惠码")
	}
	if promo.Status != common.RedemptionCodeStatusEnabled {
		return nil, errors.New("优惠码不可用")
	}
	if promo.ExpiredTime != 0 && promo.ExpiredTime < common.GetTimestamp() {
		return nil, errors.New("优惠码已过期")
	}
	if promo.MaxRedeemCount > 0 && promo.RedeemedCount >= promo.MaxRedeemCount {
		return nil, errors.New("优惠码已达使用次数上限")
	}
	if !promoAppliesToTarget(&promo, target, planId) {
		return nil, errors.New("优惠码不适用于当前订单")
	}
	discount := decimal.Zero
	original := decimal.NewFromFloat(originalAmount)
	switch promo.DiscountType {
	case PromoCodeDiscountTypePercent:
		discount = original.Mul(decimal.NewFromInt(promo.DiscountValue)).Div(decimal.NewFromInt(100))
	case PromoCodeDiscountTypeFixed:
		discount = decimal.NewFromInt(promo.DiscountValue).Div(decimal.NewFromFloat(common.QuotaPerUnit))
	default:
		return nil, errors.New("无效的优惠类型")
	}
	if discount.LessThan(decimal.Zero) {
		discount = decimal.Zero
	}
	if discount.GreaterThan(original) {
		discount = original
	}
	discount = discount.Round(2)
	paid := original.Sub(discount).Round(2)
	if paid.LessThan(decimal.Zero) {
		paid = decimal.Zero
	}
	paidFloat := paid.InexactFloat64()
	return &PromoCodeDiscountResult{
		PromoCodeId:     promo.Id,
		Code:            promo.Code,
		Name:            promo.Name,
		DiscountType:    promo.DiscountType,
		DiscountValue:   promo.DiscountValue,
		OriginalAmount:  original.Round(2).InexactFloat64(),
		DiscountAmount:  discount.InexactFloat64(),
		PaidAmount:      paidFloat,
		ActualPaidQuota: int(math.Round(paidFloat * common.QuotaPerUnit)),
	}, nil
}

func ApplyPromoCodeResultToTopUp(topUp *TopUp, discount *PromoCodeDiscountResult) {
	if topUp == nil || discount == nil {
		return
	}
	topUp.OriginalMoney = discount.OriginalAmount
	topUp.DiscountMoney = discount.DiscountAmount
	topUp.ActualMoney = discount.PaidAmount
	topUp.PromoCodeId = discount.PromoCodeId
	topUp.PromoCode = discount.Code
	topUp.Money = discount.PaidAmount
	topUp.AffiliateSourceQuota = discount.ActualPaidQuota
}

func ApplyPromoCodeResultToStripeTopUp(topUp *TopUp, discount *PromoCodeDiscountResult, rechargeMoney float64) {
	if topUp == nil || discount == nil {
		return
	}
	ApplyPromoCodeResultToTopUp(topUp, discount)
	topUp.Money = rechargeMoney
}

func ApplyPromoCodeResultToSubscriptionOrder(order *SubscriptionOrder, discount *PromoCodeDiscountResult) {
	if order == nil || discount == nil {
		return
	}
	order.OriginalMoney = discount.OriginalAmount
	order.DiscountMoney = discount.DiscountAmount
	order.ActualMoney = discount.PaidAmount
	order.PromoCodeId = discount.PromoCodeId
	order.PromoCode = discount.Code
	order.Money = discount.PaidAmount
	order.AffiliateSourceQuota = discount.ActualPaidQuota
}

func topUpAffiliateSourceQuota(topUp *TopUp, fallbackQuota int) int {
	if topUp != nil && topUp.AffiliateSourceQuota > 0 {
		return topUp.AffiliateSourceQuota
	}
	return fallbackQuota
}

func TopUpAffiliateSourceQuota(topUp *TopUp, fallbackQuota int) int {
	return topUpAffiliateSourceQuota(topUp, fallbackQuota)
}

func subscriptionOrderAffiliateSourceQuota(order *SubscriptionOrder) int {
	if order == nil {
		return 0
	}
	if order.AffiliateSourceQuota > 0 {
		return order.AffiliateSourceQuota
	}
	return int(decimal.NewFromFloat(order.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
}

func recordPromoCodeUsageTx(tx *gorm.DB, promoCodeId int, userId int, orderType string, orderNo string, originalAmount float64, discountAmount float64, paidAmount float64, enforceLimit bool) error {
	if tx == nil || promoCodeId <= 0 || userId <= 0 || strings.TrimSpace(orderNo) == "" {
		return nil
	}
	var existing PromoCodeUsage
	err := tx.Where("promo_code_id = ? AND order_no = ?", promoCodeId, orderNo).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var promo PromoCode
	query := lockForUpdate(tx)
	if !enforceLimit {
		query = query.Unscoped()
	}
	if err := query.Where("id = ?", promoCodeId).First(&promo).Error; err != nil {
		if !enforceLimit && errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if enforceLimit && promo.MaxRedeemCount > 0 && promo.RedeemedCount >= promo.MaxRedeemCount {
		return errors.New("优惠码已达使用次数上限")
	}
	usage := &PromoCodeUsage{
		PromoCodeId:    promoCodeId,
		UserId:         userId,
		OrderType:      orderType,
		OrderNo:        orderNo,
		OriginalAmount: originalAmount,
		DiscountAmount: discountAmount,
		PaidAmount:     paidAmount,
		CreatedTime:    common.GetTimestamp(),
	}
	if err := tx.Create(usage).Error; err != nil {
		return err
	}
	promo.RedeemedCount++
	if promo.MaxRedeemCount > 0 && promo.RedeemedCount >= promo.MaxRedeemCount {
		promo.Status = common.RedemptionCodeStatusUsed
	}
	promo.UpdatedTime = common.GetTimestamp()
	return tx.Save(&promo).Error
}

func recordTopUpPromoUsageTx(tx *gorm.DB, topUp *TopUp, enforceLimit bool) error {
	if topUp == nil || topUp.PromoCodeId <= 0 {
		return nil
	}
	return recordPromoCodeUsageTx(tx, topUp.PromoCodeId, topUp.UserId, PromoCodeTargetTopUp, topUp.TradeNo, topUp.OriginalMoney, topUp.DiscountMoney, topUp.ActualMoney, enforceLimit)
}

func RecordTopUpPromoUsageForPayment(topUp *TopUp) error {
	if topUp == nil || topUp.PromoCodeId <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return recordTopUpPromoUsageTx(tx, topUp, false)
	})
}

func recordSubscriptionPromoUsageTx(tx *gorm.DB, order *SubscriptionOrder, enforceLimit bool) error {
	if order == nil || order.PromoCodeId <= 0 {
		return nil
	}
	return recordPromoCodeUsageTx(tx, order.PromoCodeId, order.UserId, PromoCodeTargetSubscription, order.TradeNo, order.OriginalMoney, order.DiscountMoney, order.ActualMoney, enforceLimit)
}

func formatPromoAppliedLog(code string, discount float64) string {
	if strings.TrimSpace(code) == "" || discount <= 0 {
		return ""
	}
	return fmt.Sprintf("，优惠码: %s，优惠金额: %.2f", code, discount)
}
