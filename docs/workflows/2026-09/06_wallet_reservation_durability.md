# Durable wallet reservations for synchronous sessions

## Goal and scope

Every live `BillingSession`, including an ordinary synchronous request and a
wallet request admitted by the configured trust threshold, owns one durable
`task_submissions` journal. The journal is the receipt for initial admission,
retry-group resizing, final settlement and refund. This extends the existing B
reservation owner; it does not add a second debit path or change funding order,
subscription reset epochs, token exemptions, arrears policy, task handoff,
deferred image settlement or Alpha Search accounting.

## Admission and trust

The primary database is authoritative for trust. A wallet session receives the
existing zero-reservation trust bypass only when the configured threshold is
positive, `ForcePreConsume` is false, the locked wallet balance is above the
threshold, and the locked token is unlimited or above the same threshold.
Cached wallet or token balances never grant trust. Subscription
sessions remain ineligible.

Trusted admissions and zero-valued admissions with a live `BillingSession`
create a zero-value active journal before an upstream attempt. An attempt that
the controller declares free and intentionally gives no session remains outside
the money lifecycle. Non-trusted paid admission moves wallet and token quota in the
same locked transaction that records the exact expected and target reservation.
A rejected transaction cannot publish a live session or reach upstream.

## Operation reconciliation and recovery

Each monetary operation has a fresh random operation ID. A reported commit
error is resolved only by reading the same journal and matching the operation,
lease, funding/token identities, expected amount, target amount and requested
final state. A match proves the commit; an exact prior state proves rollback;
an unavailable or contradictory receipt remains an error and is never guessed.

Refund always reconciles the journal's current reservation to zero, so retry,
concurrent closure and restart cannot duplicate a credit. Expired active rows
are released by the existing B recovery transaction and retain `cache_pending`
until both derived cache projections are invalidated. Recovery never contacts a
provider and cannot reconstruct usage that was not observed before a process
failure.

Ordinary Text, Audio and Realtime handlers retain their established behavior of
publishing measured usage counters and consume logs when post-use settlement
reports an error. For a positive known actual quota, the journal moves to
`settlement_pending`, preserving the durable reservation and measured target.
Abandoned recovery never refunds this state. This avoids turning known successful
usage into a full refund, while deliberately not replaying the adjustment because
the legacy counters/log are not transactionally owned by this journal. These rows
require explicit reconciliation. If the database is unavailable for both the
settlement and this preservation write, the row can remain `active`; subsequent
abandoned recovery remains an operational risk in that narrower failure window.

Closed ordinary rows may be removed only after a bounded retention period and
only when they are not active or transferred, have no task link, no pending
cache marker, no pending folded-batch receipt and no accounting/log dependency.
Retention is evidence cleanup, never a balance operation.

## Realtime and violation fees

Realtime `response.done` admission resizes the same live session to the
cumulative completed-response cost using the same frozen group/model snapshot
and tiered expression as final settlement. A failed required increase stops the
cycle before another upstream exchange, while final post-use settlement keeps
the existing arrears behavior. The final aggregate is not charged a second
time; it reconciles the cumulative reservation to the actual total.

The existing violation trigger and tariff remain unchanged. Its wallet/token
movement, counters and log are committed synchronously under a distinct durable
journal operation. Replaying the same fee owner is idempotent and never retries
the upstream request. The fee remains a post-use adjustment and may enter
arrears under the existing policy.

## Compatibility and verification

No public billing identity or existing service signature changes. Generic
ordinary journals have no `task_row_id`; async handoff continues to transfer
the same row to `TaskAccounting`. The journal adds portable scalar
`last_expected_quota`, `last_target_quota` and `last_final_state` columns beside
`last_operation_id`; normal and fast GORM migrations add them on SQLite, MySQL
and PostgreSQL. Older images ignore these columns. Before rollback, drain active
ordinary/task journals, `settlement_pending` rows and pending cache/log work
because older code cannot classify D operations from the new exact receipt
fields.

GitHub-hosted tests cover ordinary and trusted zero journals, stale-cache trust,
required rejection, ambiguous initial/resize/settlement/refund commits, expired
restart release, known-settlement retention, cumulative Realtime reservation,
frozen tiered Realtime pricing and idempotent violation fees. They assert
authoritative wallet/token balances, journal state, counters and logs. No
executable tests or builds run on the production checkout.
