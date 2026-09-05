package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var serviceTestSQLite *gorm.DB

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open("file:service-accounting?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := migrateTaskAccountingFixture(db); err != nil {
		panic(err)
	}
	serviceTestSQLite = db
	model.DB, model.LOG_DB = db, db
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = true, false, false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	os.Exit(m.Run())
}

func migrateTaskAccountingFixture(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.Task{}, &model.TaskAccounting{}, &model.TaskAccountingEvent{},
		&model.TaskAccountingLogReceipt{}, &model.User{}, &model.Token{},
		&model.Channel{}, &model.Log{}, &model.UserSubscription{},
		&model.SubscriptionPreConsumeRecord{},
	)
}

func resetTaskAccountingFixture(t *testing.T, db, logDB *gorm.DB) {
	t.Helper()
	for _, value := range []any{&model.TaskAccountingLogReceipt{}, &model.TaskAccountingEvent{}, &model.TaskAccounting{}, &model.Task{}, &model.Token{}, &model.Channel{}, &model.SubscriptionPreConsumeRecord{}, &model.UserSubscription{}, &model.User{}} {
		require.NoError(t, db.Unscoped().Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(value).Error)
	}
	if logDB != db {
		require.NoError(t, logDB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.TaskAccountingLogReceipt{}).Error)
	}
	require.NoError(t, logDB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Log{}).Error)
}

func truncate(t *testing.T) {
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
}

func useTaskAccountingDB(t *testing.T, db, logDB *gorm.DB, dialect string) {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	model.DB, model.LOG_DB = db, logDB
	common.UsingSQLite = dialect == "sqlite"
	common.UsingMySQL = dialect == "mysql"
	common.UsingPostgreSQL = dialect == "postgres"
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = oldSQLite, oldMySQL, oldPostgres
	})
}

func seedDurablyReservedWalletTask(t *testing.T, charged int) (*model.Task, *relaycommon.RelayInfo) {
	t.Helper()
	user := &model.User{Username: "acct-user", Quota: 10000 - charged, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "acct-token-key", Name: "acct-token", Status: common.TokenStatusEnabled, RemainQuota: 10000 - charged, UsedQuota: charged}
	require.NoError(t, model.DB.Create(token).Error)
	channel := &model.Channel{Name: "acct-channel", Key: "provider-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(channel).Error)
	info := &relaycommon.RelayInfo{
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, UsingGroup: "default",
		OriginModelName: "async-model", RequestId: common.GetUUID(),
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id},
		PriceData:   types.PriceData{Quota: charged, ModelRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	info.BillingSource = BillingSourceWallet
	info.Billing = &BillingSession{
		relayInfo: info, funding: &WalletFunding{userId: user.Id, consumed: charged},
		preConsumedQuota: charged, tokenConsumed: charged,
	}
	task := model.InitTask("video", info)
	task.Status = model.TaskStatusInProgress
	task.Progress = "30%"
	task.Action = "generate"
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: info.OriginModelName, ModelRatio: 1, GroupRatio: 1}
	return task, info
}

func handoffWalletTask(t *testing.T, charged int) (*model.Task, *relaycommon.RelayInfo) {
	t.Helper()
	task, info := seedDurablyReservedWalletTask(t, charged)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	c.Set("username", "acct-user")
	c.Set("token_name", "acct-token")
	require.NoError(t, HandoffTaskBilling(c, info, task, "", charged))
	require.NotZero(t, task.ID)
	return task, info
}

func TestTaskAccountingWalletLifecycleAndDuplicate(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	task, _ := handoffWalletTask(t, 3000)

	var afterHandoff model.User
	require.NoError(t, model.DB.First(&afterHandoff, task.UserId).Error)
	require.Equal(t, 7000, afterHandoff.Quota)
	require.Equal(t, 3000, afterHandoff.UsedQuota)
	require.Equal(t, 1, afterHandoff.RequestCount)
	require.NoError(t, model.DB.Delete(&model.Token{}, task.PrivateData.TokenId).Error)

	expected := task.Status
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FailReason = "raw provider response must not enter billing event"
	task.FinishTime = time.Now().Unix()
	result, err := FinalizeTaskAccounting(context.Background(), task, expected, 0, task.FailReason)
	require.NoError(t, err)
	require.True(t, result.Applied)

	var user model.User
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 10000, user.Quota)
	require.Zero(t, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	var token model.Token
	require.NoError(t, model.DB.Unscoped().First(&token, task.PrivateData.TokenId).Error)
	require.Equal(t, 10000, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	require.True(t, token.DeletedAt.Valid)
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, task.ChannelId).Error)
	require.Zero(t, channel.UsedQuota)
	require.Equal(t, model.TaskStatusFailure, task.Status)
	require.Zero(t, task.Quota)

	_, err = FinalizeTaskAccounting(context.Background(), task, expected, 0, "conflicting repeat")
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 10000, user.Quota)
	require.Equal(t, 1, user.RequestCount)
	var receipts int64
	require.NoError(t, model.LOG_DB.Model(&model.TaskAccountingLogReceipt{}).Count(&receipts).Error)
	require.EqualValues(t, 2, receipts)
	var refund model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).First(&refund).Error)
	require.Equal(t, "async task failed", refund.Content)
	require.NotContains(t, refund.Other, "raw provider response")
}

