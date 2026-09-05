package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

// Seed only the disposable Actions database used by the immutable-image rehearsal.
func TestAlphaSearchCompatibilitySeed(t *testing.T) {
	db := taskAccountingCompatibilityDatabase(t)
	user := &User{Username: "c-alpha-user", Password: "ci-only", Quota: 100000,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "c-alpha-user"}
	user.SetSetting(dto.UserSetting{BillingPreference: "wallet_only", QuotaWarningThreshold: 1})
	require.NoError(t, db.Create(user).Error)
	token := &Token{UserId: user.Id, Key: "calphasynthetictoken", Name: "c-alpha-token",
		Status: common.TokenStatusEnabled, RemainQuota: 100000, Group: "default",
		ModelLimitsEnabled: true, ModelLimits: "c-alpha-public"}
	require.NoError(t, token.Insert())
	channel := &Channel{Name: "c-alpha-channel", Type: constant.ChannelTypeOpenAI,
		Key: "ci-gateway-key", Status: common.ChannelStatusEnabled,
		BaseURL: common.GetPointer("http://127.0.0.1:38080/v1"), Models: "c-alpha-public",
		Group: "default", Weight: common.GetPointer(uint(1)), Priority: common.GetPointer(int64(100)),
		AutoBan: common.GetPointer(0), ModelMapping: common.GetPointer(`{"c-alpha-public":"c-alpha-mapped"}`)}
	require.NoError(t, channel.Insert())
	for key, value := range map[string]string{
		"ModelPrice":                `{"c-alpha-public":1234}`,
		"LogConsumeEnabled":         "true",
		"tool_price_setting.prices": `{"web_search_preview":10}`,
	} {
		require.NoError(t, db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Option{Key: key, Value: value}).Error)
	}
}
