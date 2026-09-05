package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type asyncTaskPublishWriter struct {
	gin.ResponseWriter
	onSuccess func()
	once      sync.Once
}

func (w *asyncTaskPublishWriter) WriteHeader(statusCode int) {
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		w.once.Do(w.onSuccess)
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

type asyncTaskUpstreamSnapshot struct {
	err             error
	submission      model.TaskSubmission
	user            model.User
	token           model.Token
	taskCount       int64
	accountingCount int64
	eventCount      int64
}

func migrateAsyncTaskControllerFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(
		&model.Task{},
		&model.TaskAccounting{},
		&model.TaskAccountingEvent{},
		&model.TaskAccountingLogReceipt{},
	))
}

func seedAsyncTaskControllerFunding(t *testing.T, db *gorm.DB, quota int) (*model.User, *model.Token) {
	t.Helper()
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	user := &model.User{
		Username: "b" + suffix,
		Password: "test-pass",
		Quota:    quota,
		AffCode:  "a" + suffix,
	}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{
		UserId:      user.Id,
		Key:         "k" + suffix,
		Name:        "t" + suffix,
		RemainQuota: quota,
	}
	require.NoError(t, db.Create(token).Error)
	return user, token
}

func seedAsyncTaskControllerChannel(t *testing.T, db *gorm.DB, channelType int, name, baseURL string, settings dto.ChannelOtherSettings) *model.Channel {
	t.Helper()
	weight, autoBan, concurrencyLimit := uint(1), 0, 0
	priority := int64(1)
	channel := &model.Channel{
		Type: channelType, Key: "fixture-key", Status: common.ChannelStatusEnabled,
		Name: name, Weight: &weight, ConcurrencyLimit: &concurrencyLimit, CreatedTime: time.Now().Unix(),
		BaseURL: &baseURL, Models: finalGroupRelayModel, Group: "fg-same", Priority: &priority, AutoBan: &autoBan,
	}
	channel.SetOtherSettings(settings)
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "fg-same", Model: finalGroupRelayModel, ChannelId: channel.Id, Enabled: true,
		Priority: &priority, Weight: weight,
	}).Error)
	return channel
}

func captureAsyncTaskUpstreamSnapshot(db *gorm.DB, userID, tokenID int) asyncTaskUpstreamSnapshot {
	var snapshot asyncTaskUpstreamSnapshot
	if err := db.Where("user_id = ?", userID).Order("created_at desc").First(&snapshot.submission).Error; err != nil {
		snapshot.err = err
		return snapshot
	}
	for _, query := range []struct {
		value any
		id    int
	}{
		{value: &snapshot.user, id: userID},
		{value: &snapshot.token, id: tokenID},
	} {
		if err := db.First(query.value, query.id).Error; err != nil {
			snapshot.err = err
			return snapshot
		}
	}
	for value, destination := range map[any]*int64{
		&model.Task{}:                &snapshot.taskCount,
		&model.TaskAccounting{}:      &snapshot.accountingCount,
		&model.TaskAccountingEvent{}: &snapshot.eventCount,
	} {
		if err := db.Model(value).Count(destination).Error; err != nil {
			snapshot.err = err
			return snapshot
		}
	}
	return snapshot
}

func requireAsyncTaskReservedBeforeUpstream(t *testing.T, snapshot asyncTaskUpstreamSnapshot, initialQuota, chargedQuota int) {
	t.Helper()
	require.NoError(t, snapshot.err)
	require.Equal(t, model.TaskSubmissionStateActive, snapshot.submission.State)
	require.Equal(t, chargedQuota, snapshot.submission.ReservedQuota)
	require.Zero(t, snapshot.submission.AcceptedQuota)
	require.Nil(t, snapshot.submission.TaskRowID)
	require.Equal(t, initialQuota-chargedQuota, snapshot.user.Quota)
	require.Zero(t, snapshot.user.UsedQuota)
	require.Zero(t, snapshot.user.RequestCount)
	require.Equal(t, initialQuota-chargedQuota, snapshot.token.RemainQuota)
	require.Equal(t, chargedQuota, snapshot.token.UsedQuota)
	require.Zero(t, snapshot.taskCount)
	require.Zero(t, snapshot.accountingCount)
	require.Zero(t, snapshot.eventCount)
}

