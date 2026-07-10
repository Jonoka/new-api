package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func insertSubscriptionExpireTestUser(t *testing.T, id int, group string) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:       id,
		Username: "subscription_expire_user",
		Group:    group,
		Status:   common.UserStatusEnabled,
	}).Error)
}

func getSubscriptionExpireTestUserGroup(t *testing.T, id int) string {
	t.Helper()
	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", id).Select(commonGroupCol).Find(&group).Error)
	return group
}

func TestExpireDueSubscriptionsFallsBackThroughRenewedUpgradeGroup(t *testing.T) {
	truncateTables(t)

	userId := 7001
	now := GetDBTimestamp()
	insertSubscriptionExpireTestUser(t, userId, "vip")
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:        userId,
		PlanId:        1,
		AmountTotal:   100,
		StartTime:     now - 300,
		EndTime:       now - 200,
		Status:        "active",
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:       userId,
		PlanId:       2,
		AmountTotal:  100,
		StartTime:    now - 200,
		EndTime:      now - 100,
		Status:       "active",
		UpgradeGroup: "vip",
	}).Error)

	count, err := ExpireDueSubscriptions(20)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, "default", getSubscriptionExpireTestUserGroup(t, userId))
}

func TestGetUserCacheExpiresDueSubscriptionAndDowngradesGroup(t *testing.T) {
	truncateTables(t)

	userId := 7002
	now := GetDBTimestamp()
	insertSubscriptionExpireTestUser(t, userId, "vip")
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:        userId,
		PlanId:        1,
		AmountTotal:   100,
		StartTime:     now - 200,
		EndTime:       now - 100,
		Status:        "active",
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}).Error)

	userCache, err := GetUserCache(userId)
	require.NoError(t, err)
	require.Equal(t, "default", userCache.Group)
	require.Equal(t, "default", getSubscriptionExpireTestUserGroup(t, userId))

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ?", userId).First(&sub).Error)
	require.Equal(t, "expired", sub.Status)
}

func TestAdminDeleteActiveSubscriptionDowngradesGroup(t *testing.T) {
	truncateTables(t)

	userId := 7003
	now := GetDBTimestamp()
	insertSubscriptionExpireTestUser(t, userId, "vip")
	sub := UserSubscription{
		UserId:        userId,
		PlanId:        1,
		AmountTotal:   100,
		StartTime:     now - 100,
		EndTime:       now + 3600,
		Status:        "active",
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}
	require.NoError(t, DB.Create(&sub).Error)

	message, err := AdminDeleteUserSubscription(sub.Id)

	require.NoError(t, err)
	require.Contains(t, message, "default")
	require.Equal(t, "default", getSubscriptionExpireTestUserGroup(t, userId))
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", sub.Id).Count(&count).Error)
	require.Zero(t, count)
}
