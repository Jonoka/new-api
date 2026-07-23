package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrInvoicePaymentNotFound         = errors.New("发票支付订单不存在")
	ErrInvoicePaymentSnapshotMismatch = errors.New("发票支付快照不匹配")
	ErrInvoicePaymentStatusInvalid    = errors.New("发票支付状态无效")
)

type InvoiceExternalPaymentOptions struct {
	PaymentMethod      string
	PaymentProvider    string
	ProviderMerchantId string
	RequestIP          string
	ProviderPayload    string
}

type InvoicePaymentCallback struct {
	PaymentProvider    string
	PaymentMethod      string
	ProviderOrderId    string
	ProviderMerchantId string
	ProviderAmount     string
	ProviderCurrency   string
	ProviderPayload    string
}

func invoiceCNYToMinor(amount float64) int64 {
	return decimal.NewFromFloat(amount).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
}

func invoiceMinorCNYText(amountMinor int64) string {
	return decimal.NewFromInt(amountMinor).Div(decimal.NewFromInt(100)).StringFixed(2)
}

func newInvoicePaymentTradeNo(userId int) string {
	return fmt.Sprintf("INVPAYUSR%dNO%s%d", userId, common.GetRandomString(8), time.Now().UnixNano())
}

func validInvoiceExternalProvider(provider string) bool {
	switch provider {
	case PaymentProviderEpay, PaymentProviderBepusdt, PaymentProviderOkpay:
		return true
	default:
		return false
	}
}