func TestTaskAccountingDecisionRecoveryAndFirstDecisionWins(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	task, _ := handoffWalletTask(t, 2400)
	expected := task.Status
	task.Status = model.TaskStatusSuccess
	task.Progress = "100%"
	task.FinishTime = time.Now().Unix()
	accepted, err := model.AcceptTaskTerminalDecision(context.Background(), model.TaskTerminalDecision{
		TaskRowID: task.ID, ExpectedStatus: expected, Status: task.Status, Progress: task.Progress,
		FinishTime: task.FinishTime, Data: []byte(`{"result":"ok"}`), FinalQuota: 600, Reason: "frozen actual",
	})
	require.NoError(t, err)
	require.True(t, accepted.Won)
	var before model.Task
	require.NoError(t, model.DB.First(&before, task.ID).Error)
	require.Equal(t, expected, before.Status)
	require.Equal(t, 2400, before.Quota)
	stale := before
	stale.Progress = "95%"
	won, err := stale.UpdateWithStatus(expected)
	require.NoError(t, err)
	require.False(t, won)

	loser, err := model.AcceptTaskTerminalDecision(context.Background(), model.TaskTerminalDecision{
		TaskRowID: task.ID, ExpectedStatus: expected, Status: model.TaskStatusFailure,
		Progress: "100%", FinalQuota: 0, Reason: "late timeout",
	})
	require.NoError(t, err)
	require.False(t, loser.Won)
	require.Equal(t, 600, loser.Accounting.DecisionQuota)

	require.NoError(t, model.RecoverTaskAccounting(context.Background(), 100))
	var after model.Task
	require.NoError(t, model.DB.First(&after, task.ID).Error)
	require.Equal(t, model.TaskStatusSuccess, after.Status)
	require.Equal(t, 600, after.Quota)
	var user model.User
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 9400, user.Quota)
	require.Equal(t, 600, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
}

func TestTaskAccountingApplyRollbackThenRecovery(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	task, _ := handoffWalletTask(t, 1800)
	expected := task.Status
	task.Status, task.Progress = model.TaskStatusSuccess, "100%"
	accepted, err := model.AcceptTaskTerminalDecision(context.Background(), model.TaskTerminalDecision{
		TaskRowID: task.ID, ExpectedStatus: expected, Status: task.Status,
		Progress: task.Progress, FinalQuota: 300, Reason: "partial actual",
	})
	require.NoError(t, err)
	require.True(t, accepted.Won)
	require.NoError(t, model.DB.Delete(&model.Channel{}, task.ChannelId).Error)
	_, err = model.ApplyTaskAccountingDecision(context.Background(), task.ID)
	require.Error(t, err)
	var pending model.Task
	require.NoError(t, model.DB.First(&pending, task.ID).Error)
	require.Equal(t, expected, pending.Status)
	require.Equal(t, 1800, pending.Quota)
	var user model.User
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 8200, user.Quota)
	require.Equal(t, 1800, user.UsedQuota)

	require.NoError(t, model.DB.Create(&model.Channel{Id: task.ChannelId, Name: "restored-channel", Key: "provider-key", Status: common.ChannelStatusEnabled, UsedQuota: 1800}).Error)
	require.NoError(t, model.RecoverTaskAccounting(context.Background(), 100))
	require.NoError(t, model.DB.First(&pending, task.ID).Error)
	require.Equal(t, model.TaskStatusSuccess, pending.Status)
	require.Equal(t, 300, pending.Quota)
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 9700, user.Quota)
	require.Equal(t, 300, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
}

