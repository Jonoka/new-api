package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReconcileGroupReservationResetPreservesOtherCurrentPeriodUsage(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			suffix := fmt.Sprintf("epoch-%d", time.Now().UnixNano())
			user, token := seedGroupReservationWallet(t, db, suffix, 2000, 2000)
			plan := &SubscriptionPlan{Title: suffix, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 2000, QuotaResetPeriod: SubscriptionResetNever}
			require.NoError(t, db.Create(plan).Error)
			now := time.Now().Unix()
			sub := &UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 2000, StartTime: now - 100, EndTime: now + 3600, Status: "active", LastResetTime: now - 100}
			require.NoError(t, db.Create(sub).Error)
			req := GroupReservationRequest{Source: GroupReservationSubscription, RequestId: suffix, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TargetReserved: 300}
			result, err := ReconcileGroupReservation(req)
			require.NoError(t, err)
			req.SubscriptionId = result.SubscriptionId
			// Maintenance reset and another request's 600 units happen before the
			// live request retries. UpdatedAt seconds and total usage cannot identify
			// which period still contains this request's original 300 units.
			require.NoError(t, db.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(map[string]any{"last_reset_time": now - 1, "amount_used": 600}).Error)
			req.ExpectedReserved, req.TargetReserved = 300, 500
			_, err = ReconcileGroupReservation(req)
			require.NoError(t, err)
			require.NoError(t, db.First(sub, sub.Id).Error)
			require.EqualValues(t, 1100, sub.AmountUsed)
			req.ExpectedReserved, req.TargetReserved = 500, 0
			_, err = ReconcileGroupReservation(req)
			require.NoError(t, err)
			require.NoError(t, db.First(sub, sub.Id).Error)
			require.EqualValues(t, 600, sub.AmountUsed)
			_, remain, used := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 2000, remain)
			require.Zero(t, used)
		})
	}
}