func receiveAsyncTaskUpstreamSnapshot(t *testing.T, snapshots <-chan asyncTaskUpstreamSnapshot) asyncTaskUpstreamSnapshot {
	t.Helper()
	select {
	case snapshot := <-snapshots:
		return snapshot
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the fake upstream request")
		return asyncTaskUpstreamSnapshot{}
	}
}

func requireAsyncTaskDurableAtPublication(t *testing.T, db *gorm.DB, userID, tokenID, channelID, initialQuota, chargedQuota int) {
	t.Helper()
	var submission model.TaskSubmission
	require.NoError(t, db.Where("user_id = ?", userID).Order("created_at desc").First(&submission).Error)
	require.Equal(t, model.TaskSubmissionStateTransferred, submission.State)
	require.Equal(t, chargedQuota, submission.ReservedQuota)
	require.Equal(t, chargedQuota, submission.AcceptedQuota)
	require.NotNil(t, submission.TaskRowID)
	require.Positive(t, submission.TransferredAt)
	require.Zero(t, submission.LeaseExpiresAt)

	var task model.Task
	require.NoError(t, db.First(&task, *submission.TaskRowID).Error)
	require.Equal(t, chargedQuota, task.Quota)
	require.Equal(t, userID, task.UserId)
	require.Equal(t, channelID, task.ChannelId)

	var accounting model.TaskAccounting
	require.NoError(t, db.Where("task_row_id = ?", task.ID).First(&accounting).Error)
	require.Equal(t, chargedQuota, accounting.ChargedQuota)
	require.Equal(t, userID, accounting.UserID)
	require.Equal(t, tokenID, accounting.TokenID)
	require.Equal(t, channelID, accounting.ChannelID)

	var event model.TaskAccountingEvent
	require.NoError(t, db.Where("task_row_id = ? AND kind = ?", task.ID, "initial").First(&event).Error)
	require.True(t, event.Ready)
	require.False(t, event.Delivered)
	require.NotEmpty(t, event.EventID)
	var facts model.TaskAccountingLogFacts
	require.NoError(t, common.Unmarshal([]byte(event.FactsJSON), &facts))
	require.Equal(t, model.LogTypeConsume, facts.LogType)
	require.Equal(t, userID, facts.UserID)
	require.Equal(t, tokenID, facts.TokenID)
	require.Equal(t, channelID, facts.ChannelID)
	require.Equal(t, chargedQuota, facts.Quota)

	var user model.User
	var token model.Token
	var channel model.Channel
	require.NoError(t, db.First(&user, userID).Error)
	require.NoError(t, db.First(&token, tokenID).Error)
	require.NoError(t, db.First(&channel, channelID).Error)
	require.Equal(t, initialQuota-chargedQuota, user.Quota)
	require.Equal(t, chargedQuota, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Equal(t, initialQuota-chargedQuota, token.RemainQuota)
	require.Equal(t, chargedQuota, token.UsedQuota)
	require.EqualValues(t, chargedQuota, channel.UsedQuota)
}

func registerAsyncTaskHandoffFailure(t *testing.T, db *gorm.DB) *atomic.Bool {
	t.Helper()
	name := "test:async-task-handoff-" + common.GetUUID()
	var hit atomic.Bool
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "task_accountings" {
			hit.Store(true)
			tx.AddError(errors.New("injected task accounting create failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(name) })
	return &hit
}

func requireAsyncTaskReleasedOnce(t *testing.T, db *gorm.DB, c *gin.Context, userID, tokenID, channelID, initialQuota int) {
	t.Helper()
	var submission model.TaskSubmission
	require.NoError(t, db.Where("user_id = ?", userID).Order("created_at desc").First(&submission).Error)
	require.Equal(t, model.TaskSubmissionStateReleased, submission.State)
	require.Zero(t, submission.ReservedQuota)
	require.Zero(t, submission.AcceptedQuota)
	require.Positive(t, submission.ReleasedAt)
	releasedAt := submission.ReleasedAt

	info := c.MustGet("relay_info").(*relaycommon.RelayInfo)
	require.NotNil(t, info.Billing)
	info.Billing.Refund(c)
	require.NoError(t, db.Where("submission_id = ?", submission.SubmissionID).First(&submission).Error)
	require.Equal(t, releasedAt, submission.ReleasedAt)

	var user model.User
	var token model.Token
	var channel model.Channel
	require.NoError(t, db.First(&user, userID).Error)
	require.NoError(t, db.First(&token, tokenID).Error)
	require.NoError(t, db.First(&channel, channelID).Error)
	require.Equal(t, initialQuota, user.Quota)
	require.Zero(t, user.UsedQuota)
	require.Zero(t, user.RequestCount)
	require.Equal(t, initialQuota, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	require.Zero(t, channel.UsedQuota)

	for value := range map[any]struct{}{
		&model.Task{}:                {},
		&model.TaskAccounting{}:      {},
		&model.TaskAccountingEvent{}: {},
		&model.Log{}:                 {},
	} {
		var count int64
		require.NoError(t, db.Model(value).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestAsyncTaskGenericControllerPublishesOnlyAfterDurableHandoff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const initialQuota = 2000
	db := setupFinalGroupRelayDB(t)
	migrateAsyncTaskControllerFixture(t, db)
	user, token := seedAsyncTaskControllerFunding(t, db, initialQuota)
	snapshots := make(chan asyncTaskUpstreamSnapshot, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/videos/generations" {
			snapshots <- asyncTaskUpstreamSnapshot{err: fmt.Errorf("unexpected upstream path %q", r.URL.Path)}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		snapshots <- captureAsyncTaskUpstreamSnapshot(db, user.Id, token.Id)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"upstream-generic-b"}`))
	}))
	t.Cleanup(upstream.Close)
	channel := seedAsyncTaskControllerChannel(t, db, constant.ChannelTypeXai, "b-generic", upstream.URL, dto.ChannelOtherSettings{})
	c, recorder := finalGroupRelayContext(t, user, token, channel, "fg-same", "fg-same", "b-generic-submit")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"fg-relay-model","prompt":"test","duration":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	requestCtx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Request = c.Request.WithContext(requestCtx)
	var publications atomic.Int32
	c.Writer = &asyncTaskPublishWriter{ResponseWriter: c.Writer, onSuccess: func() {
		publications.Add(1)
		requireAsyncTaskDurableAtPublication(t, db, user.Id, token.Id, channel.Id, initialQuota, finalGroupBaseQuota)
	}}

	RelayTask(c)
	cancel()

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.EqualValues(t, 1, publications.Load())
	require.Contains(t, recorder.Body.String(), `"task_id":"task_`)
	require.NotContains(t, recorder.Body.String(), "upstream-generic-b")
	requireAsyncTaskReservedBeforeUpstream(t, receiveAsyncTaskUpstreamSnapshot(t, snapshots), initialQuota, finalGroupBaseQuota)

	var task model.Task
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&task).Error)
	require.Equal(t, "upstream-generic-b", task.PrivateData.UpstreamTaskID)
}

func TestAsyncTaskGenericControllerHandoffFailureDoesNotPublishAndReleasesOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const initialQuota = 2000
	db := setupFinalGroupRelayDB(t)
	migrateAsyncTaskControllerFixture(t, db)
	user, token := seedAsyncTaskControllerFunding(t, db, initialQuota)
	snapshots := make(chan asyncTaskUpstreamSnapshot, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snapshots <- captureAsyncTaskUpstreamSnapshot(db, user.Id, token.Id)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"upstream-generic-failure"}`))
	}))
	t.Cleanup(upstream.Close)
	channel := seedAsyncTaskControllerChannel(t, db, constant.ChannelTypeXai, "b-generic-fail", upstream.URL, dto.ChannelOtherSettings{})
	handoffFailure := registerAsyncTaskHandoffFailure(t, db)
	c, recorder := finalGroupRelayContext(t, user, token, channel, "fg-same", "fg-same", "b-generic-failure")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"fg-relay-model","prompt":"test","duration":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	requestCtx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Request = c.Request.WithContext(requestCtx)

	RelayTask(c)
	cancel()

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	require.True(t, handoffFailure.Load())
	require.NotContains(t, recorder.Body.String(), `"task_id":"task_`)
	require.Contains(t, recorder.Body.String(), "persist_task_billing_failed")
	requireAsyncTaskReservedBeforeUpstream(t, receiveAsyncTaskUpstreamSnapshot(t, snapshots), initialQuota, finalGroupBaseQuota)
	requireAsyncTaskReleasedOnce(t, db, c, user.Id, token.Id, channel.Id, initialQuota)
}

func TestAsyncTaskImageTasksEndpointPublishesOnlyAfterDurableHandoff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const initialQuota = 2000
	db := setupFinalGroupRelayDB(t)
	migrateAsyncTaskControllerFixture(t, db)
	user, token := seedAsyncTaskControllerFunding(t, db, initialQuota)
	snapshots := make(chan asyncTaskUpstreamSnapshot, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != dto.DefaultImageTasksEndpoint {
			snapshots <- asyncTaskUpstreamSnapshot{err: fmt.Errorf("unexpected upstream path %q", r.URL.Path)}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		snapshots <- captureAsyncTaskUpstreamSnapshot(db, user.Id, token.Id)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"upstream-image-b","task_id":"task_image_b","status":"queued","progress":0}`))
	}))
	t.Cleanup(upstream.Close)
	channel := seedAsyncTaskControllerChannel(t, db, constant.ChannelTypeOpenAI, "b-image", upstream.URL, dto.ChannelOtherSettings{
		ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint,
	})
	c, recorder := finalGroupRelayContext(t, user, token, channel, "fg-same", "fg-same", "b-image-submit")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"fg-relay-model","prompt":"test","n":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	requestCtx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Request = c.Request.WithContext(requestCtx)
	var publications atomic.Int32
	c.Writer = &asyncTaskPublishWriter{ResponseWriter: c.Writer, onSuccess: func() {
		publications.Add(1)
		requireAsyncTaskDurableAtPublication(t, db, user.Id, token.Id, channel.Id, initialQuota, finalGroupBaseQuota)
	}}

	RelayImageTaskSubmit(c)
	cancel()

	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.EqualValues(t, 1, publications.Load())
	require.JSONEq(t, `{"id":"upstream-image-b","task_id":"task_image_b","status":"queued","progress":0}`, recorder.Body.String())
	requireAsyncTaskReservedBeforeUpstream(t, receiveAsyncTaskUpstreamSnapshot(t, snapshots), initialQuota, finalGroupBaseQuota)

	var task model.Task
	require.NoError(t, db.Where("task_id = ?", "task_image_b").First(&task).Error)
	require.Equal(t, "upstream-image-b", task.PrivateData.UpstreamTaskID)
	require.Equal(t, model.TaskProtocolOpenAIImageTasks, task.PrivateData.TaskProtocol)
}

func TestAsyncTaskImageTasksEndpointHandoffFailureDoesNotPublishAndReleasesOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const initialQuota = 2000
	db := setupFinalGroupRelayDB(t)
	migrateAsyncTaskControllerFixture(t, db)
	user, token := seedAsyncTaskControllerFunding(t, db, initialQuota)
	snapshots := make(chan asyncTaskUpstreamSnapshot, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snapshots <- captureAsyncTaskUpstreamSnapshot(db, user.Id, token.Id)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"upstream-image-failure","task_id":"task_image_failure","status":"queued","progress":0,"data":[{"url":"https://example.invalid/result.png"}]}`))
	}))
	t.Cleanup(upstream.Close)
	channel := seedAsyncTaskControllerChannel(t, db, constant.ChannelTypeOpenAI, "b-image-fail", upstream.URL, dto.ChannelOtherSettings{
		ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint,
	})
	handoffFailure := registerAsyncTaskHandoffFailure(t, db)
	c, recorder := finalGroupRelayContext(t, user, token, channel, "fg-same", "fg-same", "b-image-failure")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"fg-relay-model","prompt":"test","n":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	requestCtx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Request = c.Request.WithContext(requestCtx)

	RelayImageTaskSubmit(c)
	cancel()

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	require.True(t, handoffFailure.Load())
	require.NotContains(t, recorder.Body.String(), "task_image_failure")
	require.NotContains(t, recorder.Body.String(), "upstream-image-failure")
	requireAsyncTaskReservedBeforeUpstream(t, receiveAsyncTaskUpstreamSnapshot(t, snapshots), initialQuota, finalGroupBaseQuota)
	requireAsyncTaskReleasedOnce(t, db, c, user.Id, token.Id, channel.Id, initialQuota)
}