func TestTaskAccountingSeparateLogDBRecoversDelivery(t *testing.T) {
	logDB, err := gorm.Open(sqlite.Open("file:accounting-log-recovery?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	useTaskAccountingDB(t, serviceTestSQLite, logDB, "sqlite")
	resetTaskAccountingFixture(t, model.DB, serviceTestSQLite)
	// LOG_DB intentionally has no tables for the first delivery attempt.
	task, _ := handoffWalletTask(t, 900)
	expected := task.Status
	task.Status, task.Progress = model.TaskStatusFailure, "100%"
	_, err = FinalizeTaskAccounting(context.Background(), task, expected, 0, "provider diagnostic")
	require.NoError(t, err)
	var pending int64
	require.NoError(t, model.DB.Model(&model.TaskAccountingEvent{}).Where("delivered = ?", false).Count(&pending).Error)
	require.EqualValues(t, 2, pending)
	require.NoError(t, logDB.AutoMigrate(&model.Log{}, &model.TaskAccountingLogReceipt{}))
	require.NoError(t, model.RecoverTaskAccounting(context.Background(), 100))
	var logs, receipts int64
	require.NoError(t, logDB.Model(&model.Log{}).Count(&logs).Error)
	require.NoError(t, logDB.Model(&model.TaskAccountingLogReceipt{}).Count(&receipts).Error)
	require.EqualValues(t, 2, logs)
	require.EqualValues(t, 2, receipts)
	require.NoError(t, model.RecoverTaskAccounting(context.Background(), 100))
	require.NoError(t, logDB.Model(&model.Log{}).Count(&logs).Error)
	require.EqualValues(t, 2, logs)
}

func TestTaskAccountingSubscriptionWithoutToken(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	user := &model.User{Username: "subscription-user", Quota: 5000, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	channel := &model.Channel{Name: "subscription-channel", Key: "provider-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(channel).Error)
	subscription := &model.UserSubscription{UserId: user.Id, AmountTotal: 10000, AmountUsed: 1000, Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, model.DB.Create(subscription).Error)
	task := &model.Task{TaskID: model.GenerateTaskID(), UserId: user.Id, ChannelId: channel.Id, Group: "default", Status: model.TaskStatusInProgress, Progress: "30%",
		PrivateData: model.TaskPrivateData{BillingSource: BillingSourceSubscription, SubscriptionId: subscription.Id}}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		_, err := model.PersistAsyncTaskHandoffTx(tx, model.AsyncTaskHandoffRequest{Task: task, ChargedQuota: 1000,
			InitialLog: model.TaskAccountingLogFacts{UserID: user.Id, ChannelID: channel.Id, ModelName: "subscription-model", Group: "default", Other: map[string]any{"is_task": true}}})
		return err
	})
	require.NoError(t, err)
	expected := task.Status
	task.Status, task.Progress = model.TaskStatusSuccess, "100%"
	_, err = FinalizeTaskAccounting(context.Background(), task, expected, 0, "explicit zero actual")
	require.NoError(t, err)
	require.NoError(t, model.DB.First(subscription, subscription.Id).Error)
	require.Zero(t, subscription.AmountUsed)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	require.Equal(t, 5000, user.Quota)
	require.Zero(t, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Zero(t, task.PrivateData.TokenId)
}

type accountingQuotaAdaptor struct{ quota int }

func (*accountingQuotaAdaptor) Init(*relaycommon.RelayInfo) {}
func (*accountingQuotaAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, nil
}
func (*accountingQuotaAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}
func (a *accountingQuotaAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return a.quota
}

type terminalPollingAdaptor struct {
	result *relaycommon.TaskInfo
	quota  int
}

func (*terminalPollingAdaptor) Init(*relaycommon.RelayInfo) {}
func (*terminalPollingAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
}
func (a *terminalPollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return a.result, nil
}
func (a *terminalPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return a.quota
}

func TestGenericPollerUsesDurableTerminalAccounting(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	task, _ := handoffWalletTask(t, 1200)
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, task.ChannelId).Error)
	upstreamID := task.GetUpstreamTaskID()
	adaptor := &terminalPollingAdaptor{result: &relaycommon.TaskInfo{TaskID: upstreamID, Status: string(model.TaskStatusSuccess), Progress: "100%", Url: "https://example.invalid/result"}, quota: 300}
	err := updateVideoSingleTask(context.Background(), adaptor, &channel, upstreamID, map[string]*model.Task{taskPollingKey(channel.Id, upstreamID): task})
	require.NoError(t, err)
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.Equal(t, model.TaskStatusSuccess, persisted.Status)
	require.Equal(t, 300, persisted.Quota)
	var user model.User
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 9700, user.Quota)
	require.Equal(t, 300, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
}

func TestTimeoutSweepUsesDurableTerminalAccounting(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
	task, _ := handoffWalletTask(t, 750)
	task.SubmitTime = time.Now().Add(-time.Hour).Unix()
	won, err := task.UpdateWithStatus(task.Status)
	require.NoError(t, err)
	require.True(t, won)
	sweepTimedOutTaskBatch(context.Background(), []*model.Task{task}, "task timed out")
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.Equal(t, model.TaskStatusFailure, persisted.Status)
	require.Zero(t, persisted.Quota)
	var user model.User
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	require.Equal(t, 10000, user.Quota)
	require.Zero(t, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
}

func TestResolveTerminalTaskQuotaUsesFrozenContext(t *testing.T) {
	task := &model.Task{Quota: 4000, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
		ModelRatio: 2, GroupRatio: 1.5, OtherRatios: map[string]float64{"duration": 2},
	}}}
	quota, _, ok := CalculateTaskQuotaByTokens(task, 100)
	require.True(t, ok)
	require.Equal(t, common.QuotaFromFloat(600), quota)
	resolved, reason := ResolveTerminalTaskQuota(&accountingQuotaAdaptor{}, task, &relaycommon.TaskInfo{TotalTokens: 100})
	require.Equal(t, quota, resolved)
	require.Contains(t, reason, "modelRatio=2.00")
	resolved, _ = ResolveTerminalTaskQuota(&accountingQuotaAdaptor{quota: 0}, task, &relaycommon.TaskInfo{})
	require.Equal(t, task.Quota, resolved)
}

