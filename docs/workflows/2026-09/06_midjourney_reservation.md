# Durable Midjourney reservation and terminal accounting

## Goal and scope

Legacy Midjourney submit and swap requests must reserve wallet and token quota in
the authoritative primary database before contacting a provider. Accepted async
work must transfer that reservation to the existing durable task-accounting owner,
and terminal failure must refund wallet and token exactly once before the public
Midjourney row becomes terminal.

This change preserves Midjourney's wallet-only funding policy, selected group and
channel routing, per-call price, request and response bodies, and upstream/public
Midjourney task ID. It does not add subscription fallback, change trust policy,
repair historical rows, or charge the explicitly free in-paint and custom-zoom
actions.

## Submission contract

1. Resolve the existing Midjourney model/action and final selected-group price.
2. For a charged action, create a random internal submission identity, force full
   pre-consumption, and reserve wallet plus finite token quota in one existing
   group-reservation transaction. Database rejection stops before provider I/O.
3. Send the unchanged request to the selected Midjourney provider.
4. Treat submit response codes 1, 21 and 22, and swap response code 1, as accepted.
   A transport/HTTP/provider rejection releases the live submission owner. Release
   is journal-idempotent; if immediate release cannot be confirmed, normal
   submission recovery completes it without provider I/O.
5. For accepted charged work, a first primary transaction creates the legacy
   `midjourneys` row and internal `tasks` row, sets `midjourneys.task_row_id`, and
   attaches that task row to the active submission journal. The unchanged
   `HandoffTaskBilling` transaction then creates `task_accountings` ownership and
   the initial log event and transfers that exact journal. The public response is
   written only after both durable steps succeed.

If the process stops between those transactions, the active journal already names
the internal task. Standard submission recovery releases wallet/token quota and
marks that task failed in the same transaction; the Midjourney poller can then
project the released terminal state without provider I/O. An observed link-commit
error is accepted only when the exact Midjourney row, internal task and active
submission association are readable and match.

Free actions retain their prior unjournaled and uncharged behavior. Provider
rejections retain the legacy Midjourney response schema and do not receive a task
accounting owner or consumption log. Newly created free and rejected rows persist
zero quota so the legacy poller cannot later refund money that this request never
debited.

## Terminal and restart contract

The internal task uses platform `mj` and stores the upstream Midjourney ID only as
private provider metadata. The nullable `midjourneys.task_row_id` is an internal
database association and is omitted from JSON responses.

The Midjourney poller is the sole provider-polling owner for these rows. Generic
task polling, generic timeout sweeping and generic task lists exclude platform
`mj`; users continue to read these requests through the legacy Midjourney APIs.
When a provider reports success or failure, the poller freezes the provider
projection in the internal task decision and calls `FinalizeTaskAccounting` with
the charged quota for success or zero for failure. B's accounting transaction
applies funding, finite-token quota, user/channel used counters and the terminal
decision once. Only then does the poller project terminal fields to the legacy
Midjourney row. If the selected channel can no longer be loaded, the same linked
owner is finalized as failed before the public failure is projected.

If the process stops after accounting but before public projection, the next
Midjourney pass reads the terminal internal task and projects its frozen data
without contacting the provider. Concurrent or duplicate terminal observations
reuse the first durable decision and a conditional legacy-row update.

Rows whose `task_row_id` is null are historical/legacy rows. They keep the
pre-existing failure behavior: the poller first wins the legacy public-status CAS,
then credits the stored quota to that user's wallet and writes the legacy refund
log. It does not infer a token refund, create a task-accounting owner or backfill a
relationship. This compatibility path is not atomic with the status CAS: a stop or
database failure after the status update can still strand the wallet refund, and
the row alone cannot prove whether an old free/rejected request was originally
debited. Those are pre-existing limitations, not properties of the linked B flow.
New linked rows instead apply wallet, finite-token, used counters and terminal
state atomically and idempotently through task accounting.

## Schema, compatibility and rollback

`midjourneys.task_row_id` is an additive nullable indexed integer column migrated
through the existing `Midjourney` AutoMigrate registration. The implementation
uses GORM transactions and scalar columns supported by SQLite, MySQL 5.7.8+ and
PostgreSQL 9.6+. No backfill runs and no historical relationship is inferred.

Before cutover, operators should drain or at least inventory active unlinked rows
because their old wallet-only refund cannot provide linked accounting guarantees.
An undrained row is not silently disabled: it continues through the compatibility
routine above. The cutover performs no historical balance repair or data mutation.

An older binary ignores the nullable link but cannot safely own polling for linked
nonterminal rows because it would bypass canonical token/accounting settlement.
Before rollback, stop submissions and polling, drain active task submissions,
linked nonterminal Midjourney rows, unapplied task decisions, pending cache markers
and undelivered accounting events. Do not drop the column or restore a stale
database over later money movements.

## Verification

GitHub-hosted tests cover exact K-of-N provider sends for wallet and token limits,
insufficient admission, provider rejection release, accepted-code compatibility,
accepted handoff/link, duplicate terminal failure, restart projection, legacy
wallet-only refund compatibility, internal task privacy, and free actions. Database
matrix fixtures use SQLite plus configured MySQL/PostgreSQL DSNs. No source tests,
builds, provider calls or fault injection run on the production checkout.

Local source verification uses the repository Go version's pinned `gofmt` and
`git diff --check`. Executable verification must run in GitHub Actions with:

```text
go test ./relay ./controller ./service
go test ./model
go test -race ./relay ./controller ./service ./model
```