// CreateCombinedInvoiceExternalPayment 冻结来源订单并创建独立的服务费支付记录。
// 此事务不会创建充值单，也不会改动用户余额、返佣或优惠码。
func CreateCombinedInvoiceExternalPayment(userId int, references []InvoiceOrderReference, req InvoiceRequest, options InvoiceExternalPaymentOptions) (*InvoiceRecord, error) {
	if userId <= 0 {
		return nil, errors.New("无效的用户")
	}
	options.PaymentProvider = strings.TrimSpace(options.PaymentProvider)
	options.PaymentMethod = strings.TrimSpace(options.PaymentMethod)
	options.ProviderMerchantId = strings.TrimSpace(options.ProviderMerchantId)
	options.RequestIP = strings.TrimSpace(options.RequestIP)
	if !validInvoiceExternalProvider(options.PaymentProvider) || options.PaymentMethod == "" || options.PaymentMethod == PaymentMethodBalance {
		return nil, errors.New("不支持的发票支付方式")
	}

	created := InvoiceRecord{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		orders, err := resolveInvoiceOrdersTx(tx, userId, references)
		if err != nil {
			return err
		}
		preview, err := buildInvoiceOrderPreview(orders)
		if err != nil {
			return err
		}
		req.Required = true
		normalizedReq, validatedFee, err := ValidateInvoiceRequest(req, preview.BaseAmount)
		if err != nil {
			return err
		}
		if !decimal.NewFromFloat(validatedFee).Equal(decimal.NewFromFloat(preview.FeeAmount)) {
			return errors.New("发票费用计算结果发生变化，请重新提交")
		}

		now := common.GetTimestamp()
		paymentMinor := invoiceCNYToMinor(preview.FeeAmount)
		invoiceStatus := InvoiceStatusPaymentPending
		paymentStatus := InvoicePaymentStatusPending
		paidTime := int64(0)
		providerAmount := ""
		providerCurrency := ""
		if options.PaymentProvider == PaymentProviderEpay || options.PaymentProvider == PaymentProviderBepusdt {
			providerAmount = invoiceMinorCNYText(paymentMinor)
			providerCurrency = "CNY"
		}
		if paymentMinor == 0 {
			invoiceStatus = InvoiceStatusPending
			paymentStatus = InvoicePaymentStatusSuccess
			paidTime = now
			providerAmount = "0.00"
			providerCurrency = "CNY"
		}

		created = InvoiceRecord{
			UserId:             userId,
			SourceType:         InvoiceSourceCombined,
			SourceId:           newInvoicePaymentTradeNo(userId),
			PaymentMethod:      options.PaymentMethod,
			PaymentProvider:    options.PaymentProvider,
			PaymentStatus:      paymentStatus,
			ProviderMerchantId: options.ProviderMerchantId,
			ProviderAmount:     providerAmount,
			ProviderCurrency:   providerCurrency,
			PaymentAmountMinor: paymentMinor,
			RequestIP:          options.RequestIP,
			PaidTime:           paidTime,
			ProviderPayload:    options.ProviderPayload,
			InvoiceType:        normalizedReq.Type,
			InvoiceKind:        normalizedReq.Kind,
			Title:              normalizedReq.Title,
			TaxNo:              normalizedReq.TaxNo,
			Email:              normalizedReq.Email,
			Phone:              normalizedReq.Phone,
			Remark:             normalizedReq.Remark,
			BaseAmount:         preview.BaseAmount,
			FeeAmount:          preview.FeeAmount,
			TotalAmount:        preview.TotalAmount,
			Status:             invoiceStatus,
			CreateTime:         now,
			UpdateTime:         now,
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := enqueueInvoicePendingNotificationTx(tx, &created); err != nil {
			return err
		}
		for _, order := range orders {
			link := InvoiceOrderLink{
				InvoiceId:     created.Id,
				UserId:        userId,
				SourceType:    order.Reference.SourceType,
				SourceId:      order.Reference.SourceId,
				PaymentMethod: order.PaymentMethod,
				BaseAmount:    order.BaseAmount,
				CreateTime:    now,
			}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
			if err := applyInvoiceSnapshotToOrderWithStatusTx(tx, userId, order, normalizedReq, invoiceStatus); err != nil {
				return err
			}
			created.Orders = append(created.Orders, link)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func invoicePaymentQuery(tx *gorm.DB, tradeNo string) *gorm.DB {
	return tx.Where("source_type = ? AND source_id = ?", InvoiceSourceCombined, strings.TrimSpace(tradeNo))
}

func GetInvoicePaymentByTradeNo(tradeNo string) (*InvoiceRecord, error) {
	var record InvoiceRecord
	if err := invoicePaymentQuery(DB, tradeNo).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoicePaymentNotFound
		}
		return nil, err
	}
	if err := attachInvoiceOrderLinks([]*InvoiceRecord{&record}); err != nil {
		return nil, err
	}
	return &record, nil
}

func GetUserInvoicePaymentByTradeNo(userId int, tradeNo string) (*InvoiceRecord, error) {
	var record InvoiceRecord
	if err := invoicePaymentQuery(DB, tradeNo).Where("user_id = ?", userId).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoicePaymentNotFound
		}
		return nil, err
	}
	if err := attachInvoiceOrderLinks([]*InvoiceRecord{&record}); err != nil {
		return nil, err
	}
	return &record, nil
}

func GetInvoicePaymentByProviderOrderId(provider string, providerOrderId string) (*InvoiceRecord, error) {
	var records []InvoiceRecord
	err := DB.Where("source_type = ? AND payment_provider = ? AND provider_order_id = ?", InvoiceSourceCombined, strings.TrimSpace(provider), strings.TrimSpace(providerOrderId)).Limit(2).Find(&records).Error
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrInvoicePaymentNotFound
	}
	if len(records) > 1 {
		return nil, errors.New("第三方订单号匹配到多个发票支付记录")
	}
	return &records[0], nil
}

