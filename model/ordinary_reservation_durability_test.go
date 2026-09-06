package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func ordinaryReservationRequest(user *User, token *Token, target int) GroupReservationRequest {
	return GroupReservationRequest{
		Source: GroupReservationWallet, UserId: user.Id, ModelName: "ordinary-durable-model",
		TokenId: token.Id, TokenKey: token.Key, TargetReserved: target,
		SubmissionID: common.GetUUID(), SubmissionLeaseToken: common.GetUUID(),
	}
}

func TestWalletOrdinaryReservationCommitErrorsClassifyEveryMoneyOperation(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) { checkOrdinaryReservationCommitErrors(t, fixture) })
	}
}

func checkOrdinaryReservationCommitErrors(t *testing.T, fixture groupReservationDatabase) {
	db := useGroupReservationDatabase(t, fixture)
	require.NoError(t, db.AutoMigrate(&TaskSubmission{}, &TaskAccountingEvent{}))
	user, token := seedGroupReservationWallet(t, db, "ordinary-commit", 2000, 2000)
	pool := installTaskSubmissionCommitErrorPool(t, db)

	req := ordinaryReservationRequest(user, token, 300)
	pool.failCommit = true
	result, err := ReconcileGroupReservation(req)
	require.NoError(t, err)
	require.Equal(t, 300, result.Reserved)

	req.ExpectedReserved, req.TargetReserved, req.SubmissionOperationID = 300, 600, common.GetUUID()
	pool.failCommit = true
	result, err = ReconcileGroupReservation(req)
	require.NoError(t, err)
	require.Equal(t, 600, result.Reserved)

	req.ExpectedReserved, req.TargetReserved, req.SubmissionOperationID = 600, 450, common.GetUUID()
	req.PostConsume, req.SubmissionFinalState = true, TaskSubmissionStateSettled
	pool.failCommit = true
	result, err = ReconcileGroupReservation(req)
	require.NoError(t, err)
	require.Equal(t, 450, result.Reserved)

	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 1550, userQuota)
	require.Equal(t, 1550, tokenRemain)
	require.Equal(t, 450, tokenUsed)
	submission, err := GetTaskSubmission(req.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, TaskSubmissionStateSettled, submission.State)
	require.Equal(t, 600, submission.LastExpectedQuota)
	require.Equal(t, 450, submission.LastTargetQuota)
	require.Equal(t, TaskSubmissionStateSettled, submission.LastFinalState)
}

func TestWalletOrdinaryReservationAmbiguousResizeThenRefundUsesDurableAmountOnce(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) { checkOrdinaryReservationAmbiguousResize(t, fixture) })
	}
}

func checkOrdinaryReservationAmbiguousResize(t *testing.T, fixture groupReservationDatabase) {
	db := useGroupReservationDatabase(t, fixture)
	require.NoError(t, db.AutoMigrate(&TaskSubmission{}, &TaskAccountingEvent{}))
	user, token := seedGroupReservationWallet(t, db, "ordinary-refund", 1000, 1000)
	req := ordinaryReservationRequest(user, token, 200)
	_, err := ReconcileGroupReservation(req)
	require.NoError(t, err)

	pool := installTaskSubmissionCommitErrorPool(t, db)
	pool.failCommit = true
	pool.queryErrorsAfterCommit = 1
	resize := req
	resize.ExpectedReserved, resize.TargetReserved = 200, 500
	resize.SubmissionOperationID = common.GetUUID()
	_, err = ReconcileGroupReservation(resize)
	require.ErrorIs(t, err, errInjectedTaskSubmissionCommit)

	refund := req
	refund.ExpectedReserved, refund.TargetReserved = 200, 0
	refund.PostConsume = true
	refund.UseDurableExpected = true
	refund.SubmissionOperationID = common.GetUUID()
	refund.SubmissionFinalState = TaskSubmissionStateReleased
	pool.queryErrorsAfterCommit = 0
	pool.failCommit = true
	result, err := ReconcileGroupReservation(refund)
	require.NoError(t, err)
	require.Equal(t, 500, result.PreviousReserved)
	require.Zero(t, result.Reserved)

	_, err = ReconcileGroupReservation(refund)
	require.ErrorIs(t, err, ErrTaskSubmissionReleased)
	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 1000, userQuota)
	require.Equal(t, 1000, tokenRemain)
	require.Zero(t, tokenUsed)
}

