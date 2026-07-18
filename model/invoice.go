package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	InvoiceTypePersonal = "personal"
	InvoiceTypeCompany  = "company"
	InvoiceKindNormal   = "normal"
	InvoiceKindSpecial  = "special"

	InvoiceSourceTopUp        = "topup"
	InvoiceSourceSubscription = "subscription"

	InvoiceStatusPending = "pending"
	// InvoiceStatusPaymentPending 表示服务费尚未确认支付，不能进入开票流程。
	InvoiceStatusPaymentPending = "payment_pending"
	InvoiceStatusIssued         = "issued"
	InvoiceStatusClosed         = "closed"

	InvoicePaymentStatusPending  = "pending"
	InvoicePaymentStatusSuccess  = "success"
	InvoicePaymentStatusFailed   = "failed"
	InvoicePaymentStatusExpired  = "expired"
	InvoicePaymentStatusCanceled = "canceled"

	InvoiceFeeRuleFixed   = "fixed"
	InvoiceFeeRulePercent = "percent"
)

const (
	defaultInvoiceTypes    = `["personal","company"]`
	defaultInvoiceKinds    = `["normal"]`
	defaultInvoiceFeeRules = `[
  {"min":0,"max":500,"type":"fixed","value":50},
  {"min":501,"max":2000,"type":"fixed","value":100},
  {"min":2001,"max":5000,"type":"fixed","value":175},
  {"min":5000,"type":"percent","value":5}
]`
)

var (
	InvoiceEnabled  = false
	InvoiceTypes    = defaultInvoiceTypes
	InvoiceKinds    = defaultInvoiceKinds
	InvoiceFeeRules = defaultInvoiceFeeRules
)

type InvoiceFeeRule struct {
	Min    float64 `json:"min"`
	Max    float64 `json:"max,omitempty"`
	Type   string  `json:"type"`
	Value  float64 `json:"value"`
	MaxFee float64 `json:"max_fee,omitempty"`
}

type InvoiceRequest struct {
	Required bool   `json:"required"`
	Type     string `json:"type"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	TaxNo    string `json:"tax_no"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Remark   string `json:"remark"`
}

