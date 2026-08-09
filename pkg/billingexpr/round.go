package billingexpr

import "github.com/QuantumNous/new-api/common"

// QuotaRound converts a float64 quota value to int using half-away-from-zero
// rounding with int32 saturation. Every tiered billing path must use this
// function to avoid +-1 discrepancies.
func QuotaRound(f float64) int {
	return common.QuotaRound(f)
}

// QuotaRoundStrict 拒绝无法安全写入额度列的预扣估算值。
func QuotaRoundStrict(f float64) (int, error) {
	return common.QuotaRoundStrict(f)
}
