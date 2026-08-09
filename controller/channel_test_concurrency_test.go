package controller

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelTestReleasesConcurrencyWhenReturningEarly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
	})

	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initModelListColumnNames(t)
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}))
	userID := 880001
	require.NoError(t, db.Create(&model.User{
		Id:       userID,
		Username: "channel_test_user",
		Group:    "default",
		Quota:    1000,
		Status:   common.UserStatusEnabled,
	}).Error)

	limit := 1
	channel := &model.Channel{
		Id:               880001,
		Type:             constant.ChannelTypeAnthropic,
		Key:              "sk-test",
		Name:             "anthropic-test",
		Status:           common.ChannelStatusEnabled,
		Models:           "claude-3-5-sonnet",
		ConcurrencyLimit: &limit,
	}

	result := testChannel(channel, userID, "claude-3-5-sonnet", string(constant.EndpointTypeOpenAIResponseCompact), false)

	require.Error(t, result.localErr)
	require.True(t, model.IsChannelConcurrencyAvailable(channel))
}
