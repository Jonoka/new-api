package model

import (
	"context"

	"gorm.io/gorm"
)

// CompleteViolationFeeSubmissionTx commits fee counters and its durable log
// event in the same primary transaction as the post-use money adjustment.
func CompleteViolationFeeSubmissionTx(tx *gorm.DB, submissionID string, facts TaskAccountingLogFacts) error {
	return completeSynchronousSubmissionTx(tx, submissionID, "violation_fee", facts, true)
}

// ResolveViolationFeeSettlement classifies an ambiguous or repeated fee from
// the exact journal operation without replaying money or counters.
func ResolveViolationFeeSettlement(ctx context.Context, req GroupReservationRequest) (*GroupReservationResult, error) {
	return resolveSynchronousSubmissionSettlement(ctx, req)
}
