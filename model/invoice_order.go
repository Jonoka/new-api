package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	// InvoiceSourceCombined 表示一张发票由多笔历史订单合并申请。
	InvoiceSourceCombined = "batch"

	invoiceOrderWindowSeconds = int64(30 * 24 * 60 * 60)
	maxInvoiceOrderSelection  = 100
)

// InvoiceOrderReference 是用户选择历史订单时使用的稳定引用。
type InvoiceOrderReference struct {
	SourceType string `json:"source_type"`
	SourceId   string `json:"source_id"`
}

// InvoiceEligibleOrder 是发票中心展示的近 30 天已支付订单快照。
type InvoiceEligibleOrder struct {
	SourceType      string  `json:"source_type"`
	SourceId        string  `json:"source_id"`
	PaymentMethod   string  `json:"payment_method"`
	PaymentProvider string  `json:"payment_provider"`
	PaidAmount      float64 `json:"paid_amount"`
	InvoiceAmount   float64 `json:"invoice_amount"`
	Currency        string  `json:"currency"`
	CompleteTime    int64   `json:"complete_time"`
	Invoiced        bool    `json:"invoiced"`
	InvoiceEligible bool    `json:"invoice_eligible"`
	InvoiceStatus   string  `json:"invoice_status"`
}

// InvoiceOrderLink 记录合并发票与来源订单的关系。
// 来源类型与来源订单号的唯一索引是防止并发重复开票的最后一道保护。
type InvoiceOrderLink struct {
	Id            int     `json:"id"`
	InvoiceId     int     `json:"invoice_id" gorm:"index"`
	UserId        int     `json:"user_id" gorm:"index"`
	SourceType    string  `json:"source_type" gorm:"type:varchar(32);uniqueIndex:idx_invoice_order_source,priority:1"`
	SourceId      string  `json:"source_id" gorm:"type:varchar(255);uniqueIndex:idx_invoice_order_source,priority:2"`
	PaymentMethod string  `json:"payment_method" gorm:"type:varchar(50)"`
	BaseAmount    float64 `json:"base_amount"`
	CreateTime    int64   `json:"create_time" gorm:"index"`
}

type InvoiceOrderPreview struct {
	BaseAmount  float64 `json:"order_amount"`
	FeeAmount   float64 `json:"invoice_fee"`
	TotalAmount float64 `json:"invoice_total_amount"`
	FeeQuota    int     `json:"fee_quota"`
	Currency    string  `json:"currency"`
}

type resolvedInvoiceOrder struct {
	Reference       InvoiceOrderReference
	PaymentMethod   string
	PaymentProvider string
	PaidAmount      float64
	BaseAmount      float64
	CompleteTime    int64
}

func invoiceOrderCutoff() int64 {
	return common.GetTimestamp() - invoiceOrderWindowSeconds
}

func invoiceOrderPaidAmount(money float64, actualMoney float64, promoCodeId int) float64 {
	// 使用过优惠码的订单必须信任实付快照；全额优惠时 actual_money 合法地为 0。
	if promoCodeId > 0 {
		return decimal.NewFromFloat(actualMoney).Round(2).InexactFloat64()
	}
	if actualMoney > 0 {
		return decimal.NewFromFloat(actualMoney).Round(2).InexactFloat64()
	}
	return decimal.NewFromFloat(money).Round(2).InexactFloat64()
}

func invoiceOrderPaymentProvider(paymentProvider string, paymentMethod string) string {
	paymentProvider = strings.TrimSpace(paymentProvider)
	if paymentProvider != "" {
		return paymentProvider
	}
	switch strings.TrimSpace(paymentMethod) {
	case PaymentMethodStripe:
		return PaymentProviderStripe
	case PaymentMethodCreem:
		return PaymentProviderCreem
	case PaymentMethodWaffo:
		return PaymentProviderWaffo
	case PaymentMethodWaffoPancake:
		return PaymentProviderWaffoPancake
	case PaymentMethodBalance:
		return PaymentProviderBalance
	case PaymentMethodBepusdt:
		return PaymentProviderBepusdt
	case PaymentMethodOkpay:
		return PaymentProviderOkpay
	default:
		return PaymentProviderEpay
	}
}

