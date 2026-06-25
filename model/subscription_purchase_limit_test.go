package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func insertUserForSubscriptionLimitTest(t *testing.T, id int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "subscription_limit_user",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForLimitTest(t *testing.T, id int, maxPurchasePerUser int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:                 id,
		Title:              "Limit Plan",
		PriceAmount:        9.99,
		Currency:           "USD",
		DurationUnit:       SubscriptionDurationMonth,
		DurationValue:      1,
		Enabled:            true,
		MaxPurchasePerUser: maxPurchasePerUser,
		TotalAmount:        1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertUserSubscriptionForLimitTest(t *testing.T, userId int, planId int, status string, startTime int64, endTime int64) {
	t.Helper()
	sub := &UserSubscription{
		UserId:      userId,
		PlanId:      planId,
		AmountTotal: 1000,
		AmountUsed:  0,
		StartTime:   startTime,
		EndTime:     endTime,
		Status:      status,
		Source:      "test",
	}
	require.NoError(t, DB.Create(sub).Error)
}

func createSubscriptionFromPlanForLimitTest(t *testing.T, userId int, plan *SubscriptionPlan) error {
	t.Helper()
	return DB.Transaction(func(tx *gorm.DB) error {
		_, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "test")
		return err
	})
}

func countUserSubscriptionsByPlanForLimitTest(t *testing.T, userId int, planId int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", userId, planId).
		Count(&count).Error)
	return count
}

func TestCreateUserSubscriptionFromPlanTx_MaxPurchasePerUserCountsOnlyActiveUnexpired(t *testing.T) {
	testCases := []struct {
		name          string
		status        string
		endTimeOffset time.Duration
		expectAllowed bool
	}{
		{
			name:          "expired active subscription ignored",
			status:        "active",
			endTimeOffset: -time.Hour,
			expectAllowed: true,
		},
		{
			name:          "cancelled subscription ignored",
			status:        "cancelled",
			endTimeOffset: time.Hour,
			expectAllowed: true,
		},
		{
			name:          "active unexpired subscription counted",
			status:        "active",
			endTimeOffset: time.Hour,
			expectAllowed: false,
		},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			userId := 6100 + index
			planId := 7100 + index
			now := common.GetTimestamp()
			insertUserForSubscriptionLimitTest(t, userId)
			plan := insertSubscriptionPlanForLimitTest(t, planId, 1)
			insertUserSubscriptionForLimitTest(t, userId, planId, tc.status, now-3600, time.Now().Add(tc.endTimeOffset).Unix())

			err := createSubscriptionFromPlanForLimitTest(t, userId, plan)

			if tc.expectAllowed {
				require.NoError(t, err)
				assert.EqualValues(t, 2, countUserSubscriptionsByPlanForLimitTest(t, userId, planId))
			} else {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), "购买上限"))
				assert.EqualValues(t, 1, countUserSubscriptionsByPlanForLimitTest(t, userId, planId))
			}
		})
	}
}

func TestCreateUserSubscriptionFromPlanTx_MaxPurchasePerUserAllowsDifferentPlansAndUnlimited(t *testing.T) {
	truncateTables(t)
	userId := 6200
	now := common.GetTimestamp()
	insertUserForSubscriptionLimitTest(t, userId)
	limitedPlan := insertSubscriptionPlanForLimitTest(t, 7200, 1)
	otherPlan := insertSubscriptionPlanForLimitTest(t, 7201, 1)
	unlimitedPlan := insertSubscriptionPlanForLimitTest(t, 7202, 0)
	insertUserSubscriptionForLimitTest(t, userId, otherPlan.Id, "active", now-3600, now+3600)
	insertUserSubscriptionForLimitTest(t, userId, unlimitedPlan.Id, "active", now-3600, now+3600)

	require.NoError(t, createSubscriptionFromPlanForLimitTest(t, userId, limitedPlan))
	require.NoError(t, createSubscriptionFromPlanForLimitTest(t, userId, unlimitedPlan))
	assert.EqualValues(t, 1, countUserSubscriptionsByPlanForLimitTest(t, userId, limitedPlan.Id))
	assert.EqualValues(t, 2, countUserSubscriptionsByPlanForLimitTest(t, userId, unlimitedPlan.Id))
}
