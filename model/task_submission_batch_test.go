package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var errInjectedTaskSubmissionCommit = errors.New("injected error after commit")
var errInjectedTaskSubmissionQuery = errors.New("injected receipt query error")

type taskSubmissionCommitErrorPool struct {
	gorm.ConnPool
	beginner gorm.TxBeginner

	mu                       sync.Mutex
	failCommit               bool
	failCommitBefore         bool
	queryErrors              int
	queryErrorsAfterCommit   int
	queryErrorsAfterRollback int
}

func (pool *taskSubmissionCommitErrorPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := pool.beginner.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &taskSubmissionCommitErrorTx{Tx: tx, pool: pool}, nil
}

func (pool *taskSubmissionCommitErrorPool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	pool.mu.Lock()
	if pool.queryErrors > 0 {
		pool.queryErrors--
		pool.mu.Unlock()
		return nil, errInjectedTaskSubmissionQuery
	}
	pool.mu.Unlock()
	return pool.ConnPool.QueryContext(ctx, query, args...)
}

func (pool *taskSubmissionCommitErrorPool) setQueryErrors(count int) {
	pool.mu.Lock()
	pool.queryErrors = count
	pool.mu.Unlock()
}

type taskSubmissionCommitErrorTx struct {
	*sql.Tx
	pool *taskSubmissionCommitErrorPool
}

func (tx *taskSubmissionCommitErrorTx) Commit() error {
	tx.pool.mu.Lock()
	if tx.pool.failCommitBefore {
		tx.pool.failCommitBefore = false
		tx.pool.queryErrors += tx.pool.queryErrorsAfterCommit
		tx.pool.mu.Unlock()
		return errInjectedTaskSubmissionCommit
	}
	tx.pool.mu.Unlock()
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	tx.pool.mu.Lock()
	defer tx.pool.mu.Unlock()
	if !tx.pool.failCommit {
		return nil
	}
	tx.pool.failCommit = false
	tx.pool.queryErrors += tx.pool.queryErrorsAfterCommit
	return errInjectedTaskSubmissionCommit
}

func TestTaskSubmissionBatchAmbiguousRollbackRestoresAfterUnavailableReceipt(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			require.NoError(t, db.AutoMigrate(&TaskSubmission{}))
			resetTaskSubmissionBatchState(t)
			common.BatchUpdateEnabled = true
			user, token := seedGroupReservationWallet(t, db, "ambiguous-rollback-"+fixture.name, 1000, 1000)
			require.NoError(t, DecreaseUserQuota(user.Id, 100, false))
			require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 100))

			pool := installTaskSubmissionCommitErrorPool(t, db)
			pool.failCommitBefore = true
			pool.queryErrorsAfterCommit = 1
			req := newTaskSubmissionBatchRequest(user, token, 200)
			_, err := ReconcileGroupReservation(req)
			require.ErrorIs(t, err, errInjectedTaskSubmissionCommit)
			require.Equal(t, 1, pendingTaskSubmissionBatchCount())
			userDelta, tokenDelta := readPendingTaskSubmissionBatchDeltas(user.Id, token.Id)
			require.Zero(t, userDelta)
			require.Zero(t, tokenDelta)
			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 1000, userQuota)
			require.Equal(t, 1000, tokenRemain)
			require.Zero(t, tokenUsed)

			require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))
			require.NoError(t, RecoverPendingTaskSubmissionBatches(context.Background(), 100))
			require.Zero(t, pendingTaskSubmissionBatchCount())
			userDelta, tokenDelta = readPendingTaskSubmissionBatchDeltas(user.Id, token.Id)
			require.Equal(t, -100, userDelta)
			require.Equal(t, -100, tokenDelta)

			flushUserQuotaBatchUpdates()
			flushTokenQuotaBatchUpdates()
			flushUserQuotaBatchUpdates()
			flushTokenQuotaBatchUpdates()
			userQuota, tokenRemain, tokenUsed = readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 900, userQuota)
			require.Equal(t, 900, tokenRemain)
			require.Equal(t, 100, tokenUsed)
		})
	}
}

func (tx *taskSubmissionCommitErrorTx) Rollback() error {
	err := tx.Tx.Rollback()
	tx.pool.mu.Lock()
	tx.pool.queryErrors += tx.pool.queryErrorsAfterRollback
	tx.pool.mu.Unlock()
	return err
}

func installTaskSubmissionCommitErrorPool(t *testing.T, db *gorm.DB) *taskSubmissionCommitErrorPool {
	t.Helper()
	base := db.Statement.ConnPool
	beginner, ok := base.(gorm.TxBeginner)
	require.True(t, ok)
	pool := &taskSubmissionCommitErrorPool{ConnPool: base, beginner: beginner}
	oldConfigPool := db.Config.ConnPool
	db.Statement.ConnPool = pool
	db.Config.ConnPool = pool
	t.Cleanup(func() {
		db.Statement.ConnPool = base
		db.Config.ConnPool = oldConfigPool
	})
	return pool
}