func invoiceOrderCompleteTime(completeTime int64, createTime int64) int64 {
	if completeTime > 0 {
		return completeTime
	}
	return createTime
}

func invoiceOrderAmountCNY(paidAmount float64, paymentProvider string) float64 {
	return decimal.NewFromFloat(PaymentAmountToCNY(paidAmount, paymentProvider)).Round(2).InexactFloat64()
}

func invoiceOrderKey(sourceType string, sourceId string) string {
	return strings.TrimSpace(sourceType) + "\x00" + strings.TrimSpace(sourceId)
}

// GetRecentInvoiceOrders 返回近 30 天的成功订单，并明确标记是否仍可申请发票。
// 已经提交过申请的订单保留在结果中供用户核对，但前端必须禁选。
func GetRecentInvoiceOrders(userId int) ([]InvoiceEligibleOrder, error) {
	if userId <= 0 {
		return nil, errors.New("无效的用户")
	}
	cutoff := invoiceOrderCutoff()

	var topUps []TopUp
	if err := DB.Where(
		"user_id = ? AND status = ? AND (complete_time >= ? OR (complete_time = 0 AND create_time >= ?))",
		userId, common.TopUpStatusSuccess, cutoff, cutoff,
	).Find(&topUps).Error; err != nil {
		return nil, err
	}

	var subscriptions []SubscriptionOrder
	if err := DB.Where(
		"user_id = ? AND status = ? AND (complete_time >= ? OR (complete_time = 0 AND create_time >= ?))",
		userId, common.TopUpStatusSuccess, cutoff, cutoff,
	).Find(&subscriptions).Error; err != nil {
		return nil, err
	}

	var records []InvoiceRecord
	if err := DB.Select("source_type", "source_id", "status").
		Where("user_id = ? AND source_type IN ?", userId, []string{InvoiceSourceTopUp, InvoiceSourceSubscription}).
		Find(&records).Error; err != nil {
		return nil, err
	}
	recordStatus := make(map[string]string, len(records))
	for _, record := range records {
		status := strings.TrimSpace(record.Status)
		if status == "" {
			status = InvoiceStatusPending
		}
		recordStatus[invoiceOrderKey(record.SourceType, record.SourceId)] = status
	}

	var links []InvoiceOrderLink
	if err := DB.Where("user_id = ?", userId).Find(&links).Error; err != nil {
		return nil, err
	}
	linkedStatus := make(map[string]string, len(links))
	if len(links) > 0 {
		invoiceIds := make([]int, 0, len(links))
		for _, link := range links {
			invoiceIds = append(invoiceIds, link.InvoiceId)
		}
		var linkedRecords []InvoiceRecord
		if err := DB.Select("id", "status").Where("id IN ?", invoiceIds).Find(&linkedRecords).Error; err != nil {
			return nil, err
		}
		statuses := make(map[int]string, len(linkedRecords))
		for _, record := range linkedRecords {
			statuses[record.Id] = record.Status
		}
		for _, link := range links {
			status := statuses[link.InvoiceId]
			if status == "" {
				status = InvoiceStatusPending
			}
			linkedStatus[invoiceOrderKey(link.SourceType, link.SourceId)] = status
		}
	}

	// 部分订阅支付会同步生成一条同订单号的充值记录，发票中心只展示订阅订单。
	subscriptionTradeNos := make(map[string]struct{}, len(subscriptions))
	for _, order := range subscriptions {
		subscriptionTradeNos[order.TradeNo] = struct{}{}
	}

	orders := make([]InvoiceEligibleOrder, 0, len(topUps)+len(subscriptions))
	appendOrder := func(
		sourceType string,
		sourceId string,
		paymentMethod string,
		paymentProvider string,
		money float64,
		actualMoney float64,
		paidAmountCNY float64,
		promoCodeId int,
		completeTime int64,
		createTime int64,
		invoiceRequired bool,
		invoiceStatus string,
	) {
		paidAmount := invoiceOrderPaidAmount(money, actualMoney, promoCodeId)
		if paidAmount <= 0 || strings.TrimSpace(sourceId) == "" {
			return
		}
		provider := invoiceOrderPaymentProvider(paymentProvider, paymentMethod)
		key := invoiceOrderKey(sourceType, sourceId)
		status := strings.TrimSpace(invoiceStatus)
		if linked, ok := linkedStatus[key]; ok {
			status = linked
		}
		if direct, ok := recordStatus[key]; ok {
			status = direct
		}
		invoiceAmount := decimal.NewFromFloat(paidAmountCNY).Round(2).InexactFloat64()
		if invoiceAmount <= 0 {
			invoiceAmount = invoiceOrderAmountCNY(paidAmount, provider)
		}
		invoiced := invoiceRequired || status != ""
		invoiceEligible := !invoiced && invoiceAmount > 0
		orders = append(orders, InvoiceEligibleOrder{
			SourceType:      sourceType,
			SourceId:        sourceId,
			PaymentMethod:   paymentMethod,
			PaymentProvider: provider,
			PaidAmount:      invoiceAmount,
			InvoiceAmount:   invoiceAmount,
			Currency:        "CNY",
			CompleteTime:    invoiceOrderCompleteTime(completeTime, createTime),
			Invoiced:        invoiced,
			InvoiceEligible: invoiceEligible,
			InvoiceStatus:   status,
		})
	}

	for _, topUp := range topUps {
		if _, duplicatedSubscription := subscriptionTradeNos[topUp.TradeNo]; duplicatedSubscription {
			continue
		}
		appendOrder(
			InvoiceSourceTopUp, topUp.TradeNo, topUp.PaymentMethod, topUp.PaymentProvider,
			topUp.Money, topUp.ActualMoney, topUp.PaidAmountCNY, topUp.PromoCodeId, topUp.CompleteTime, topUp.CreateTime,
			topUp.InvoiceRequired, topUp.InvoiceStatus,
		)
	}
	for _, order := range subscriptions {
		appendOrder(
			InvoiceSourceSubscription, order.TradeNo, order.PaymentMethod, order.PaymentProvider,
			order.Money, order.ActualMoney, order.PaidAmountCNY, order.PromoCodeId, order.CompleteTime, order.CreateTime,
			order.InvoiceRequired, order.InvoiceStatus,
		)
	}

	sort.SliceStable(orders, func(i, j int) bool {
		if orders[i].CompleteTime == orders[j].CompleteTime {
			return orders[i].SourceId > orders[j].SourceId
		}
		return orders[i].CompleteTime > orders[j].CompleteTime
	})
	return orders, nil
}