func mergeInvoiceProviderSnapshot(record *InvoiceRecord, snapshot InvoicePaymentCallback) error {
	if record.PaymentProvider != strings.TrimSpace(snapshot.PaymentProvider) {
		return fmt.Errorf("%w: 支付网关不匹配", ErrInvoicePaymentSnapshotMismatch)
	}
	if snapshot.PaymentMethod != "" && record.PaymentMethod != strings.TrimSpace(snapshot.PaymentMethod) {
		return fmt.Errorf("%w: 支付方式不匹配", ErrInvoicePaymentSnapshotMismatch)
	}
	if value := strings.TrimSpace(snapshot.ProviderMerchantId); value != "" {
		if record.ProviderMerchantId != "" && record.ProviderMerchantId != value {
			return fmt.Errorf("%w: 商户号不匹配", ErrInvoicePaymentSnapshotMismatch)
		}
		record.ProviderMerchantId = value
	}
	if value := strings.TrimSpace(snapshot.ProviderAmount); value != "" {
		actual, err := decimal.NewFromString(value)
		if err != nil {
			return fmt.Errorf("%w: 支付金额无效", ErrInvoicePaymentSnapshotMismatch)
		}
		if record.ProviderAmount != "" {
			expected, err := decimal.NewFromString(record.ProviderAmount)
			if err != nil || !actual.Equal(expected) {
				return fmt.Errorf("%w: 支付金额不匹配", ErrInvoicePaymentSnapshotMismatch)
			}
		}
		record.ProviderAmount = value
	}
	if value := strings.ToUpper(strings.TrimSpace(snapshot.ProviderCurrency)); value != "" {
		if record.ProviderCurrency != "" && strings.ToUpper(record.ProviderCurrency) != value {
			return fmt.Errorf("%w: 支付币种不匹配", ErrInvoicePaymentSnapshotMismatch)
		}
		record.ProviderCurrency = value
	}
	if value := strings.TrimSpace(snapshot.ProviderOrderId); value != "" {
		if record.ProviderOrderId != "" && record.ProviderOrderId != value {
			return fmt.Errorf("%w: 第三方订单号不匹配", ErrInvoicePaymentSnapshotMismatch)
		}
		record.ProviderOrderId = value
	}
	if strings.TrimSpace(snapshot.ProviderPayload) != "" {
		record.ProviderPayload = snapshot.ProviderPayload
	}
	return nil
}

func UpdateInvoicePaymentProviderSnapshot(tradeNo string, snapshot InvoicePaymentCallback) (*InvoiceRecord, error) {
	var updated InvoiceRecord
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := invoicePaymentQuery(lockForUpdate(tx), tradeNo).First(&updated).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoicePaymentNotFound
			}
			return err
		}
		if err := mergeInvoiceProviderSnapshot(&updated, snapshot); err != nil {
			return err
		}
		if updated.ProviderOrderId != "" {
			var duplicateCount int64
			if err := tx.Model(&InvoiceRecord{}).Where("id <> ? AND payment_provider = ? AND provider_order_id = ?", updated.Id, updated.PaymentProvider, updated.ProviderOrderId).Count(&duplicateCount).Error; err != nil {
				return err
			}
			if duplicateCount > 0 {
				return fmt.Errorf("%w: 第三方订单号已被使用", ErrInvoicePaymentSnapshotMismatch)
			}
		}
		updated.UpdateTime = common.GetTimestamp()
		return tx.Save(&updated).Error
	})
	if err != nil {
		return nil, err
	}
	if err := attachInvoiceOrderLinks([]*InvoiceRecord{&updated}); err != nil {
		return nil, err
	}
	return &updated, nil
}

