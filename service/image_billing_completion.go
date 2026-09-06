package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CompleteDeferredImageBilling completes the ordinary synchronous image path
// when the response contains image data and no asynchronous task identity.
func CompleteDeferredImageBilling(c *gin.Context, info *relaycommon.RelayInfo) error {
	if info == nil || !info.DeferTaskBilling {
		return nil
	}
	if info.DeferredImageSettlement == nil || info.TaskSubmissionTaskRowID > 0 {
		return errors.New("synchronous image settlement is unavailable")
	}
	if err := info.DeferredImageSettlement(c); err != nil {
		return err
	}
	info.DeferTaskBilling = false
	info.DeferredImageSettlement = nil
	return nil
}

func prepareDeferredImageSettlement(c *gin.Context, info *relaycommon.RelayInfo, params model.RecordConsumeLogParams, countRequest bool, metricUsage *ChannelMetricUsage) func(*gin.Context) error {
	facts := BuildTaskAccountingLogFacts(c, info, params.Quota)
	facts.ModelName, facts.Content, facts.Other = params.ModelName, params.Content, params.Other
	facts.PromptTokens, facts.CompletionTokens = params.PromptTokens, params.CompletionTokens
	facts.UseTimeSeconds, facts.IsStream = params.UseTimeSeconds, params.IsStream
	if metricUsage != nil {
		metricContext := c.Copy()
		usage := *metricUsage
		info.AttachImageMetricUsage = func(quota int, err error) { AttachChannelMetricUsageAfterSettlement(metricContext, usage, quota, err) }
	}
	operationID := common.GetUUID()
	return func(ctx *gin.Context) error {
		return settleDeferredImageSubmission(ctx, info, facts, countRequest, metricUsage, operationID)
	}
}

func settleDeferredImageSubmission(c *gin.Context, info *relaycommon.RelayInfo, facts model.TaskAccountingLogFacts, countRequest bool, metricUsage *ChannelMetricUsage, operationID string) error {
	return settleSynchronousSubmission(c, info, facts, metricUsage, operationID, "image", func(tx *gorm.DB, submissionID string, facts model.TaskAccountingLogFacts) error {
		return model.CompleteImageSubmissionTx(tx, submissionID, facts, countRequest)
	}, model.ResolveImageSubmissionSettlement)
}

type synchronousSubmissionCompleter func(*gorm.DB, string, model.TaskAccountingLogFacts) error
type synchronousSubmissionResolver func(context.Context, model.GroupReservationRequest) (*model.GroupReservationResult, error)

func settleSynchronousSubmission(c *gin.Context, info *relaycommon.RelayInfo, facts model.TaskAccountingLogFacts, metricUsage *ChannelMetricUsage, operationID, billingKind string, complete synchronousSubmissionCompleter, resolve synchronousSubmissionResolver) error {
	if complete == nil || resolve == nil {
		return errors.New("synchronous submission accounting callbacks are required")
	}
	info.EnsureTaskSubmissionIdentity()
	var session *BillingSession
	if info.Billing != nil {
		var ok bool
		session, ok = info.Billing.(*BillingSession)
		if !ok {
			return errors.New("unsupported " + billingKind + " billing session")
		}
		session.mu.Lock()
		defer session.mu.Unlock()
		if session.settled {
			return nil
		}
		if session.refunded {
			return errors.New(billingKind + " reservation was released")
		}
	} else if facts.Quota != 0 {
		return errors.New("paid " + billingKind + " has no reservation")
	}
	source, reserved := BillingSourceWallet, 0
	if session != nil {
		source, reserved = session.funding.Source(), session.preConsumedQuota
	}
	req := model.GroupReservationRequest{
		Source: source, RequestId: info.RequestId, UserId: info.UserId, ModelName: info.OriginModelName,
		SubscriptionId: info.SubscriptionId, TokenId: info.TokenId, TokenKey: info.TokenKey,
		TokenUnlimited: info.TokenUnlimited, SkipTokenQuota: info.SkipTokenQuota || info.IsPlayground,
		ExpectedReserved: reserved, TargetReserved: facts.Quota, PostConsume: true,
		UseDurableExpected: true,
		SubmissionID:       info.TaskSubmissionID, SubmissionLeaseToken: info.TaskSubmissionLeaseToken,
		SubmissionOperationID: operationID, SubmissionFinalState: model.TaskSubmissionStateSettled,
	}
	result, err := model.WithReconciledGroupReservation(req, func(tx *gorm.DB, result *model.GroupReservationResult) error {
		if source == BillingSourceSubscription && facts.Other != nil {
			facts.Other["subscription_post_delta"] = info.SubscriptionPostDelta + int64(facts.Quota-result.PreviousReserved)
		}
		return complete(tx, req.SubmissionID, facts)
	})
	if err != nil {
		resolveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resolved, resolveErr := resolve(resolveCtx, req)
		cancel()
		if resolveErr == nil {
			result, err = resolved, nil
		}
	}
	if metricUsage != nil {
		AttachChannelMetricUsageAfterSettlement(c, *metricUsage, facts.Quota, err)
	}
	if err != nil {
		return err
	}
	if session != nil {
		reserved = result.PreviousReserved
		session.applyReservationResult(result)
		session.fundingSettled, session.settled = true, true
		session.stopTaskSubmissionHeartbeatLocked()
		if source == BillingSourceSubscription {
			info.SubscriptionPostDelta += int64(facts.Quota - reserved)
		}
	}
	info.PriceData.Quota = facts.Quota
	projectionCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := model.ReconcileTaskSubmissionCache(projectionCtx, req.SubmissionID); err != nil {
		common.SysLog(billingKind + " submission cache reconciliation pending: " + err.Error())
	}
	if err := model.DeliverPendingTaskAccountingLogs(projectionCtx, 100); err != nil {
		common.SysLog(billingKind + " submission log delivery pending: " + err.Error())
	}
	if session != nil && facts.Quota != 0 {
		if source == BillingSourceSubscription {
			checkAndSendSubscriptionQuotaNotify(info)
		} else {
			checkAndSendQuotaNotify(info, facts.Quota-reserved, reserved)
		}
	}
	return nil
}