func normalizeInvoiceOrderReferences(references []InvoiceOrderReference) ([]InvoiceOrderReference, error) {
	if len(references) == 0 {
		return nil, errors.New("请至少选择一笔订单")
	}
	if len(references) > maxInvoiceOrderSelection {
		return nil, fmt.Errorf("单次最多选择 %d 笔订单", maxInvoiceOrderSelection)
	}
	normalized := make([]InvoiceOrderReference, 0, len(references))
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		reference.SourceType = strings.TrimSpace(reference.SourceType)
		reference.SourceId = strings.TrimSpace(reference.SourceId)
		if reference.SourceType != InvoiceSourceTopUp && reference.SourceType != InvoiceSourceSubscription {
			return nil, errors.New("订单类型无效")
		}
		if reference.SourceId == "" {
			return nil, errors.New("订单号不能为空")
		}
		if len(reference.SourceId) > 255 {
			return nil, errors.New("订单号长度无效")
		}
		key := invoiceOrderKey(reference.SourceType, reference.SourceId)
		if _, exists := seen[key]; exists {
			return nil, errors.New("不能重复选择同一订单")
		}
		seen[key] = struct{}{}
		normalized = append(normalized, reference)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].SourceType == normalized[j].SourceType {
			return normalized[i].SourceId < normalized[j].SourceId
		}
		return normalized[i].SourceType < normalized[j].SourceType
	})
	return normalized, nil
}

