package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type invoicePaymentAmounts struct {
	Request          model.InvoiceRequest
	Required         bool
	BaseCNY          float64
	FeeCNY           float64
	TotalCNY         float64
	FeePaymentAmount float64
	TotalPayment     float64
}

func roundPaymentAmount(amount float64) float64 {
	return decimal.NewFromFloat(amount).Round(2).InexactFloat64()
}

func buildInvoicePaymentAmounts(req model.InvoiceRequest, paymentProvider string, businessPaymentAmount float64) (*invoicePaymentAmounts, error) {
	return buildInvoicePaymentAmountsWithValidator(req, paymentProvider, businessPaymentAmount, model.ValidateInvoiceRequest)
}

func buildInvoicePaymentPreviewAmounts(req model.InvoiceRequest, paymentProvider string, businessPaymentAmount float64) (*invoicePaymentAmounts, error) {
	return buildInvoicePaymentAmountsWithValidator(req, paymentProvider, businessPaymentAmount, model.ValidateInvoicePreviewRequest)
}

func buildInvoicePaymentAmountsWithValidator(req model.InvoiceRequest, paymentProvider string, businessPaymentAmount float64, validate func(model.InvoiceRequest, float64) (model.InvoiceRequest, float64, error)) (*invoicePaymentAmounts, error) {
	baseCNY := model.PaymentAmountToCNY(businessPaymentAmount, paymentProvider)
	normalizedReq, feeCNY, err := validate(req, baseCNY)
	if err != nil {
		return nil, err
	}
	feePaymentAmount := model.AmountCNYToPaymentCurrency(feeCNY, paymentProvider)
	totalPayment := decimal.NewFromFloat(businessPaymentAmount).Add(decimal.NewFromFloat(feePaymentAmount)).Round(2).InexactFloat64()
	totalCNY := decimal.NewFromFloat(baseCNY).Add(decimal.NewFromFloat(feeCNY)).Round(2).InexactFloat64()
	return &invoicePaymentAmounts{
		Request:          normalizedReq,
		Required:         normalizedReq.Required,
		BaseCNY:          baseCNY,
		FeeCNY:           feeCNY,
		TotalCNY:         totalCNY,
		FeePaymentAmount: feePaymentAmount,
		TotalPayment:     totalPayment,
	}, nil
}

func paymentProviderFromSubscriptionPaymentMethod(paymentMethod string) string {
	switch paymentMethod {
	case model.PaymentMethodStripe:
		return model.PaymentProviderStripe
	case model.PaymentMethodCreem:
		return model.PaymentProviderCreem
	case model.PaymentMethodWaffoPancake:
		return model.PaymentProviderWaffoPancake
	case model.PaymentMethodBepusdt:
		return model.PaymentProviderBepusdt
	case model.PaymentMethodOkpay:
		return model.PaymentProviderOkpay
	case model.PaymentMethodBalance:
		return model.PaymentProviderBalance
	default:
		if paymentMethod == "" {
			return ""
		}
		return model.PaymentProviderEpay
	}
}

func invoicePaymentProviderForDisplay(paymentMethod string, displayCurrency string) string {
	paymentProvider := paymentProviderFromSubscriptionPaymentMethod(paymentMethod)
	if paymentProvider != "" {
		return paymentProvider
	}
	if displayCurrency == model.SubscriptionCurrencyUSD {
		return model.PaymentProviderStripe
	}
	return model.PaymentProviderEpay
}

func addInvoiceFieldsToResponse(response gin.H, amounts *invoicePaymentAmounts) {
	if response == nil || amounts == nil {
		return
	}
	response["invoice_required"] = amounts.Required
	response["invoice_kind"] = amounts.Request.Kind
	response["invoice_base_amount"] = amounts.BaseCNY
	response["invoice_fee"] = amounts.FeeCNY
	response["invoice_total_amount"] = amounts.TotalCNY
	response["invoice_fee_payment_amount"] = amounts.FeePaymentAmount
	response["invoice_payment_amount"] = amounts.TotalPayment
	response["invoice_currency"] = "CNY"
}

func applyInvoiceToTopUp(topUp *model.TopUp, amounts *invoicePaymentAmounts, businessOriginalAmount float64, businessPaidAmount float64, includeFeeInOrderMoney bool) {
	if topUp == nil {
		return
	}
	if topUp.OriginalMoney == 0 {
		topUp.OriginalMoney = roundPaymentAmount(businessOriginalAmount)
	}
	if topUp.ActualMoney == 0 && businessPaidAmount > 0 {
		topUp.ActualMoney = roundPaymentAmount(businessPaidAmount)
	}
	if amounts == nil || !amounts.Required {
		return
	}
	model.AddInvoiceSnapshotToTopUp(topUp, amounts.Request, amounts.BaseCNY, amounts.FeeCNY)
	if includeFeeInOrderMoney {
		topUp.Money = amounts.TotalPayment
	}
}

func applyInvoiceToSubscriptionOrder(order *model.SubscriptionOrder, amounts *invoicePaymentAmounts, businessOriginalAmount float64, businessPaidAmount float64, affiliateSourceQuota int) {
	if order == nil {
		return
	}
	if order.OriginalMoney == 0 {
		order.OriginalMoney = roundPaymentAmount(businessOriginalAmount)
	}
	if order.ActualMoney == 0 && businessPaidAmount > 0 {
		order.ActualMoney = roundPaymentAmount(businessPaidAmount)
	}
	if order.AffiliateSourceQuota <= 0 && affiliateSourceQuota > 0 {
		order.AffiliateSourceQuota = affiliateSourceQuota
	}
	if amounts == nil || !amounts.Required {
		return
	}
	model.AddInvoiceSnapshotToSubscriptionOrder(order, amounts.Request, amounts.BaseCNY, amounts.FeeCNY)
	order.Money = amounts.TotalPayment
}