type InvoiceRecord struct {
	Id                 int                `json:"id"`
	UserId             int                `json:"user_id" gorm:"index"`
	SourceType         string             `json:"source_type" gorm:"type:varchar(32);index;uniqueIndex:idx_invoice_source,priority:1"`
	SourceId           string             `json:"source_id" gorm:"type:varchar(255);index;uniqueIndex:idx_invoice_source,priority:2"`
	PaymentMethod      string             `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider    string             `json:"payment_provider" gorm:"type:varchar(50);default:'';index"`
	PaymentStatus      string             `json:"payment_status" gorm:"type:varchar(32);default:'';index"`
	ProviderOrderId    string             `json:"provider_order_id" gorm:"type:varchar(128);default:'';index"`
	ProviderMerchantId string             `json:"-" gorm:"type:varchar(128);default:''"`
	ProviderAmount     string             `json:"provider_amount" gorm:"type:varchar(64);default:''"`
	ProviderCurrency   string             `json:"provider_currency" gorm:"type:varchar(32);default:''"`
	PaymentAmountMinor int64              `json:"payment_amount_minor" gorm:"default:0"`
	RequestIP          string             `json:"request_ip" gorm:"type:varchar(64);default:''"`
	PaidTime           int64              `json:"paid_time" gorm:"index"`
	ProviderPayload    string             `json:"-" gorm:"type:text"`
	InvoiceType        string             `json:"invoice_type" gorm:"type:varchar(32)"`
	InvoiceKind        string             `json:"invoice_kind" gorm:"type:varchar(32)"`
	Title              string             `json:"title" gorm:"type:varchar(255)"`
	TaxNo              string             `json:"tax_no" gorm:"type:varchar(128)"`
	Email              string             `json:"email" gorm:"type:varchar(255)"`
	Phone              string             `json:"phone" gorm:"type:varchar(64)"`
	Remark             string             `json:"remark" gorm:"type:text"`
	BaseAmount         float64            `json:"base_amount"`
	FeeAmount          float64            `json:"fee_amount"`
	TotalAmount        float64            `json:"total_amount"`
	Status             string             `json:"status" gorm:"type:varchar(32);index"`
	DownloadUrl        string             `json:"download_url" gorm:"type:text"`
	AdminRemark        string             `json:"admin_remark" gorm:"type:text"`
	CreateTime         int64              `json:"create_time" gorm:"index"`
	UpdateTime         int64              `json:"update_time"`
	IssuedTime         int64              `json:"issued_time"`
	Orders             []InvoiceOrderLink `json:"orders,omitempty" gorm:"-"`
}

func InvoiceTypesJSON() string {
	if strings.TrimSpace(InvoiceTypes) == "" {
		return defaultInvoiceTypes
	}
	return InvoiceTypes
}

func InvoiceKindsJSON() string {
	if strings.TrimSpace(InvoiceKinds) == "" {
		return defaultInvoiceKinds
	}
	return InvoiceKinds
}

func InvoiceFeeRulesJSON() string {
	if strings.TrimSpace(InvoiceFeeRules) == "" {
		return defaultInvoiceFeeRules
	}
	return InvoiceFeeRules
}

func UpdateInvoiceTypesByJSONString(value string) error {
	types, err := ParseInvoiceTypes(value)
	if err != nil {
		return err
	}
	data, err := common.Marshal(types)
	if err != nil {
		return err
	}
	InvoiceTypes = string(data)
	return nil
}

func UpdateInvoiceKindsByJSONString(value string) error {
	kinds, err := ParseInvoiceKinds(value)
	if err != nil {
		return err
	}
	data, err := common.Marshal(kinds)
	if err != nil {
		return err
	}
	InvoiceKinds = string(data)
	return nil
}

func UpdateInvoiceFeeRulesByJSONString(value string) error {
	rules, err := ParseInvoiceFeeRules(value)
	if err != nil {
		return err
	}
	data, err := common.Marshal(rules)
	if err != nil {
		return err
	}
	InvoiceFeeRules = string(data)
	return nil
}

func ParseInvoiceTypes(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultInvoiceTypes
	}
	var raw []string
	if err := common.Unmarshal([]byte(value), &raw); err != nil {
		return nil, errors.New("发票类型配置不是有效 JSON")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		typ := normalizeInvoiceType(item)
		if typ == "" {
			return nil, errors.New("发票类型仅支持 personal/company")
		}
		if !seen[typ] {
			seen[typ] = true
			result = append(result, typ)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("至少需要启用一种发票类型")
	}
	return result, nil
}

func ParseInvoiceKinds(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultInvoiceKinds
	}
	var raw []string
	if err := common.Unmarshal([]byte(value), &raw); err != nil {
		return nil, errors.New("开票票种配置不是有效 JSON")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		kind := normalizeInvoiceKind(item)
		if kind == "" {
			return nil, errors.New("开票票种仅支持 normal/special")
		}
		if !seen[kind] {
			seen[kind] = true
			result = append(result, kind)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("至少需要启用一种开票票种")
	}
	return result, nil
}

func ParseInvoiceFeeRules(value string) ([]InvoiceFeeRule, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultInvoiceFeeRules
	}
	var rules []InvoiceFeeRule
	if err := common.Unmarshal([]byte(value), &rules); err != nil {
		return nil, errors.New("发票费用规则不是有效 JSON")
	}
	if len(rules) == 0 {
		return nil, errors.New("发票费用规则不能为空")
	}
	for i := range rules {
		ruleType := strings.ToLower(strings.TrimSpace(rules[i].Type))
		switch ruleType {
		case InvoiceFeeRuleFixed, InvoiceFeeRulePercent:
			rules[i].Type = ruleType
		default:
			return nil, errors.New("发票费用规则 type 仅支持 fixed/percent")
		}
		if rules[i].Min < 0 || rules[i].Max < 0 || rules[i].Value < 0 || rules[i].MaxFee < 0 {
			return nil, errors.New("发票费用规则不能为负数")
		}
		if rules[i].Max > 0 && rules[i].Max < rules[i].Min {
			return nil, errors.New("发票费用规则 max 不能小于 min")
		}
		if ruleType != InvoiceFeeRulePercent {
			rules[i].MaxFee = 0
		}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].Min < rules[j].Min
	})
	return rules, nil
}

func GetInvoiceTypes() []string {
	types, err := ParseInvoiceTypes(InvoiceTypesJSON())
	if err != nil {
		return []string{InvoiceTypePersonal, InvoiceTypeCompany}
	}
	return types
}

func GetInvoiceKinds() []string {
	kinds, err := ParseInvoiceKinds(InvoiceKindsJSON())
	if err != nil {
		return []string{InvoiceKindNormal}
	}
	return kinds
}

func GetInvoiceFeeRules() []InvoiceFeeRule {
	rules, err := ParseInvoiceFeeRules(InvoiceFeeRulesJSON())
	if err != nil {
		rules, _ = ParseInvoiceFeeRules(defaultInvoiceFeeRules)
	}
	return rules
}

func InvoiceConfigSnapshot() map[string]interface{} {
	return map[string]interface{}{
		"enabled":   InvoiceEnabled,
		"types":     GetInvoiceTypes(),
		"kinds":     GetInvoiceKinds(),
		"fee_rules": GetInvoiceFeeRules(),
		"currency":  "CNY",
	}
}

func CalculateInvoiceFee(baseAmountCNY float64) (float64, error) {
	if baseAmountCNY < 0 {
		return 0, errors.New("发票金额不能为负数")
	}
	amount := decimal.NewFromFloat(baseAmountCNY)
	for _, rule := range GetInvoiceFeeRules() {
		min := decimal.NewFromFloat(rule.Min)
		max := decimal.NewFromFloat(rule.Max)
		if amount.LessThan(min) {
			continue
		}
		if rule.Max > 0 && amount.GreaterThan(max) {
			continue
		}
		switch rule.Type {
		case InvoiceFeeRuleFixed:
			return decimal.NewFromFloat(rule.Value).Round(2).InexactFloat64(), nil
		case InvoiceFeeRulePercent:
			fee := amount.Mul(decimal.NewFromFloat(rule.Value)).Div(decimal.NewFromInt(100))
			if rule.MaxFee > 0 {
				maxFee := decimal.NewFromFloat(rule.MaxFee)
				if fee.GreaterThan(maxFee) {
					fee = maxFee
				}
			}
			return fee.Round(2).InexactFloat64(), nil
		}
	}
	return 0, errors.New("未匹配到发票费用规则")
}

func ValidateInvoiceRequest(req InvoiceRequest, baseAmountCNY float64) (InvoiceRequest, float64, error) {
	rawKind := strings.TrimSpace(req.Kind)
	req.Type = normalizeInvoiceType(req.Type)
	req.Kind = normalizeInvoiceKind(req.Kind)
	req.Title = strings.TrimSpace(req.Title)
	req.TaxNo = strings.TrimSpace(req.TaxNo)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Remark = strings.TrimSpace(req.Remark)

	if !req.Required {
		return InvoiceRequest{}, 0, nil
	}
	if !InvoiceEnabled {
		return req, 0, errors.New("当前不支持开发票")
	}
	if req.Type == "" {
		return req, 0, errors.New("请选择发票类型")
	}
	if !InvoiceTypeEnabled(req.Type) {
		return req, 0, errors.New("当前不支持该发票类型")
	}
	if rawKind != "" && req.Kind == "" {
		return req, 0, errors.New("开票票种仅支持 normal/special")
	}
	if req.Kind == "" {
		kinds := GetInvoiceKinds()
		if len(kinds) > 0 {
			req.Kind = kinds[0]
		}
	}
	if req.Kind == "" {
		return req, 0, errors.New("请选择开票票种")
	}
	if !InvoiceKindEnabled(req.Kind) {
		return req, 0, errors.New("当前不支持该开票票种")
	}
	if req.Title == "" {
		return req, 0, errors.New("请填写发票抬头")
	}
	if req.Type == InvoiceTypeCompany && req.TaxNo == "" {
		return req, 0, errors.New("请填写纳税人识别号")
	}
	fee, err := CalculateInvoiceFee(baseAmountCNY)
	if err != nil {
		return req, 0, err
	}
	return req, fee, nil
}

func ValidateInvoicePreviewRequest(req InvoiceRequest, baseAmountCNY float64) (InvoiceRequest, float64, error) {
	rawKind := strings.TrimSpace(req.Kind)
	req.Type = normalizeInvoiceType(req.Type)
	req.Kind = normalizeInvoiceKind(req.Kind)
	req.Title = strings.TrimSpace(req.Title)
	req.TaxNo = strings.TrimSpace(req.TaxNo)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Remark = strings.TrimSpace(req.Remark)

	if !req.Required {
		return InvoiceRequest{}, 0, nil
	}
	if !InvoiceEnabled {
		return req, 0, errors.New("当前不支持开发票")
	}
	if req.Type == "" {
		types := GetInvoiceTypes()
		if len(types) > 0 {
			req.Type = types[0]
		}
	}
	if req.Type == "" {
		return req, 0, errors.New("请选择发票类型")
	}
	if !InvoiceTypeEnabled(req.Type) {
		return req, 0, errors.New("当前不支持该发票类型")
	}
	if rawKind != "" && req.Kind == "" {
		return req, 0, errors.New("开票票种仅支持 normal/special")
	}
	if req.Kind == "" {
		kinds := GetInvoiceKinds()
		if len(kinds) > 0 {
			req.Kind = kinds[0]
		}
	}
	if req.Kind == "" {
		return req, 0, errors.New("请选择开票票种")
	}
	if !InvoiceKindEnabled(req.Kind) {
		return req, 0, errors.New("当前不支持该开票票种")
	}
	fee, err := CalculateInvoiceFee(baseAmountCNY)
	if err != nil {
		return req, 0, err
	}
	return req, fee, nil
}

func InvoiceTypeEnabled(invoiceType string) bool {
	invoiceType = normalizeInvoiceType(invoiceType)
	for _, typ := range GetInvoiceTypes() {
		if typ == invoiceType {
			return true
		}
	}
	return false
}

func InvoiceKindEnabled(invoiceKind string) bool {
	invoiceKind = normalizeInvoiceKind(invoiceKind)
	for _, kind := range GetInvoiceKinds() {
		if kind == invoiceKind {
			return true
		}
	}
	return false
}

func normalizeInvoiceType(invoiceType string) string {
	switch strings.ToLower(strings.TrimSpace(invoiceType)) {
	case InvoiceTypePersonal:
		return InvoiceTypePersonal
	case InvoiceTypeCompany:
		return InvoiceTypeCompany
	default:
		return ""
	}
}

func normalizeInvoiceKind(invoiceKind string) string {
	switch strings.ToLower(strings.TrimSpace(invoiceKind)) {
	case InvoiceKindNormal:
		return InvoiceKindNormal
	case InvoiceKindSpecial:
		return InvoiceKindSpecial
	default:
		return ""
	}
}

func AmountCNYToPaymentCurrency(amountCNY float64, paymentProvider string) float64 {
	amount := decimal.NewFromFloat(amountCNY)
	switch strings.TrimSpace(paymentProvider) {
	case PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake, PaymentProviderBalance:
		if operation_setting.Price <= 0 {
			return 0
		}
		return amount.Div(decimal.NewFromFloat(operation_setting.Price)).Round(2).InexactFloat64()
	default:
		return amount.Round(2).InexactFloat64()
	}
}

func PaymentAmountToCNY(amount float64, paymentProvider string) float64 {
	value := decimal.NewFromFloat(amount)
	switch strings.TrimSpace(paymentProvider) {
	case PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake, PaymentProviderBalance:
		return value.Mul(decimal.NewFromFloat(operation_setting.Price)).Round(2).InexactFloat64()
	default:
		return value.Round(2).InexactFloat64()
	}
}

func AddInvoiceSnapshotToTopUp(topUp *TopUp, req InvoiceRequest, baseAmountCNY float64, feeCNY float64) {
	if topUp == nil || !req.Required {
		return
	}
	topUp.InvoiceRequired = true
	topUp.InvoiceType = req.Type
	topUp.InvoiceKind = req.Kind
	topUp.InvoiceTitle = req.Title
	topUp.InvoiceTaxNo = req.TaxNo
	topUp.InvoiceEmail = req.Email
	topUp.InvoicePhone = req.Phone
	topUp.InvoiceRemark = req.Remark
	topUp.InvoiceBaseAmount = decimal.NewFromFloat(baseAmountCNY).Round(2).InexactFloat64()
	topUp.InvoiceFeeAmount = decimal.NewFromFloat(feeCNY).Round(2).InexactFloat64()
	topUp.InvoiceStatus = InvoiceStatusPending
}

func AddInvoiceSnapshotToSubscriptionOrder(order *SubscriptionOrder, req InvoiceRequest, baseAmountCNY float64, feeCNY float64) {
	if order == nil || !req.Required {
		return
	}
	order.InvoiceRequired = true
	order.InvoiceType = req.Type
	order.InvoiceKind = req.Kind
	order.InvoiceTitle = req.Title
	order.InvoiceTaxNo = req.TaxNo
	order.InvoiceEmail = req.Email
	order.InvoicePhone = req.Phone
	order.InvoiceRemark = req.Remark
	order.InvoiceBaseAmount = decimal.NewFromFloat(baseAmountCNY).Round(2).InexactFloat64()
	order.InvoiceFeeAmount = decimal.NewFromFloat(feeCNY).Round(2).InexactFloat64()
	order.InvoiceStatus = InvoiceStatusPending
}

func CreateInvoiceRecordFromTopUpTx(tx *gorm.DB, topUp *TopUp) error {
	if tx == nil || topUp == nil || !topUp.InvoiceRequired {
		return nil
	}
	record := &InvoiceRecord{
		UserId:             topUp.UserId,
		SourceType:         InvoiceSourceTopUp,
		SourceId:           topUp.TradeNo,
		PaymentMethod:      topUp.PaymentMethod,
		PaymentProvider:    invoiceOrderPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod),
		PaymentStatus:      InvoicePaymentStatusSuccess,
		ProviderOrderId:    topUp.ProviderOrderId,
		ProviderAmount:     topUp.ProviderAmount,
		ProviderCurrency:   topUp.ProviderCurrency,
		PaymentAmountMinor: invoiceCNYToMinor(topUp.InvoiceFeeAmount),
		RequestIP:          topUp.RequestIP,
		PaidTime:           invoiceOrderCompleteTime(topUp.CompleteTime, topUp.CreateTime),
		InvoiceType:        topUp.InvoiceType,
		InvoiceKind:        topUp.InvoiceKind,
		Title:              topUp.InvoiceTitle,
		TaxNo:              topUp.InvoiceTaxNo,
		Email:              topUp.InvoiceEmail,
		Phone:              topUp.InvoicePhone,
		Remark:             topUp.InvoiceRemark,
		BaseAmount:         topUp.InvoiceBaseAmount,
		FeeAmount:          topUp.InvoiceFeeAmount,
		TotalAmount:        decimal.NewFromFloat(topUp.InvoiceBaseAmount).Add(decimal.NewFromFloat(topUp.InvoiceFeeAmount)).Round(2).InexactFloat64(),
		Status:             InvoiceStatusPending,
		CreateTime:         common.GetTimestamp(),
		UpdateTime:         common.GetTimestamp(),
	}
	if err := tx.Where("source_type = ? AND source_id = ?", record.SourceType, record.SourceId).First(&InvoiceRecord{}).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := tx.Create(record).Error; err != nil {
		return err
	}
	topUp.InvoiceStatus = InvoiceStatusPending
	return tx.Save(topUp).Error
}

func CreateInvoiceRecordFromSubscriptionOrderTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if tx == nil || order == nil || !order.InvoiceRequired {
		return nil
	}
	record := &InvoiceRecord{
		UserId:             order.UserId,
		SourceType:         InvoiceSourceSubscription,
		SourceId:           order.TradeNo,
		PaymentMethod:      order.PaymentMethod,
		PaymentProvider:    invoiceOrderPaymentProvider(order.PaymentProvider, order.PaymentMethod),
		PaymentStatus:      InvoicePaymentStatusSuccess,
		ProviderOrderId:    order.ProviderOrderId,
		ProviderAmount:     order.ProviderAmount,
		ProviderCurrency:   order.ProviderCurrency,
		PaymentAmountMinor: invoiceCNYToMinor(order.InvoiceFeeAmount),
		RequestIP:          order.RequestIP,
		PaidTime:           invoiceOrderCompleteTime(order.CompleteTime, order.CreateTime),
		ProviderPayload:    order.ProviderPayload,
		InvoiceType:        order.InvoiceType,
		InvoiceKind:        order.InvoiceKind,
		Title:              order.InvoiceTitle,
		TaxNo:              order.InvoiceTaxNo,
		Email:              order.InvoiceEmail,
		Phone:              order.InvoicePhone,
		Remark:             order.InvoiceRemark,
		BaseAmount:         order.InvoiceBaseAmount,
		FeeAmount:          order.InvoiceFeeAmount,
		TotalAmount:        decimal.NewFromFloat(order.InvoiceBaseAmount).Add(decimal.NewFromFloat(order.InvoiceFeeAmount)).Round(2).InexactFloat64(),
		Status:             InvoiceStatusPending,
		CreateTime:         common.GetTimestamp(),
		UpdateTime:         common.GetTimestamp(),
	}
	if err := tx.Where("source_type = ? AND source_id = ?", record.SourceType, record.SourceId).First(&InvoiceRecord{}).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := tx.Create(record).Error; err != nil {
		return err
	}
	order.InvoiceStatus = InvoiceStatusPending
	return tx.Save(order).Error
}

func GetUserInvoiceRecords(userId int, pageInfo *common.PageInfo) ([]*InvoiceRecord, int64, error) {
	var records []*InvoiceRecord
	var total int64
	if userId <= 0 {
		return records, 0, nil
	}
	tx := DB.Model(&InvoiceRecord{}).Where("user_id = ?", userId)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	if err := attachInvoiceOrderLinks(records); err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func GetAllInvoiceRecords(status string, pageInfo *common.PageInfo) ([]*InvoiceRecord, int64, error) {
	var records []*InvoiceRecord
	var total int64
	tx := DB.Model(&InvoiceRecord{})
	if strings.TrimSpace(status) != "" {
		tx = tx.Where("status = ?", strings.TrimSpace(status))
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	if err := attachInvoiceOrderLinks(records); err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func attachInvoiceOrderLinks(records []*InvoiceRecord) error {
	if len(records) == 0 {
		return nil
	}
	ids := make([]int, 0, len(records))
	byId := make(map[int]*InvoiceRecord, len(records))
	for _, record := range records {
		if record == nil || record.Id <= 0 {
			continue
		}
		ids = append(ids, record.Id)
		byId[record.Id] = record
	}
	if len(ids) == 0 {
		return nil
	}
	var links []InvoiceOrderLink
	if err := DB.Where("invoice_id IN ?", ids).Order("id asc").Find(&links).Error; err != nil {
		return err
	}
	for _, link := range links {
		if record := byId[link.InvoiceId]; record != nil {
			record.Orders = append(record.Orders, link)
		}
	}
	return nil
}

func UpdateInvoiceRecord(id int, downloadUrl string, status string, adminRemark string) error {
	if id <= 0 {
		return errors.New("invalid invoice id")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = InvoiceStatusIssued
	}
	if status != InvoiceStatusPending && status != InvoiceStatusIssued && status != InvoiceStatusClosed {
		return errors.New("无效的发票状态")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var record InvoiceRecord
		if err := lockForUpdate(tx).Where("id = ?", id).First(&record).Error; err != nil {
			return err
		}
		if record.PaymentStatus != "" && record.PaymentStatus != InvoicePaymentStatusSuccess {
			if status != InvoiceStatusClosed {
				return errors.New("发票服务费尚未支付，只能关闭该申请")
			}
			record.AdminRemark = strings.TrimSpace(adminRemark)
			return cancelInvoiceExternalPaymentTx(tx, &record)
		}
		record.DownloadUrl = strings.TrimSpace(downloadUrl)
		record.Status = status
		record.AdminRemark = strings.TrimSpace(adminRemark)
		record.UpdateTime = common.GetTimestamp()
		if status == InvoiceStatusIssued && record.IssuedTime == 0 {
			record.IssuedTime = common.GetTimestamp()
		}
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		return syncInvoiceSourceStatusTx(tx, &record)
	})
}

func syncInvoiceSourceStatusTx(tx *gorm.DB, record *InvoiceRecord) error {
	if tx == nil || record == nil {
		return nil
	}
	updates := map[string]interface{}{
		"invoice_status": record.Status,
	}
	switch record.SourceType {
	case InvoiceSourceTopUp:
		return tx.Model(&TopUp{}).Where("trade_no = ?", record.SourceId).Updates(updates).Error
	case InvoiceSourceSubscription:
		return tx.Model(&SubscriptionOrder{}).Where("trade_no = ?", record.SourceId).Updates(updates).Error
	case InvoiceSourceCombined:
		var links []InvoiceOrderLink
		if err := tx.Where("invoice_id = ?", record.Id).Find(&links).Error; err != nil {
			return err
		}
		for _, link := range links {
			switch link.SourceType {
			case InvoiceSourceTopUp:
				if err := tx.Model(&TopUp{}).Where("trade_no = ?", link.SourceId).Updates(updates).Error; err != nil {
					return err
				}
			case InvoiceSourceSubscription:
				if err := tx.Model(&SubscriptionOrder{}).Where("trade_no = ?", link.SourceId).Updates(updates).Error; err != nil {
					return err
				}
				// 订阅支付可能存在同订单号的充值镜像。
				if err := tx.Model(&TopUp{}).Where("trade_no = ?", link.SourceId).Updates(updates).Error; err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown invoice order source type %s", link.SourceType)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown invoice source type %s", record.SourceType)
	}
}