func resolveInvoiceOrdersTx(tx *gorm.DB, userId int, references []InvoiceOrderReference) ([]resolvedInvoiceOrder, error) {
	if tx == nil {
		return nil, errors.New("数据库事务不可用")
	}
	normalized, err := normalizeInvoiceOrderReferences(references)
	if err != nil {
		return nil, err
	}
	cutoff := invoiceOrderCutoff()
	resolved := make([]resolvedInvoiceOrder, 0, len(normalized))
	for _, reference := range normalized {
		var (
			paymentMethod   string
			paymentProvider string
			paidAmount      float64
			baseAmount      float64
			completeTime    int64
			invoiceRequired bool
		)
		switch reference.SourceType {
		case InvoiceSourceTopUp:
			var topUp TopUp
			if err := tx.Set("gorm:query_option", "FOR UPDATE").
				Where("user_id = ? AND trade_no = ? AND status = ?", userId, reference.SourceId, common.TopUpStatusSuccess).
				First(&topUp).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, errors.New("所选充值订单不存在或尚未支付")
				}
				return nil, err
			}
			var subscriptionCount int64
			if err := tx.Model(&SubscriptionOrder{}).Where("trade_no = ?", topUp.TradeNo).Count(&subscriptionCount).Error; err != nil {
				return nil, err
			}
			if subscriptionCount > 0 {
				return nil, errors.New("订阅订单必须使用订阅来源申请发票")
			}
			paymentMethod = topUp.PaymentMethod
			paymentProvider = invoiceOrderPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod)
			paidAmount = invoiceOrderPaidAmount(topUp.Money, topUp.ActualMoney, topUp.PromoCodeId)
			baseAmount = decimal.NewFromFloat(topUp.PaidAmountCNY).Round(2).InexactFloat64()
			completeTime = invoiceOrderCompleteTime(topUp.CompleteTime, topUp.CreateTime)
			invoiceRequired = topUp.InvoiceRequired
		case InvoiceSourceSubscription:
			var order SubscriptionOrder
			if err := tx.Set("gorm:query_option", "FOR UPDATE").
				Where("user_id = ? AND trade_no = ? AND status = ?", userId, reference.SourceId, common.TopUpStatusSuccess).
				First(&order).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, errors.New("所选订阅订单不存在或尚未支付")
				}
				return nil, err
			}
			paymentMethod = order.PaymentMethod
			paymentProvider = invoiceOrderPaymentProvider(order.PaymentProvider, order.PaymentMethod)
			paidAmount = invoiceOrderPaidAmount(order.Money, order.ActualMoney, order.PromoCodeId)
			baseAmount = decimal.NewFromFloat(order.PaidAmountCNY).Round(2).InexactFloat64()
			completeTime = invoiceOrderCompleteTime(order.CompleteTime, order.CreateTime)
			invoiceRequired = order.InvoiceRequired
		}
		if completeTime < cutoff {
			return nil, errors.New("仅支持申请近 30 天内订单的发票")
		}
		if paidAmount <= 0 {
			return nil, errors.New("零金额订单不能申请发票")
		}
		if invoiceRequired {
			return nil, errors.New("所选订单已经申请过发票")
		}
		var directCount int64
		if err := tx.Model(&InvoiceRecord{}).
			Where("source_type = ? AND source_id = ?", reference.SourceType, reference.SourceId).
			Count(&directCount).Error; err != nil {
			return nil, err
		}
		if directCount > 0 {
			return nil, errors.New("所选订单已经申请过发票")
		}
		var linkCount int64
		if err := tx.Model(&InvoiceOrderLink{}).
			Where("source_type = ? AND source_id = ?", reference.SourceType, reference.SourceId).
			Count(&linkCount).Error; err != nil {
			return nil, err
		}
		if linkCount > 0 {
			return nil, errors.New("所选订单已经申请过发票")
		}
		if baseAmount <= 0 {
			baseAmount = invoiceOrderAmountCNY(paidAmount, paymentProvider)
		}
		if baseAmount <= 0 {
			return nil, errors.New("订单开票金额换算失败，请检查人民币汇率配置")
		}
		resolved = append(resolved, resolvedInvoiceOrder{
			Reference:       reference,
			PaymentMethod:   paymentMethod,
			PaymentProvider: paymentProvider,
			PaidAmount:      paidAmount,
			BaseAmount:      baseAmount,
			CompleteTime:    completeTime,
		})
	}
	return resolved, nil
}

