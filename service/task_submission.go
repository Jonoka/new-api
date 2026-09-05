package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const taskSubmissionHeartbeatInterval = 10 * time.Second
const taskSubmissionHeartbeatWriteTimeout = 5 * time.Second

func EnsureTaskSubmissionIdentity(info *relaycommon.RelayInfo) {
	if info != nil {
		info.EnsureTaskSubmissionIdentity()
	}
}

// CreateQueuedTaskSubmission publishes a Canvas/image task only together with
// its zero-value submission recovery identity.
func CreateQueuedTaskSubmission(task *model.Task) (string, string, error) {
	if task == nil {
		return "", "", errors.New("queued task is required")
	}
	submissionID := common.GetUUID()
	leaseToken := common.GetUUID()
	if err := model.CreateQueuedTaskSubmission(task, submissionID, leaseToken); err != nil {
		return "", "", err
	}
	return submissionID, leaseToken, nil
}

// StartTaskSubmissionHeartbeat synchronously proves ownership before starting
// a bounded heartbeat. The returned stop function is idempotent.
func StartTaskSubmissionHeartbeat(ctx context.Context, submissionID, leaseToken string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	writeCtx, cancel := context.WithTimeout(ctx, taskSubmissionHeartbeatWriteTimeout)
	won, err := model.ExtendTaskSubmissionLease(writeCtx, submissionID, leaseToken, model.TaskSubmissionLeaseDuration)
	cancel()
	if err != nil {
		return nil, err
	}
	if !won {
		return nil, model.ErrTaskSubmissionLeaseLost
	}
	return startTaskSubmissionHeartbeatAfterReservation(ctx, submissionID, leaseToken), nil
}

// startTaskSubmissionHeartbeatAfterReservation relies on the reservation
// transaction that just renewed the lease, avoiding a second fallible DB step
// before BillingSession is published to RelayInfo.
func startTaskSubmissionHeartbeatAfterReservation(ctx context.Context, submissionID, leaseToken string) func() {
	if ctx == nil {
		ctx = context.Background()
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(cancel)
		<-done
	}
	go func() {
		ticker := time.NewTicker(taskSubmissionHeartbeatInterval)
		defer ticker.Stop()
		defer close(done)
		defer cancel()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				writeCtx, writeCancel := context.WithTimeout(heartbeatCtx, taskSubmissionHeartbeatWriteTimeout)
				won, err := model.ExtendTaskSubmissionLease(writeCtx, submissionID, leaseToken, model.TaskSubmissionLeaseDuration)
				writeCancel()
				if err != nil {
					if heartbeatCtx.Err() != nil {
						return
					}
					continue
				}
				if !won {
					return
				}
			}
		}
	}()
	return stop
}
