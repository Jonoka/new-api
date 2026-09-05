package service

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// CompleteDeferredImageBilling completes the ordinary synchronous image path
// when the response contains image data and no asynchronous task identity.
func CompleteDeferredImageBilling(c *gin.Context, info *relaycommon.RelayInfo) {
	if info == nil || !info.DeferTaskBilling {
		return
	}
	info.DeferTaskBilling = false
	PostTextConsumeQuota(c, info, info.TaskBillingUsage, info.TaskBillingExtraContent)
	info.TaskBillingUsage = nil
	info.TaskBillingExtraContent = nil
}
