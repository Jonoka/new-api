package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalcNextResetTime_RollsFromSubscriptionStart(t *testing.T) {
	base := time.Date(2026, 6, 7, 23, 30, 0, 0, time.UTC) // Sunday night
	endUnix := base.AddDate(0, 2, 0).Unix()

	testCases := []struct {
		name     string
		plan     SubscriptionPlan
		expected time.Time
	}{
		{
			name: "daily rolls 24 hours from purchase time",
			plan: SubscriptionPlan{
				QuotaResetPeriod: SubscriptionResetDaily,
			},
			expected: base.AddDate(0, 0, 1),
		},
		{
			name: "weekly rolls 7 days from purchase time",
			plan: SubscriptionPlan{
				QuotaResetPeriod: SubscriptionResetWeekly,
			},
			expected: base.AddDate(0, 0, 7),
		},
		{
			name: "monthly rolls one month from purchase time",
			plan: SubscriptionPlan{
				QuotaResetPeriod: SubscriptionResetMonthly,
			},
			expected: base.AddDate(0, 1, 0),
		},
		{
			name: "custom keeps second-based rolling reset",
			plan: SubscriptionPlan{
				QuotaResetPeriod:        SubscriptionResetCustom,
				QuotaResetCustomSeconds: 3600,
			},
			expected: base.Add(time.Hour),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := calcNextResetTime(base, &tc.plan, endUnix)

			assert.Equal(t, tc.expected.Unix(), actual)
		})
	}
}

func TestCalcNextResetTime_DoesNotSchedulePastSubscriptionEnd(t *testing.T) {
	base := time.Date(2026, 6, 7, 23, 30, 0, 0, time.UTC)
	plan := &SubscriptionPlan{QuotaResetPeriod: SubscriptionResetWeekly}
	endBeforeNextRollingReset := base.AddDate(0, 0, 6).Unix()

	actual := calcNextResetTime(base, plan, endBeforeNextRollingReset)

	assert.EqualValues(t, 0, actual)
}