func resetTaskSubmissionBatchState(t *testing.T) {
	t.Helper()
	userQuotaBatchApplyLock.Lock()
	tokenQuotaBatchApplyLock.Lock()
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	batchUpdateStores[BatchUpdateTypeTokenQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	pendingTaskSubmissionBatches.Lock()
	pendingTaskSubmissionBatches.operations = make(map[string]pendingTaskSubmissionBatch)
	pendingTaskSubmissionBatches.Unlock()
	tokenQuotaBatchApplyLock.Unlock()
	userQuotaBatchApplyLock.Unlock()
	t.Cleanup(func() {
		resetTaskSubmissionBatchStateWithoutCleanup()
	})
}

func resetTaskSubmissionBatchStateWithoutCleanup() {
	userQuotaBatchApplyLock.Lock()
	tokenQuotaBatchApplyLock.Lock()
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	batchUpdateStores[BatchUpdateTypeTokenQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	pendingTaskSubmissionBatches.Lock()
	pendingTaskSubmissionBatches.operations = make(map[string]pendingTaskSubmissionBatch)
	pendingTaskSubmissionBatches.Unlock()
	tokenQuotaBatchApplyLock.Unlock()
	userQuotaBatchApplyLock.Unlock()
}

func pendingTaskSubmissionBatchCount() int {
	pendingTaskSubmissionBatches.Lock()
	defer pendingTaskSubmissionBatches.Unlock()
	return len(pendingTaskSubmissionBatches.operations)
}

func readPendingTaskSubmissionBatchDeltas(userID, tokenID int) (int, int) {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	userDelta := batchUpdateStores[BatchUpdateTypeUserQuota][userID]
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	tokenDelta := batchUpdateStores[BatchUpdateTypeTokenQuota][tokenID]
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	return userDelta, tokenDelta
}

func newTaskSubmissionBatchRequest(user *User, token *Token, target int) GroupReservationRequest {
	return GroupReservationRequest{
		Source: GroupReservationWallet, UserId: user.Id, ModelName: "batch-receipt-model",
		TokenId: token.Id, TokenKey: token.Key, TargetReserved: target,
		SubmissionID: common.GetUUID(), SubmissionLeaseToken: common.GetUUID(),
	}
}

func TestTaskSubmissionBatchRollbackRestoresBeforeUnavailableReceiptRead(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			require.NoError(t, db.AutoMigrate(&TaskSubmission{}))
			resetTaskSubmissionBatchState(t)
			common.BatchUpdateEnabled = true
			user, token := seedGroupReservationWallet(t, db, "rollback-batch-"+fixture.name, 1000, 1000)
			require.NoError(t, DecreaseUserQuota(user.Id, 100, false))
			require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 100))

			pool := installTaskSubmissionCommitErrorPool(t, db)
			pool.queryErrorsAfterRollback = 1
			req := newTaskSubmissionBatchRequest(user, token, 200)
			callbackErr := errors.New("injected callback rollback")
			_, err := WithReconciledGroupReservation(req, func(*gorm.DB, *GroupReservationResult) error {
				return callbackErr
			})
			require.ErrorIs(t, err, callbackErr)

			_, err = taskSubmissionBatchReceiptContains(context.Background(), req.SubmissionID, common.GetUUID())
			require.ErrorIs(t, err, errInjectedTaskSubmissionQuery)
			userDelta, tokenDelta := readPendingTaskSubmissionBatchDeltas(user.Id, token.Id)
			require.Equal(t, -100, userDelta)
			require.Equal(t, -100, tokenDelta)
			require.Zero(t, pendingTaskSubmissionBatchCount())
			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 1000, userQuota)
			require.Equal(t, 1000, tokenRemain)
			require.Zero(t, tokenUsed)
		})
	}
}

func TestTaskSubmissionBatchCommitErrorResolvesCommittedReceipt(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			require.NoError(t, db.AutoMigrate(&TaskSubmission{}))
			resetTaskSubmissionBatchState(t)
			common.BatchUpdateEnabled = true
			user, token := seedGroupReservationWallet(t, db, "commit-batch-"+fixture.name, 1000, 1000)
			require.NoError(t, DecreaseUserQuota(user.Id, 100, false))
			require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 100))

			pool := installTaskSubmissionCommitErrorPool(t, db)
			pool.failCommit = true
			req := newTaskSubmissionBatchRequest(user, token, 200)
			result, err := ReconcileGroupReservation(req)
			require.NoError(t, err)
			require.Equal(t, 200, result.Reserved)
			require.Zero(t, pendingTaskSubmissionBatchCount())
			userDelta, tokenDelta := readPendingTaskSubmissionBatchDeltas(user.Id, token.Id)
			require.Zero(t, userDelta)
			require.Zero(t, tokenDelta)
			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 700, userQuota)
			require.Equal(t, 700, tokenRemain)
			require.Equal(t, 300, tokenUsed)

			var submission TaskSubmission
			require.NoError(t, db.Where("submission_id = ?", req.SubmissionID).First(&submission).Error)
			var operationIDs []string
			require.NoError(t, common.UnmarshalJsonStr(submission.FoldedBatchOperationIDs, &operationIDs))
			require.Len(t, operationIDs, 1)
		})
	}
}