func TestBuildTaskConsumptionLogContentShowsEffectiveVariantPrice(t *testing.T) {
	quota, err := common.QuotaFromFloatStrict(0.7 * common.QuotaPerUnit)
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "textGenerate"}, OriginModelName: "grok-imagine-video",
		PriceData: types.PriceData{UsePrice: true, ModelPrice: 0.07, ModelPriceUnit: types.ModelPriceUnitSecond,
			Quota: quota, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}, OtherRatios: map[string]float64{"seconds": 10},
			BillingMeta: map[string]string{"resolution": "720p", "variant_price_status": "matched"}}}
	content := buildTaskConsumptionLogContent(info)
	for _, want := range []string{"按秒计费", "计费分辨率 720p", "档位单价 $0.070000 / 秒", "时长 10 秒", "分组倍率 1", "合计 $0.700000"} {
		require.Contains(t, content, want)
	}
	require.False(t, strings.Contains(content, "resolution: 1.40"))
}

func TestAsyncTaskAccountingExternalDatabases(t *testing.T) {
	fixtures := []struct {
		name, dsn string
		dialector gorm.Dialector
	}{}
	if dsn := os.Getenv("NEW_API_TEST_POSTGRES_DSN"); dsn != "" {
		fixtures = append(fixtures, struct {
			name, dsn string
			dialector gorm.Dialector
		}{"postgres", dsn, postgres.Open(dsn)})
	}
	if dsn := os.Getenv("NEW_API_TEST_MYSQL_DSN"); dsn != "" {
		fixtures = append(fixtures, struct {
			name, dsn string
			dialector gorm.Dialector
		}{"mysql", dsn, mysql.Open(dsn)})
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			naming := schema.NamingStrategy{TablePrefix: "async_acct_"}
			db, err := gorm.Open(fixture.dialector, &gorm.Config{NamingStrategy: naming})
			require.NoError(t, err)
			require.NoError(t, migrateTaskAccountingFixture(db))
			logDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-log?mode=memory&cache=shared", fixture.name)), &gorm.Config{NamingStrategy: naming})
			require.NoError(t, err)
			require.NoError(t, logDB.AutoMigrate(&model.Log{}, &model.TaskAccountingLogReceipt{}))
			useTaskAccountingDB(t, db, logDB, fixture.name)
			resetTaskAccountingFixture(t, db, logDB)
			task, _ := handoffWalletTask(t, 1000)
			expected := task.Status
			task.Status, task.Progress = model.TaskStatusSuccess, "100%"
			_, err = FinalizeTaskAccounting(context.Background(), task, expected, 250, "external fixture partial refund")
			require.NoError(t, err)
			var user model.User
			require.NoError(t, db.First(&user, task.UserId).Error)
			require.Equal(t, 9750, user.Quota)
			require.Equal(t, 250, user.UsedQuota)
			require.Equal(t, 1, user.RequestCount)
		})
	}
}
