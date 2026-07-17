package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateInvoiceFeeAppliesPercentMaxFee(t *testing.T) {
	originalInvoiceFeeRules := InvoiceFeeRules
	InvoiceFeeRules = `[{"min":0,"type":"percent","value":4,"max_fee":150}]`
	defer func() {
		InvoiceFeeRules = originalInvoiceFeeRules
	}()

	fee, err := CalculateInvoiceFee(5000)

	require.NoError(t, err)
	assert.Equal(t, 150.0, fee)
}

func TestCalculateInvoiceFeeKeepsPercentFeeBelowMaxFee(t *testing.T) {
	originalInvoiceFeeRules := InvoiceFeeRules
	InvoiceFeeRules = `[{"min":0,"type":"percent","value":4,"max_fee":150}]`
	defer func() {
		InvoiceFeeRules = originalInvoiceFeeRules
	}()

	fee, err := CalculateInvoiceFee(1000)

	require.NoError(t, err)
	assert.Equal(t, 40.0, fee)
}

func TestParseInvoiceFeeRulesClearsMaxFeeForFixedRules(t *testing.T) {
	rules, err := ParseInvoiceFeeRules(`[{"min":0,"max":500,"type":"fixed","value":60,"max_fee":10}]`)

	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, 0.0, rules[0].MaxFee)
}

func TestParseInvoiceKindsNormalizesAndDeduplicates(t *testing.T) {
	kinds, err := ParseInvoiceKinds(`[" normal ","SPECIAL","normal"]`)

	require.NoError(t, err)
	assert.Equal(t, []string{InvoiceKindNormal, InvoiceKindSpecial}, kinds)
}

func TestParseInvoiceKindsRejectsUnsupportedKind(t *testing.T) {
	_, err := ParseInvoiceKinds(`["electronic"]`)

	require.ErrorContains(t, err, "normal/special")
}

func TestValidateOptionValueRejectsInvalidInvoiceConfiguration(t *testing.T) {
	require.NoError(t, validateOptionValue("InvoiceTypes", `["personal","company"]`))
	require.NoError(t, validateOptionValue("InvoiceKinds", `["normal","special"]`))
	require.NoError(t, validateOptionValue("InvoiceFeeRules", `[{"min":0,"type":"fixed","value":0}]`))

	require.Error(t, validateOptionValue("InvoiceTypes", `[]`))
	require.Error(t, validateOptionValue("InvoiceKinds", `[]`))
	require.Error(t, validateOptionValue("InvoiceKinds", `["electronic"]`))
	require.Error(t, validateOptionValue("InvoiceFeeRules", `[]`))
}

func TestValidateInvoiceRequestDefaultsToConfiguredInvoiceKind(t *testing.T) {
	originalEnabled := InvoiceEnabled
	originalTypes := InvoiceTypes
	originalKinds := InvoiceKinds
	originalRules := InvoiceFeeRules
	InvoiceEnabled = true
	InvoiceTypes = `["company"]`
	InvoiceKinds = `["normal"]`
	InvoiceFeeRules = `[{"min":0,"type":"fixed","value":0}]`
	t.Cleanup(func() {
		InvoiceEnabled = originalEnabled
		InvoiceTypes = originalTypes
		InvoiceKinds = originalKinds
		InvoiceFeeRules = originalRules
	})

	req, _, err := ValidateInvoiceRequest(InvoiceRequest{
		Required: true,
		Type:     InvoiceTypeCompany,
		Title:    "测试企业",
		TaxNo:    "91310000TEST",
	}, 100)

	require.NoError(t, err)
	assert.Equal(t, InvoiceKindNormal, req.Kind)
}

func TestValidateInvoiceRequestRejectsDisabledInvoiceKind(t *testing.T) {
	originalEnabled := InvoiceEnabled
	originalTypes := InvoiceTypes
	originalKinds := InvoiceKinds
	originalRules := InvoiceFeeRules
	InvoiceEnabled = true
	InvoiceTypes = `["company"]`
	InvoiceKinds = `["normal"]`
	InvoiceFeeRules = `[{"min":0,"type":"fixed","value":0}]`
	t.Cleanup(func() {
		InvoiceEnabled = originalEnabled
		InvoiceTypes = originalTypes
		InvoiceKinds = originalKinds
		InvoiceFeeRules = originalRules
	})

	_, _, err := ValidateInvoiceRequest(InvoiceRequest{
		Required: true,
		Type:     InvoiceTypeCompany,
		Kind:     InvoiceKindSpecial,
		Title:    "测试企业",
		TaxNo:    "91310000TEST",
	}, 100)

	require.ErrorContains(t, err, "当前不支持该开票票种")
}

func TestValidateInvoiceRequestRejectsUnknownInvoiceKind(t *testing.T) {
	originalEnabled := InvoiceEnabled
	originalTypes := InvoiceTypes
	originalKinds := InvoiceKinds
	InvoiceEnabled = true
	InvoiceTypes = `["personal"]`
	InvoiceKinds = `["normal"]`
	t.Cleanup(func() {
		InvoiceEnabled = originalEnabled
		InvoiceTypes = originalTypes
		InvoiceKinds = originalKinds
	})

	_, _, err := ValidateInvoiceRequest(InvoiceRequest{
		Required: true,
		Type:     InvoiceTypePersonal,
		Kind:     "electronic",
		Title:    "测试个人",
	}, 100)

	require.ErrorContains(t, err, "normal/special")
}
