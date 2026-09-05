package common

import basecommon "github.com/QuantumNous/new-api/common"

const (
	ContextKeyDeferTaskBilling         = "defer_task_billing"
	ContextKeyTaskSubmissionID         = "task_submission_id"
	ContextKeyTaskSubmissionLeaseToken = "task_submission_lease_token"
	ContextKeyTaskSubmissionTaskRowID  = "task_submission_task_row_id"
)

// EnsureTaskSubmissionIdentity creates an internal identity unrelated to
// request IDs and public/upstream task IDs.
func (info *RelayInfo) EnsureTaskSubmissionIdentity() {
	if info == nil {
		return
	}
	if info.TaskSubmissionID != "" && info.TaskSubmissionLeaseToken != "" {
		return
	}
	info.TaskSubmissionID = basecommon.GetUUID()
	info.TaskSubmissionLeaseToken = basecommon.GetUUID()
}

type TaskBillingAttribution struct {
	Username          string
	TokenName         string
	RequestID         string
	UpstreamRequestID string
	IP                string
}
