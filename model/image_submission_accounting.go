package model

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// CompleteImageSubmissionTx runs inside the reservation's final transaction.
// A synchronous image has no public task row, so its event uses SubmissionID.
func CompleteImageSubmissionTx(tx *gorm.DB, submissionID string, facts TaskAccountingLogFacts, countRequest bool) error {
	return completeSynchronousSubmissionTx(tx, submissionID, "synchronous_image", facts, countRequest)
}

func completeSynchronousSubmissionTx(tx *gorm.DB, submissionID, eventKind string, facts TaskAccountingLogFacts, countRequest bool) error {
	if submissionID == "" || eventKind == "" || facts.UserID <= 0 || facts.Quota < 0 {
		return errors.New("invalid synchronous submission accounting")
	}
	if err := incrementTaskUsageTx(tx, facts.UserID, facts.ChannelID, facts.Quota, countRequest); err != nil {
		return err
	}
	event, err := createTaskAccountingEventTx(tx, 0, eventKind, facts, true)
	if err != nil {
		return err
	}
	return tx.Model(event).Update("submission_id", submissionID).Error
}

func ResolveImageSubmissionSettlement(ctx context.Context, req GroupReservationRequest) (*GroupReservationResult, error) {
	return resolveSynchronousSubmissionSettlement(ctx, req)
}

func resolveSynchronousSubmissionSettlement(ctx context.Context, req GroupReservationRequest) (*GroupReservationResult, error) {
	if req.SubmissionFinalState != TaskSubmissionStateSettled {
		return nil, ErrTaskSubmissionConflict
	}
	resolution, err := resolveTaskSubmissionReservationCommit(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resolution.Committed || !resolution.CanReturn {
		return nil, ErrTaskSubmissionConflict
	}
	return &resolution.Result, nil
}
