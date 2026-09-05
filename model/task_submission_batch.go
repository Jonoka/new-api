package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type pendingTaskSubmissionBatch struct {
	OperationID  string
	SubmissionID string
	UserID       int
	TokenID      int
	UserDelta    int
	TokenDelta   int
}

var pendingTaskSubmissionBatches = struct {
	sync.Mutex
	operations map[string]pendingTaskSubmissionBatch
}{operations: make(map[string]pendingTaskSubmissionBatch)}

func appendTaskSubmissionFoldedBatchOperationID(receipt, operationID string) (string, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return receipt, errors.New("folded batch operation identity is required")
	}
	operationIDs := make([]string, 0, 1)
	if strings.TrimSpace(receipt) != "" {
		if err := common.UnmarshalJsonStr(receipt, &operationIDs); err != nil {
			return "", fmt.Errorf("decode folded batch operation receipt: %w", err)
		}
	}
	for _, existing := range operationIDs {
		if existing == operationID {
			return receipt, nil
		}
	}
	operationIDs = append(operationIDs, operationID)
	encoded, err := common.Marshal(operationIDs)
	if err != nil {
		return "", fmt.Errorf("encode folded batch operation receipt: %w", err)
	}
	return string(encoded), nil
}

func parkTaskSubmissionBatch(operation pendingTaskSubmissionBatch) error {
	if operation.OperationID == "" || operation.SubmissionID == "" || operation.UserID <= 0 ||
		(operation.UserDelta == 0 && operation.TokenDelta == 0) {
		return errors.New("invalid pending task submission batch operation")
	}
	pendingTaskSubmissionBatches.Lock()
	defer pendingTaskSubmissionBatches.Unlock()
	if existing, ok := pendingTaskSubmissionBatches.operations[operation.OperationID]; ok && existing != operation {
		return errors.New("pending task submission batch operation identity conflict")
	}
	pendingTaskSubmissionBatches.operations[operation.OperationID] = operation
	return nil
}

func snapshotPendingTaskSubmissionBatches(limit int, match func(pendingTaskSubmissionBatch) bool) []pendingTaskSubmissionBatch {
	if limit <= 0 {
		limit = 100
	}
	pendingTaskSubmissionBatches.Lock()
	defer pendingTaskSubmissionBatches.Unlock()
	operations := make([]pendingTaskSubmissionBatch, 0, limit)
	for _, operation := range pendingTaskSubmissionBatches.operations {
		if match != nil && !match(operation) {
			continue
		}
		operations = append(operations, operation)
		if len(operations) >= limit {
			break
		}
	}
	return operations
}

func hasPendingTaskSubmissionBatch(submissionID string) bool {
	pendingTaskSubmissionBatches.Lock()
	defer pendingTaskSubmissionBatches.Unlock()
	for _, operation := range pendingTaskSubmissionBatches.operations {
		if operation.SubmissionID == submissionID {
			return true
		}
	}
	return false
}

func taskSubmissionBatchReceiptContains(ctx context.Context, submissionID, operationID string) (bool, error) {
	var submission struct {
		FoldedBatchOperationIDs string
	}
	query := DB.WithContext(ctx).Model(&TaskSubmission{}).
		Select("folded_batch_operation_ids").
		Where("submission_id = ?", submissionID).
		Limit(1).Find(&submission)
	if query.Error != nil {
		return false, query.Error
	}
	if query.RowsAffected == 0 || strings.TrimSpace(submission.FoldedBatchOperationIDs) == "" {
		return false, nil
	}
	var operationIDs []string
	if err := common.UnmarshalJsonStr(submission.FoldedBatchOperationIDs, &operationIDs); err != nil {
		return false, fmt.Errorf("decode folded batch operation receipt: %w", err)
	}
	for _, candidate := range operationIDs {
		if candidate == operationID {
			return true, nil
		}
	}
	return false, nil
}

func removePendingTaskSubmissionBatch(operation pendingTaskSubmissionBatch) {
	pendingTaskSubmissionBatches.Lock()
	if current, ok := pendingTaskSubmissionBatches.operations[operation.OperationID]; ok && current == operation {
		delete(pendingTaskSubmissionBatches.operations, operation.OperationID)
	}
	pendingTaskSubmissionBatches.Unlock()
}

func restorePendingTaskSubmissionBatchLocked(operation pendingTaskSubmissionBatch) {
	if operation.UserDelta != 0 {
		batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
		batchUpdateStores[BatchUpdateTypeUserQuota][operation.UserID] += operation.UserDelta
		batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	}
	if operation.TokenDelta != 0 {
		batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
		batchUpdateStores[BatchUpdateTypeTokenQuota][operation.TokenID] += operation.TokenDelta
		batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	}
	removePendingTaskSubmissionBatch(operation)
}

// resolvePendingTaskSubmissionBatchLocked classifies one parked operation.
// Callers hold userQuotaBatchApplyLock, followed by tokenQuotaBatchApplyLock.
func resolvePendingTaskSubmissionBatchLocked(ctx context.Context, operation pendingTaskSubmissionBatch) (bool, error) {
	committed, err := taskSubmissionBatchReceiptContains(ctx, operation.SubmissionID, operation.OperationID)
	if err != nil {
		return false, err
	}
	if !committed {
		restorePendingTaskSubmissionBatchLocked(operation)
		return false, nil
	}
	removePendingTaskSubmissionBatch(operation)
	return committed, nil
}

func recoverPendingTaskSubmissionBatchesLocked(ctx context.Context, limit int, match func(pendingTaskSubmissionBatch) bool) error {
	var firstErr error
	for _, operation := range snapshotPendingTaskSubmissionBatches(limit, match) {
		if _, err := resolvePendingTaskSubmissionBatchLocked(ctx, operation); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func recoverPendingTaskSubmissionBatchesForReservationLocked(ctx context.Context, userID, tokenID int) error {
	match := func(operation pendingTaskSubmissionBatch) bool {
		return operation.UserDelta != 0 && operation.UserID == userID ||
			operation.TokenDelta != 0 && operation.TokenID == tokenID
	}
	for {
		operations := snapshotPendingTaskSubmissionBatches(100, match)
		if len(operations) == 0 {
			return nil
		}
		for _, operation := range operations {
			if _, err := resolvePendingTaskSubmissionBatchLocked(ctx, operation); err != nil {
				return err
			}
		}
	}
}

// RecoverPendingTaskSubmissionBatches resolves ambiguous folded legacy batch
// values independently of provider polling. Unreadable receipts remain parked.
func RecoverPendingTaskSubmissionBatches(ctx context.Context, limit int) error {
	userQuotaBatchApplyLock.Lock()
	defer userQuotaBatchApplyLock.Unlock()
	tokenQuotaBatchApplyLock.Lock()
	defer tokenQuotaBatchApplyLock.Unlock()
	return recoverPendingTaskSubmissionBatchesLocked(ctx, limit, nil)
}
