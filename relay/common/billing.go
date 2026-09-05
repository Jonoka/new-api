package common

import "github.com/gin-gonic/gin"

// BillingSettler 抽象计费会话的生命周期操作。
// 由 service.BillingSession 实现，存储在 RelayInfo 上以避免循环引用。
type BillingSettler interface {
	// Settle 根据实际消耗额度进行结算，计算 delta = actualQuota - preConsumedQuota，
	// 同时调整资金来源（钱包/订阅）和令牌额度。
	Settle(actualQuota int) error

	// Refund atomically releases reserved funding and token quota. A failed
	// transaction leaves the live session retryable.
	Refund(c *gin.Context)

	// NeedsRefund 返回会话是否存在需要退还的预扣状态（未结算且未退款）。
	NeedsRefund() bool

	// GetPreConsumedQuota 返回实际预扣的额度值（信任用户可能为 0）。
	GetPreConsumedQuota() int

	// Reserve reconciles the live reservation to the target in either direction.
	// It does not settle or close the session.
	Reserve(targetQuota int) error
}