func buildInvoiceOrderPreview(orders []resolvedInvoiceOrder) (InvoiceOrderPreview, error) {
	base := decimal.Zero
	for _, order := range orders {
		base = base.Add(decimal.NewFromFloat(order.BaseAmount))
	}
	base = base.Round(2)
	fee, err := CalculateInvoiceFee(base.InexactFloat64())
	if err != nil {
		return InvoiceOrderPreview{}, err
	}
	feeUSD := AmountCNYToPaymentCurrency(fee, PaymentProviderBalance)
	if fee > 0 && feeUSD <= 0 {
		return InvoiceOrderPreview{}, errors.New("人民币汇率配置错误")
	}
	feeQuota, err := calcSubscriptionBalanceQuota(feeUSD)
	if err != nil {
		return InvoiceOrderPreview{}, err
	}
	return InvoiceOrderPreview{
		BaseAmount:  base.InexactFloat64(),
		FeeAmount:   decimal.NewFromFloat(fee).Round(2).InexactFloat64(),
		TotalAmount: base.Add(decimal.NewFromFloat(fee)).Round(2).InexactFloat64(),
		FeeQuota:    feeQuota,
		Currency:    "CNY",
	}, nil
}

func PreviewInvoiceOrders(userId int, references []InvoiceOrderReference) (InvoiceOrderPreview, error) {
	if userId <= 0 {
		return InvoiceOrderPreview{}, errors.New("无效的用户")
	}
	if !InvoiceEnabled {
		return InvoiceOrderPreview{}, errors.New("当前不支持开发票")
	}
	var preview InvoiceOrderPreview
	err := DB.Transaction(func(tx *gorm.DB) error {
		orders, err := resolveInvoiceOrdersTx(tx, userId, references)
		if err != nil {
			return err
		}
		preview, err = buildInvoiceOrderPreview(orders)
		return err
	})
	return preview, err
}

