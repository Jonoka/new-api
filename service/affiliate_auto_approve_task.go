package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const affiliateAutoApproveTickInterval = 1 * time.Hour

var affiliateAutoApproveOnce sync.Once

func StartAffiliateAutoApproveTask() {
	affiliateAutoApproveOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), "affiliate auto-approve task started: tick=1h")
			ticker := time.NewTicker(affiliateAutoApproveTickInterval)
			defer ticker.Stop()

			runAffiliateAutoApproveOnce()
			for range ticker.C {
				runAffiliateAutoApproveOnce()
			}
		})
	})
}

func runAffiliateAutoApproveOnce() {
	count, err := model.AutoApproveMatureApplications()
	if err != nil {
		logger.LogError(context.Background(), "affiliate auto-approve error: "+err.Error())
		return
	}
	if count > 0 {
		logger.LogInfo(context.Background(), fmt.Sprintf("affiliate auto-approved %d applications", count))
	}
}
