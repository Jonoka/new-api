package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	taskAccountingRecoveryInterval = 15 * time.Second
	taskAccountingRecoveryLimit    = 100
	taskAccountingRecoveryTimeout  = 10 * time.Second
)

// StartTaskAccountingRecovery runs independently of provider polling. All
// claims are durable DB claims, so multiple application nodes may run it.
func StartTaskAccountingRecovery() {
	go func() {
		runTaskAccountingRecovery()
		ticker := time.NewTicker(taskAccountingRecoveryInterval)
		defer ticker.Stop()
		for range ticker.C {
			runTaskAccountingRecovery()
		}
	}()
}

func runTaskAccountingRecovery() {
	ctx, cancel := context.WithTimeout(context.Background(), taskAccountingRecoveryTimeout)
	defer cancel()
	if err := model.RecoverTaskAccounting(ctx, taskAccountingRecoveryLimit); err != nil {
		common.SysLog("task accounting recovery pending: " + err.Error())
	}
}