func applyInvoiceSnapshotToOrderTx(tx *gorm.DB, userId int, order resolvedInvoiceOrder, req InvoiceRequest) error {
	updates := map[string]interface{}{
		"invoice_required":    true,
		"invoice_type":        req.Type,
		"invoice_title":       req.Title,
		"invoice_tax_no":      req.TaxNo,
		"invoice_email":       req.Email,
		"invoice_phone":       req.Phone,
		"invoice_remark":      req.Remark,
		"invoice_base_amount": order.BaseAmount,
		"invoice_fee_amount":  0,
		"invoice_status":      InvoiceStatusPending,
	}
	var result *gorm.DB
	switch order.Reference.SourceType {
	case InvoiceSourceTopUp:
		result = tx.Model(&TopUp{}).
			Where("user_id = ? AND trade_no = ? AND status = ? AND invoice_required = ?", userId, order.Reference.SourceId, common.TopUpStatusSuccess, false).
			Updates(updates)
	case InvoiceSourceSubscription:
		result = tx.Model(&SubscriptionOrder{}).
			Where("user_id = ? AND trade_no = ? AND status = ? AND invoice_required = ?", userId, order.Reference.SourceId, common.TopUpStatusSuccess, false).
			Updates(updates)
		if result.Error == nil && result.RowsAffected == 1 {
			// 订阅支付可能存在同订单号的充值镜像，保持其发票状态一致。
			if err := tx.Model(&TopUp{}).Where("user_id = ? AND trade_no = ?", userId, order.Reference.SourceId).Updates(updates).Error; err != nil {
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
		return errors.New("所选订单已经申请过发票")
	}
	return nil
}

// CreateCombinedInvoiceWithBalance 使用账户余额支付发票服务费，并创建一张合并发票申请。
func CreateCombinedInvoiceWithBalance(userId int, references []InvoiceOrderReference, req InvoiceRequest) (*InvoiceRecord, error) {
	if userId <= 0 {
		return nil, errors.New("无效的用户")
	}
	var (
		created      InvoiceRecord
		chargedQuota int
	)
	err := DB.Transaction(func(tx *gorm.DB) error {
		orders, err := resolveInvoiceOrdersTx(tx, userId, references)
		if err != nil {
			return err
		}
		preview, err := buildInvoiceOrderPreview(orders)
		if err != nil {
			return err
		}
		normalizedReq, validatedFee, err := ValidateInvoiceRequest(req, preview.BaseAmount)
		if err != nil {
			return err
		}
		if !normalizedReq.Required {
			return errors.New("请填写发票申请信息")
		}
		if !decimal.NewFromFloat(validatedFee).Equal(decimal.NewFromFloat(preview.FeeAmount)) {
			return errors.New("发票费用计算结果发生变化，请重新提交")
		}

		var user User
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		if preview.FeeQuota > 0 {
			result := tx.Model(&User{}).
				Where("id = ? AND quota >= ?", userId, preview.FeeQuota).
				Update("quota", gorm.Expr("quota - ?", preview.FeeQuota))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("余额不足")
			}
		}

		now := common.GetTimestamp()
		created = InvoiceRecord{
			UserId:        userId,
			SourceType:    InvoiceSourceCombined,
			SourceId:      fmt.Sprintf("INVUSR%d%s%d", userId, common.GetRandomString(8), time.Now().UnixNano()),
			PaymentMethod: PaymentMethodBalance,
			InvoiceType:   normalizedReq.Type,
			Title:         normalizedReq.Title,
			TaxNo:         normalizedReq.TaxNo,
			Email:         normalizedReq.Email,
			Phone:         normalizedReq.Phone,
			Remark:        normalizedReq.Remark,
			BaseAmount:    preview.BaseAmount,
			FeeAmount:     preview.FeeAmount,
			TotalAmount:   preview.TotalAmount,
			Status:        InvoiceStatusPending,
			CreateTime:    now,
			UpdateTime:    now,
		}
		if err := tx.Create(&created).Error; err != nil {
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
			if err := applyInvoiceSnapshotToOrderTx(tx, userId, order, normalizedReq); err != nil {
				return err
			}
			created.Orders = append(created.Orders, link)
		}
		chargedQuota = preview.FeeQuota
		return nil
	})
	if err != nil {
		return nil, err
	}
	if chargedQuota > 0 {
		if err := cacheDecrUserQuota(userId, int64(chargedQuota)); err != nil {
			common.SysLog("failed to decrease user quota cache after invoice application: " + err.Error())
		}
	}
	RecordLog(
		userId,
		LogTypeTopup,
		fmt.Sprintf("使用余额支付发票服务费，开票金额: %.2f，服务费: %.2f，扣除额度: %s", created.BaseAmount, created.FeeAmount, logger.LogQuota(chargedQuota)),
	)
	return &created, nil
}