func validateInvoicePaymentCompletion(record *InvoiceRecord, callback InvoicePaymentCallback) error {
	if strings.TrimSpace(callback.PaymentMethod) == "" ||
		strings.TrimSpace(callback.ProviderOrderId) == "" ||
		strings.TrimSpace(callback.ProviderAmount) == "" ||
		strings.TrimSpace(callback.ProviderCurrency) == "" {
		return fmt.Errorf("%w: 回调支付快照不完整", ErrInvoicePaymentSnapshotMismatch)
	}
	// OKPay 下单会同步返回完整的金额、币种和第三方订单号；禁止用回调补齐
	// 旧式空快照，否则无法证明回调对应的是本次下单时确认的汇率与订单。
	if record.PaymentProvider == PaymentProviderOkpay &&
		(record.ProviderAmount == "" || record.ProviderCurrency == "" || record.ProviderOrderId == "" || record.ProviderMerchantId == "") {
		return fmt.Errorf("%w: OKPay 第三方支付快照不完整", ErrInvoicePaymentSnapshotMismatch)
	}
	if err := mergeInvoiceProviderSnapshot(record, callback); err != nil {
		return err
	}
	if record.ProviderAmount == "" || record.ProviderCurrency == "" || record.ProviderOrderId == "" {
		return fmt.Errorf("%w: 第三方支付快照不完整", ErrInvoicePaymentSnapshotMismatch)
	}
	if record.PaymentProvider == PaymentProviderEpay || record.PaymentProvider == PaymentProviderOkpay {
		if record.ProviderMerchantId == "" || strings.TrimSpace(callback.ProviderMerchantId) == "" {
			return fmt.Errorf("%w: 商户号快照不完整", ErrInvoicePaymentSnapshotMismatch)
		}
	}
	if record.PaymentProvider == PaymentProviderEpay || record.PaymentProvider == PaymentProviderBepusdt {
		if strings.ToUpper(record.ProviderCurrency) != "CNY" {
			return fmt.Errorf("%w: 支付币种必须为 CNY", ErrInvoicePaymentSnapshotMismatch)
		}
		amount, err := decimal.NewFromString(record.ProviderAmount)
		if err != nil || !amount.Mul(decimal.NewFromInt(100)).Equal(decimal.NewFromInt(record.PaymentAmountMinor)) {
			return fmt.Errorf("%w: 人民币金额与分值不一致", ErrInvoicePaymentSnapshotMismatch)
		}
	}
	return nil
}

// CompleteInvoiceExternalPayment 在同一事务内完成快照复核、状态转换和来源状态同步。
func CompleteInvoiceExternalPayment(tradeNo string, callback InvoicePaymentCallback) (*InvoiceRecord, bool, error) {
	var completed InvoiceRecord
	completedNow := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := invoicePaymentQuery(lockForUpdate(tx), tradeNo).First(&completed).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoicePaymentNotFound
			}
			return err
		}
		if err := validateInvoicePaymentCompletion(&completed, callback); err != nil {
			return err
		}
		if completed.PaymentStatus == InvoicePaymentStatusSuccess {
			return nil
		}
		if completed.Status != InvoiceStatusPaymentPending {
			return ErrInvoicePaymentStatusInvalid
		}
		switch completed.PaymentStatus {
		case InvoicePaymentStatusPending, InvoicePaymentStatusFailed, InvoicePaymentStatusExpired:
		default:
			return ErrInvoicePaymentStatusInvalid
		}
		if completed.ProviderOrderId != "" {
			var duplicateCount int64
			if err := tx.Model(&InvoiceRecord{}).Where("id <> ? AND payment_provider = ? AND provider_order_id = ?", completed.Id, completed.PaymentProvider, completed.ProviderOrderId).Count(&duplicateCount).Error; err != nil {
				return err
			}
			if duplicateCount > 0 {
				return fmt.Errorf("%w: 第三方订单号已被使用", ErrInvoicePaymentSnapshotMismatch)
			}
		}
		now := common.GetTimestamp()
		completed.PaymentStatus = InvoicePaymentStatusSuccess
		completed.Status = InvoiceStatusPending
		completed.PaidTime = now
		completed.UpdateTime = now
		if err := tx.Save(&completed).Error; err != nil {
			return err
		}
		if err := syncInvoiceSourceStatusTx(tx, &completed); err != nil {
			return err
		}
		if err := enqueueInvoicePendingNotificationTx(tx, &completed); err != nil {
			return err
		}
		completedNow = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if err := attachInvoiceOrderLinks([]*InvoiceRecord{&completed}); err != nil {
		return nil, false, err
	}
	return &completed, completedNow, nil
}