func TestTaskSubmissionBatchUnavailableBlocksThenRecoversOnceAfterJournalAdvance(t *testing.T) {
	db := useGroupReservationDatabase(t, groupReservationDatabases(t)[0])
	require.NoError(t, db.AutoMigrate(&TaskSubmission{}))
	resetTaskSubmissionBatchState(t)
	common.BatchUpdateEnabled = true
	user, token := seedGroupReservationWallet(t, db, "parked-batch", 1000, 1000)
	require.NoError(t, DecreaseUserQuota(user.Id, 100, false))
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 100))

	pool := installTaskSubmissionCommitErrorPool(t, db)
	pool.failCommit = true
	pool.queryErrorsAfterCommit = 1
	req := newTaskSubmissionBatchRequest(user, token, 200)
	_, err := ReconcileGroupReservation(req)
	require.ErrorIs(t, err, errInjectedTaskSubmissionCommit)
	require.Equal(t, 1, pendingTaskSubmissionBatchCount())
	userDelta, tokenDelta := readPendingTaskSubmissionBatchDeltas(user.Id, token.Id)
	require.Zero(t, userDelta)
	require.Zero(t, tokenDelta)

	pool.setQueryErrors(1)
	blocked := req
	blocked.ExpectedReserved, blocked.TargetReserved = 200, 300
	_, err = ReconcileGroupReservation(blocked)
	require.ErrorIs(t, err, errInjectedTaskSubmissionQuery)
	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 700, userQuota)
	require.Equal(t, 700, tokenRemain)
	require.Equal(t, 300, tokenUsed)

	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).
		Update("last_operation_id", common.GetUUID()).Error)
	require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))
	require.NoError(t, RecoverPendingTaskSubmissionBatches(context.Background(), 100))
	require.Zero(t, pendingTaskSubmissionBatchCount())
	userDelta, tokenDelta = readPendingTaskSubmissionBatchDeltas(user.Id, token.Id)
	require.Zero(t, userDelta)
	require.Zero(t, tokenDelta)
	userQuota, tokenRemain, tokenUsed = readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 700, userQuota)
	require.Equal(t, 700, tokenRemain)
	require.Equal(t, 300, tokenUsed)

	_, err = ReconcileGroupReservation(blocked)
	require.NoError(t, err)
	userQuota, tokenRemain, tokenUsed = readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 600, userQuota)
	require.Equal(t, 600, tokenRemain)
	require.Equal(t, 400, tokenUsed)
}

func TestTokenBatchFlushSerializesReservationAdmission(t *testing.T) {
	db := useGroupReservationDatabase(t, groupReservationDatabases(t)[0])
	require.NoError(t, db.AutoMigrate(&TaskSubmission{}))
	resetTaskSubmissionBatchState(t)
	common.BatchUpdateEnabled = true
	user, token := seedGroupReservationWallet(t, db, "token-flush-race", 2000, 1000)
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 100))

	started := make(chan struct{})
	release := make(chan struct{})
	originalApply := applyTokenQuotaBatchDelta
	applyTokenQuotaBatchDelta = func(id, delta int) error {
		close(started)
		<-release
		return increaseTokenQuota(id, delta)
	}
	t.Cleanup(func() { applyTokenQuotaBatchDelta = originalApply })

	flushDone := make(chan struct{})
	go func() {
		flushTokenQuotaBatchUpdates()
		close(flushDone)
	}()
	<-started

	reservationDone := make(chan error, 1)
	go func() {
		_, err := ReconcileGroupReservation(newTaskSubmissionBatchRequest(user, token, 950))
		reservationDone <- err
	}()
	select {
	case err := <-reservationDone:
		close(release)
		<-flushDone
		t.Fatalf("reservation passed detached token flush: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-flushDone
	require.ErrorIs(t, <-reservationDone, ErrGroupReservationTokenInsufficient)

	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 2000, userQuota)
	require.Equal(t, 900, tokenRemain)
	require.Equal(t, 100, tokenUsed)
	_, tokenDelta := readPendingTaskSubmissionBatchDeltas(user.Id, token.Id)
	require.Zero(t, tokenDelta)
	require.Zero(t, pendingTaskSubmissionBatchCount())
}

func TestTaskSubmissionFoldedBatchReceiptDoesNotContainTokenKey(t *testing.T) {
	db := useGroupReservationDatabase(t, groupReservationDatabases(t)[0])
	require.NoError(t, db.AutoMigrate(&TaskSubmission{}))
	resetTaskSubmissionBatchState(t)
	common.BatchUpdateEnabled = true
	user, token := seedGroupReservationWallet(t, db, fmt.Sprintf("receipt-privacy-%d", time.Now().UnixNano()), 1000, 1000)
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 10))
	req := newTaskSubmissionBatchRequest(user, token, 20)
	_, err := ReconcileGroupReservation(req)
	require.NoError(t, err)
	var submission TaskSubmission
	require.NoError(t, db.Where("submission_id = ?", req.SubmissionID).First(&submission).Error)
	require.NotContains(t, submission.FoldedBatchOperationIDs, token.Key)
}
