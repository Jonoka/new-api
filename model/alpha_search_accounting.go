package model

import (
	"context"

	"gorm.io/gorm"
)

// CompleteAlphaSearchSubmissionTx commits the successful zero-token request
// counters and log outbox in the reservation settlement transaction.
func CompleteAlphaSearchSubmissionTx(tx *gorm.DB, submissionID string, facts TaskAccountingLogFacts) error {
	return completeSynchronousSubmissionTx(tx, submissionID, "alpha_search", facts, true)
}

// ResolveAlphaSearchSettlement classifies an ambiguous final transaction from
// the durable submission receipt without replaying money or counters.
func ResolveAlphaSearchSettlement(ctx context.Context, req GroupReservationRequest) (*GroupReservationResult, error) {
	return resolveSynchronousSubmissionSettlement(ctx, req)
}
