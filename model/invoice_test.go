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