func clearInvoicePaymentSourceTx(tx *gorm.DB, userId int, link InvoiceOrderLink) error {
	updates := map[string]interface{}{
		"invoice_required":    false,
		"invoice_type":        "",
		"invoice_kind":        "",
		"invoice_title":       "",
		"invoice_tax_no":      "",
		"invoice_email":       "",
		"invoice_phone":       "",
		"invoice_remark":      "",
		"invoice_base_amount": 0,
		"invoice_fee_amount":  0,
		"invoice_status":      "",
	}
	var result *gorm.DB
	switch link.SourceType {
	case InvoiceSourceTopUp:
		result = tx.Model(&TopUp{}).
			Where("user_id = ? AND trade_no = ? AND invoice_status = ?", userId, link.SourceId, InvoiceStatusPaymentPending).
			Updates(updates)
	case InvoiceSourceSubscription:
		result = tx.Model(&SubscriptionOrder{}).
			Where("user_id = ? AND trade_no = ? AND invoice_status = ?", userId, link.SourceId, InvoiceStatusPaymentPending).
			Updates(updates)
		if result.Error == nil && result.RowsAffected == 1 {
			if err := tx.Model(&TopUp{}).
				Where("user_id = ? AND trade_no = ? AND invoice_status = ?", userId, link.SourceId, InvoiceStatusPaymentPending).
				Updates(updates).Error; err != nil {
				return err
			}
		}
	default:
		return errors.New("订单类型无效")
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("待支付开票来源状态已变化，无法取消")
	}
	return nil
}

func cancelInvoiceExternalPaymentTx(tx *gorm.DB, record *InvoiceRecord) error {
	if tx == nil || record == nil {
		return ErrInvoicePaymentNotFound
	}
	if record.SourceType != InvoiceSourceCombined || record.Status != InvoiceStatusPaymentPending || record.PaymentStatus == InvoicePaymentStatusSuccess {
		return ErrInvoicePaymentStatusInvalid
	}
	var links []InvoiceOrderLink
	if err := tx.Where("invoice_id = ?", record.Id).Order("id asc").Find(&links).Error; err != nil {
		return err
	}
	if len(links) == 0 {
		return errors.New("待支付开票申请没有可释放的来源订单")
	}
	for _, link := range links {
		if err := clearInvoicePaymentSourceTx(tx, record.UserId, link); err != nil {
			return err
		}
	}
	if err := tx.Where("invoice_id = ?", record.Id).Delete(&InvoiceOrderLink{}).Error; err != nil {
		return err
	}
	record.PaymentStatus = InvoicePaymentStatusCanceled
	record.Status = InvoiceStatusClosed
	record.UpdateTime = common.GetTimestamp()
	record.Orders = nil
	return tx.Save(record).Error
}

// CancelInvoiceExternalPayment 取消尚未支付的开票申请，并释放被占用的来源订单。
func CancelInvoiceExternalPayment(userId int, tradeNo string) (*InvoiceRecord, error) {
	if userId <= 0 || strings.TrimSpace(tradeNo) == "" {
		return nil, ErrInvoicePaymentNotFound
	}
	var canceled InvoiceRecord
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := invoicePaymentQuery(lockForUpdate(tx), tradeNo).
			Where("user_id = ?", userId).
			First(&canceled).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoicePaymentNotFound
			}
			return err
		}
		canceled.AdminRemark = "用户取消待支付申请"
		return cancelInvoiceExternalPaymentTx(tx, &canceled)
	})
	if err != nil {
		return nil, err
	}
	return &canceled, nil
}

func UpdateInvoiceExternalPaymentStatus(tradeNo string, provider string, status string, providerPayload string) error {
	if status != InvoicePaymentStatusFailed && status != InvoicePaymentStatusExpired {
		return ErrInvoicePaymentStatusInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var record InvoiceRecord
		if err := invoicePaymentQuery(lockForUpdate(tx), tradeNo).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoicePaymentNotFound
			}
			return err
		}
		if record.PaymentProvider != strings.TrimSpace(provider) {
			return fmt.Errorf("%w: 支付网关不匹配", ErrInvoicePaymentSnapshotMismatch)
		}
		if record.PaymentStatus == InvoicePaymentStatusSuccess {
			return nil
		}
		if record.PaymentStatus == InvoicePaymentStatusCanceled || record.Status != InvoiceStatusPaymentPending {
			return nil
		}
		record.PaymentStatus = status
		record.ProviderPayload = providerPayload
		record.UpdateTime = common.GetTimestamp()
		return tx.Save(&record).Error
	})
}