func TestWalletKnownSettlementMarkerClassifiesCommitErrorExactly(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) { checkKnownSettlementMarker(t, fixture) })
	}
}

func checkKnownSettlementMarker(t *testing.T, fixture groupReservationDatabase) {
	db := useGroupReservationDatabase(t, fixture)
	require.NoError(t, db.AutoMigrate(&TaskSubmission{}, &TaskAccountingEvent{}))
	user, token := seedGroupReservationWallet(t, db, "known-settlement-marker", 1000, 1000)
	req := ordinaryReservationRequest(user, token, 200)
	_, err := ReconcileGroupReservation(req)
	require.NoError(t, err)

	pool := installTaskSubmissionCommitErrorPool(t, db)
	pool.failCommit = true
	preserved, err := PreserveKnownTaskSubmissionSettlement(context.Background(), req.SubmissionID, req.SubmissionLeaseToken, user.Id, 400)
	require.NoError(t, err)
	require.Equal(t, TaskSubmissionStateSettlementPending, preserved.State)
	require.Equal(t, 200, preserved.ReservedQuota)
	require.Equal(t, 400, preserved.AcceptedQuota)

	preserved, err = PreserveKnownTaskSubmissionSettlement(context.Background(), req.SubmissionID, req.SubmissionLeaseToken, user.Id, 400)
	require.NoError(t, err)
	require.Equal(t, TaskSubmissionStateSettlementPending, preserved.State)
	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 800, userQuota)
	require.Equal(t, 800, tokenRemain)
	require.Equal(t, 200, tokenUsed)
}

func TestWalletOrdinaryReservationRestartRecoveryAndSafeRetention(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) { checkOrdinaryReservationRecovery(t, fixture) })
	}
}

func checkOrdinaryReservationRecovery(t *testing.T, fixture groupReservationDatabase) {
	db := useGroupReservationDatabase(t, fixture)
	require.NoError(t, db.AutoMigrate(&TaskSubmission{}, &TaskAccounting{}, &TaskAccountingEvent{}))
	user, token := seedGroupReservationWallet(t, db, "ordinary-restart", 1000, 1000)
	req := ordinaryReservationRequest(user, token, 250)
	_, err := ReconcileGroupReservation(req)
	require.NoError(t, err)
	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).
		Update("lease_expires_at", GetDBTimestamp()-1).Error)
	require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))
	require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))

	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 1000, userQuota)
	require.Equal(t, 1000, tokenRemain)
	require.Zero(t, tokenUsed)
	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).
		Updates(map[string]any{"updated_at": int64(1), "cache_pending": false}).Error)
	pendingEvent := &TaskAccountingEvent{
		SubmissionID: req.SubmissionID, EventID: common.GetUUID(), Kind: "retention-proof",
		FactsJSON: "{}", Ready: true, CreatedAt: 1,
	}
	require.NoError(t, db.Create(pendingEvent).Error)
	require.NoError(t, DeleteClosedOrdinaryTaskSubmissions(context.Background(), GetDBTimestamp()-1, 100))
	var count int64
	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&TaskAccountingEvent{}).Where("event_id = ?", pendingEvent.EventID).Update("delivered", true).Error)
	require.NoError(t, DeleteClosedOrdinaryTaskSubmissions(context.Background(), GetDBTimestamp()-1, 100))
	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).Count(&count).Error)
	require.Zero(t, count)
}
