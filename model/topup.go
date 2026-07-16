package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TopUp struct {
	Id                   int     `json:"id"`
	UserId               int     `json:"user_id" gorm:"index"`
	Amount               int64   `json:"amount"`
	Money                float64 `json:"money"`
	OriginalMoney        float64 `json:"original_money"`
	DiscountMoney        float64 `json:"discount_money"`
	ActualMoney          float64 `json:"actual_money"`
	PaidAmountCNY        float64 `json:"paid_amount_cny"`
	PromoCodeId          int     `json:"promo_code_id" gorm:"index"`
	PromoCode            string  `json:"promo_code" gorm:"type:varchar(64);default:''"`
	AffiliateSourceQuota int     `json:"affiliate_source_quota"`
	InvoiceRequired      bool    `json:"invoice_required"`
	InvoiceType          string  `json:"invoice_type" gorm:"type:varchar(32);default:''"`
	InvoiceKind          string  `json:"invoice_kind" gorm:"type:varchar(32);default:''"`
	InvoiceTitle         string  `json:"invoice_title" gorm:"type:varchar(255);default:''"`
	InvoiceTaxNo         string  `json:"invoice_tax_no" gorm:"type:varchar(128);default:''"`
	InvoiceEmail         string  `json:"invoice_email" gorm:"type:varchar(255);default:''"`
	InvoicePhone         string  `json:"invoice_phone" gorm:"type:varchar(64);default:''"`
	InvoiceRemark        string  `json:"invoice_remark" gorm:"type:text"`
	InvoiceBaseAmount    float64 `json:"invoice_base_amount"`
	InvoiceFeeAmount     float64 `json:"invoice_fee_amount"`
	InvoiceStatus        string  `json:"invoice_status" gorm:"type:varchar(32);default:''"`
	TradeNo              string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod        string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider      string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	RequestIP            string  `json:"request_ip" gorm:"type:varchar(64);default:''"`
	ProviderOrderId      string  `json:"provider_order_id" gorm:"type:varchar(128);default:'';index"`
	ProviderAmount       string  `json:"provider_amount" gorm:"type:varchar(64);default:''"`
	ProviderCurrency     string  `json:"provider_currency" gorm:"type:varchar(32);default:''"`
	CreateTime           int64   `json:"create_time"`
	CompleteTime         int64   `json:"complete_time"`
	Status               string  `json:"status"`
	BalanceBefore        int     `json:"-" gorm:"-"`
	BalanceAfter         int     `json:"-" gorm:"-"`
	CreditedQuota        int     `json:"-" gorm:"-"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
	PaymentMethodBepusdt      = "bepusdt"
	PaymentMethodOkpay        = "okpay"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
	PaymentProviderBepusdt      = "bepusdt"
	PaymentProviderOkpay        = "okpay"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
	increaseCryptoTopUpCache = cacheIncrUserQuota
)

func topUpTxWithRowLock(tx *gorm.DB) *gorm.DB {
	if common.UsingSQLite {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

func creditTopUpUserQuotaTx(tx *gorm.DB, userId int, quotaDelta interface{}, extraUpdates map[string]interface{}) (int, int, error) {
	var user User
	if err := topUpTxWithRowLock(tx).Select("quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return 0, 0, err
	}

	updates := make(map[string]interface{}, len(extraUpdates)+1)
	for key, value := range extraUpdates {
		updates[key] = value
	}
	updates["quota"] = gorm.Expr("quota + ?", quotaDelta)
	result := tx.Model(&User{}).Where("id = ?", userId).Updates(updates)
	if result.Error != nil {
		return 0, 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, 0, errors.New("充值用户不存在")
	}

	var balanceAfter int
	if err := tx.Model(&User{}).Select("quota").Where("id = ?", userId).Scan(&balanceAfter).Error; err != nil {
		return 0, 0, err
	}
	return user.Quota, balanceAfter, nil
}

func (topUp *TopUp) Insert() error {
	var err error
	normalizeTopUpMoneySnapshot(topUp)
	err = DB.Create(topUp).Error
	return err
}

func normalizeTopUpMoneySnapshot(topUp *TopUp) {
	if topUp == nil {
		return
	}
	if topUp.OriginalMoney == 0 {
		topUp.OriginalMoney = topUp.Money
	}
	if topUp.ActualMoney == 0 && topUp.Money > 0 && topUp.PromoCodeId == 0 {
		topUp.ActualMoney = topUp.Money
	}
	if topUp.Money == 0 && topUp.ActualMoney > 0 {
		topUp.Money = topUp.ActualMoney
	}
	if topUp.PaidAmountCNY <= 0 {
		paidAmount := invoiceOrderPaidAmount(topUp.Money, topUp.ActualMoney, topUp.PromoCodeId)
		if paidAmount > 0 {
			provider := invoiceOrderPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod)
			topUp.PaidAmountCNY = invoiceOrderAmountCNY(paidAmount, provider)
		}
	}
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByProviderOrderId(paymentProvider string, providerOrderId string) *TopUp {
	paymentProvider = strings.TrimSpace(paymentProvider)
	providerOrderId = strings.TrimSpace(providerOrderId)
	if paymentProvider == "" || providerOrderId == "" {
		return nil
	}
	var topUp TopUp
	if err := DB.Where("payment_provider = ? AND provider_order_id = ?", paymentProvider, providerOrderId).First(&topUp).Error; err != nil {
		return nil
	}
	return &topUp
}

func UpdateTopUpProviderSnapshot(tradeNo string, expectedPaymentProvider string, providerOrderId string, providerAmount string, providerCurrency string) error {
	tradeNo = strings.TrimSpace(tradeNo)
	expectedPaymentProvider = strings.TrimSpace(expectedPaymentProvider)
	providerOrderId = strings.TrimSpace(providerOrderId)
	providerAmount = strings.TrimSpace(providerAmount)
	providerCurrency = strings.ToUpper(strings.TrimSpace(providerCurrency))
	if tradeNo == "" || expectedPaymentProvider == "" || providerOrderId == "" || providerAmount == "" || providerCurrency == "" {
		return errors.New("支付网关订单快照不完整")
	}
	result := DB.Model(&TopUp{}).
		Where("trade_no = ? AND payment_provider = ? AND status = ?", tradeNo, expectedPaymentProvider, common.TopUpStatusPending).
		Updates(map[string]interface{}{
			"provider_order_id": providerOrderId,
			"provider_amount":   providerAmount,
			"provider_currency": providerCurrency,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTopUpStatusInvalid
	}
	return nil
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := topUpTxWithRowLock(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota float64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := topUpTxWithRowLock(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota = topUp.Money * common.QuotaPerUnit
		balanceBefore, balanceAfter, quotaErr := creditTopUpUserQuotaTx(tx, topUp.UserId, quota, map[string]interface{}{
			"stripe_customer": customerId,
		})
		err = quotaErr
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore

		if err := recordTopUpPromoUsageTx(tx, topUp, false); err != nil {
			return err
		}

		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}

		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, int(quota))); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupOrderLog(topUp, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(int(quota)), topUp.Amount), PaymentMethodStripe, callerIp)

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var quotaToAdd int
	var payMoney float64
	var completedTopUp TopUp
	completedNow := false

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := topUpTxWithRowLock(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		if topUp.PaymentProvider == PaymentProviderStripe {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(decimal.NewFromFloat(topUp.Money).Mul(dQuotaPerUnit).IntPart())
		} else {
			dAmount := decimal.NewFromInt(topUp.Amount)
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		}
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		balanceBefore, balanceAfter, err := creditTopUpUserQuotaTx(tx, topUp.UserId, quotaToAdd, nil)
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore

		if err := recordTopUpPromoUsageTx(tx, topUp, false); err != nil {
			return err
		}

		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}

		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, quotaToAdd)); err != nil {
			return err
		}

		payMoney = topUp.Money
		completedTopUp = *topUp
		completedNow = true
		return nil
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	if completedNow {
		RecordTopupOrderLog(&completedTopUp, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), "admin", callerIp)
	}
	return nil
}

func CompleteEpayTopUp(tradeNo string, actualPaymentMethod string) (*TopUp, int, bool, error) {
	if tradeNo == "" {
		return nil, 0, false, errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var saved TopUp
	var quotaToAdd int
	completedNow := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := topUpTxWithRowLock(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			saved = *topUp
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		if actualPaymentMethod != "" && topUp.PaymentMethod != actualPaymentMethod {
			topUp.PaymentMethod = actualPaymentMethod
		}
		quotaToAdd = int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		balanceBefore, balanceAfter, err := creditTopUpUserQuotaTx(tx, topUp.UserId, quotaToAdd, nil)
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore
		if err := recordTopUpPromoUsageTx(tx, topUp, false); err != nil {
			return err
		}
		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}
		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, quotaToAdd)); err != nil {
			return err
		}
		saved = *topUp
		completedNow = true
		return nil
	})
	if err != nil {
		return nil, 0, false, err
	}
	if completedNow {
		if err := cacheIncrUserQuota(saved.UserId, int64(quotaToAdd)); err != nil {
			common.SysLog("failed to increase user quota cache after epay topup: " + err.Error())
		}
	}
	return &saved, quotaToAdd, completedNow, nil
}

func CompleteFreeTopUp(tradeNo string, expectedPaymentProvider string) (*TopUp, int, bool, error) {
	if tradeNo == "" {
		return nil, 0, false, errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var saved TopUp
	var quotaToAdd int
	completedNow := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := topUpTxWithRowLock(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			saved = *topUp
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if topUp.PromoCodeId <= 0 || topUp.ActualMoney > 0 || topUp.OriginalMoney <= 0 || topUp.DiscountMoney <= 0 {
			return errors.New("订单不是 0 元优惠订单")
		}

		if topUp.PaymentProvider == PaymentProviderStripe {
			quotaToAdd = int(decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		} else {
			if topUp.Money > 0 {
				return errors.New("订单不是 0 元优惠订单")
			}
			quotaToAdd = int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		}
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		balanceBefore, balanceAfter, err := creditTopUpUserQuotaTx(tx, topUp.UserId, quotaToAdd, nil)
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore
		if err := recordTopUpPromoUsageTx(tx, topUp, true); err != nil {
			return err
		}
		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}
		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, 0)); err != nil {
			return err
		}
		saved = *topUp
		completedNow = true
		return nil
	})
	if err != nil {
		return nil, 0, false, err
	}
	if completedNow {
		if err := cacheIncrUserQuota(saved.UserId, int64(quotaToAdd)); err != nil {
			common.SysLog("failed to increase user quota cache after free topup: " + err.Error())
		}
	}
	return &saved, quotaToAdd, completedNow, nil
}

func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := topUpTxWithRowLock(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota = topUp.Amount

		balanceBefore, balanceAfter, quotaErr := creditTopUpUserQuotaTx(tx, topUp.UserId, quota, nil)
		err = quotaErr
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore

		// 支付邮箱只在用户尚未填写邮箱时补充，避免覆盖现有资料。
		if customerEmail != "" {
			if err := tx.Model(&User{}).Where("id = ? AND email = ?", topUp.UserId, "").Update("email", customerEmail).Error; err != nil {
				return err
			}
		}

		if err := recordTopUpPromoUsageTx(tx, topUp, false); err != nil {
			return err
		}

		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}

		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, int(quota))); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupOrderLog(topUp, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), PaymentMethodCreem, callerIp)

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := topUpTxWithRowLock(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		balanceBefore, balanceAfter, err := creditTopUpUserQuotaTx(tx, topUp.UserId, quotaToAdd, nil)
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore

		if err := recordTopUpPromoUsageTx(tx, topUp, false); err != nil {
			return err
		}

		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}

		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, quotaToAdd)); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordTopupOrderLog(topUp, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), PaymentMethodWaffo, callerIp)
	}

	return nil
}

func RechargeBepusdt(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	completedNow := false
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := topUpTxWithRowLock(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderBepusdt {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending &&
			topUp.Status != common.TopUpStatusFailed &&
			topUp.Status != common.TopUpStatusExpired {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		balanceBefore, balanceAfter, err := creditTopUpUserQuotaTx(tx, topUp.UserId, quotaToAdd, nil)
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore

		if err := recordTopUpPromoUsageTx(tx, topUp, false); err != nil {
			return err
		}

		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}

		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, quotaToAdd)); err != nil {
			return err
		}

		completedNow = true
		return nil
	})

	if err != nil {
		common.SysError("bepusdt topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if completedNow {
		if err := increaseCryptoTopUpCache(topUp.UserId, int64(quotaToAdd)); err != nil {
			common.SysLog("failed to increase user quota cache after bepusdt topup: " + err.Error())
		}
	}

	if completedNow && quotaToAdd > 0 {
		RecordTopupOrderLog(topUp, fmt.Sprintf("Bepusdt USDT充值成功，充值额度: %v，支付金额: %.2f CNY", logger.FormatQuota(quotaToAdd), topUp.Money), PaymentMethodBepusdt, callerIp)
	}

	return nil
}

func RechargeOkpay(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	completedNow := false
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := topUpTxWithRowLock(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderOkpay {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending &&
			topUp.Status != common.TopUpStatusFailed &&
			topUp.Status != common.TopUpStatusExpired {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		balanceBefore, balanceAfter, err := creditTopUpUserQuotaTx(tx, topUp.UserId, quotaToAdd, nil)
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore

		if err := recordTopUpPromoUsageTx(tx, topUp, false); err != nil {
			return err
		}

		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}

		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, quotaToAdd)); err != nil {
			return err
		}

		completedNow = true
		return nil
	})

	if err != nil {
		common.SysError("okpay topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if completedNow {
		if err := increaseCryptoTopUpCache(topUp.UserId, int64(quotaToAdd)); err != nil {
			common.SysLog("failed to increase user quota cache after okpay topup: " + err.Error())
		}
	}

	if completedNow && quotaToAdd > 0 {
		RecordTopupOrderLog(topUp, fmt.Sprintf("OKPay充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), PaymentMethodOkpay, callerIp)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string, callbackIp ...string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := topUpTxWithRowLock(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		quotaToAdd = int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		balanceBefore, balanceAfter, err := creditTopUpUserQuotaTx(tx, topUp.UserId, quotaToAdd, nil)
		if err != nil {
			return err
		}
		topUp.BalanceBefore = balanceBefore
		topUp.BalanceAfter = balanceAfter
		topUp.CreditedQuota = balanceAfter - balanceBefore

		if err := recordTopUpPromoUsageTx(tx, topUp, false); err != nil {
			return err
		}

		if err := CreateInvoiceRecordFromTopUpTx(tx, topUp); err != nil {
			return err
		}

		if err := createAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(topUp, quotaToAdd)); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordTopupOrderLog(topUp, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), PaymentMethodWaffoPancake, callbackIp...)
	}

	return nil
}

// GetUserTotalRechargeAmount 返回用户成功充值的累计实付金额（货币，与 TopUp.Money 同单位）。
// 优惠码订单必须以 actual_money 为准，避免 0 元优惠充值被回退成 money 误算为付费充值。
// 无优惠码的旧数据可能没有 actual_money，才回退到 money。
func GetUserTotalRechargeAmount(userId int) (float64, error) {
	if userId <= 0 {
		return 0, nil
	}
	var amount float64
	err := DB.Model(&TopUp{}).
		Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).
		Select("COALESCE(SUM(CASE WHEN promo_code_id > 0 THEN actual_money ELSE COALESCE(NULLIF(actual_money, 0), money) END), 0)").
		Scan(&amount).Error
	if err != nil {
		return 0, err
	}
	return amount, nil
}
