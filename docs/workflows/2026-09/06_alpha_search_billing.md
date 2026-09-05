# Alpha Search billing

## Scope

`POST /v1/alpha/search` has a search-only accounting path. It charges exactly one
model-specific configured `web_search_preview` call after a validated synchronous
result. Model token ratios, fixed request prices, and tiered-expression base costs
do not participate. Other request multipliers already validated by the request
DTO still apply to the tool cost.

## Attempt lifecycle

The controller selects a channel and final group before calling
`service.AdmitAlphaSearchBilling`. Admission resolves the configured tool price
against the original public model, applies the effective final-group ratio and
request multipliers, and reconciles the B billing reservation before upstream
I/O. Every retry calls admission again. Same-group admission is idempotent, and a
cross-group retry resizes the existing reservation to the new target.

An upstream or validation failure does not create usage counters or a consume
log. The controller's existing final error owner releases the live reservation
once. A failed reservation increase stops before sending that attempt and leaves
the prior reservation available for the final release.

The Alpha handler buffers and validates the complete provider success, then calls
`service.SettleAlphaSearchBilling` before writing response headers or bytes.
Settlement commits the funding adjustment, token used/remain quota, user used
quota and request count, channel used quota, durable submission state, and log
outbox record in one primary-database transaction. A settlement error is returned
to the handler and cannot be reported as client success. Ambiguous commits use
the B submission receipt to distinguish a committed settlement from an open
reservation.

## Free and zero-token behavior

A zero tool price or zero effective group ratio remains a successful request. It
creates a durable zero-value billing journal so request count and a zero-quota
consume log still commit exactly once. Subscription funding does not create or
charge a subscription pre-consume row for such a request. A later paid retry may
resize the same zero journal and select the configured funding source normally.

The Alpha log records zero prompt and completion tokens, the original model, final
mapped model, selected channel/group, one `web_search_preview` call, configured
price, applied multipliers, and final quota. No synthetic token usage is created.
The current usage-log fields `web_search`, `web_search_call_count`, and
`web_search_price` remain the display contract.

## Compatibility and verification

The implementation reuses GORM transactions and the B task-submission/log-outbox
tables, so it does not add a migration or database-specific SQL. SQLite is covered
by the service accounting fixture; MySQL and PostgreSQL use the existing optional
CI DSNs. Focused CI tests cover model-specific and free prices, group and request
multipliers, cross-group resize, final failure refund, settlement rollback,
zero-token counters, log facts, and exactly-once settlement. Existing Responses
and other text billing continue through `PostTextConsumeQuota` unchanged.

All builds and tests run in GitHub-hosted Actions. The source worktree is formatted
and inspected locally only. Real gateway accounting remains a separately approved
bounded canary and is not established by fixture results.
