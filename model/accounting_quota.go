package model

import (
	"errors"
	"math"
)

// Account balances may exceed the per-request pricing limit. Check native
// integer overflow before arithmetic without imposing a new balance ceiling.
func checkedAccountingQuota(base, increase, decrease int) (int, error) {
	if (increase > 0 && base > math.MaxInt-increase) || (increase < 0 && base < math.MinInt-increase) {
		return 0, errors.New("accounting quota overflow")
	}
	value := base + increase
	if (decrease > 0 && value < math.MinInt+decrease) || (decrease < 0 && value > math.MaxInt+decrease) {
		return 0, errors.New("accounting quota overflow")
	}
	return value - decrease, nil
}
