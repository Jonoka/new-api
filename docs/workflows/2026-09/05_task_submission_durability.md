# Durable async initial submissions

## Problem and scope

Async task and deferred image requests used to debit a live billing session
before contacting the provider, but created durable `TaskAccounting` ownership
only after the provider response was parsed. A process exit in that interval
could strand the funding/token reservation. A database commit whose result was
not observed could also leave a stale live session able to refund money already
transferred to the task.

The fix is deliberately task-specific. It applies when
`ForcePreConsume` or `DeferTaskBilling` is set and does not change ordinary text
request reservations or redesign the global wallet system.

## Identity and schema

Every logical submission has a random internal `submission_id`, a separate
random `lease_token`, and a fresh random `last_operation_id` for each
reservation transaction. These are independent from request IDs and public or
upstream task IDs.

The additive primary table `task_submissions` stores:

- version and lifecycle state: `active`, `transferred`, `settled`, `released`;
- lease owner and expiry;
- frozen user, wallet/subscription, subscription reset epoch and token IDs;
- nullable unique internal task row ID, including the pre-created Canvas task;
- frozen model evidence, current reserved quota and immutable accepted quota;
- last reservation operation identity and lifecycle timestamps;
- bounded release evidence and a retryable cache-invalidation marker.

There is no backfill or inference for historical tasks. A zero-value queued
Canvas/image task may initially have no funding source; the first paid
reservation establishes it. After that point all funding identities are
immutable.

## Reservation transaction

`model.GroupReservationRequest` carries the submission identity. The existing
funding/token transaction now locks or creates the active journal, checks its
lease token and expected amount, moves funding and token quota, and stores the
new amount plus `last_operation_id` before committing. Subscription records for
new journaled submissions use `submission_id` as their idempotency key.

Same-submission retry pricing may resize the amount up or down. A transaction
error is reconciled by `last_operation_id`; this also handles same-target
transactions and MySQL no-change affected-row behavior. A journal that cannot
be read is not guessed to have committed or rolled back.

`service.CreateQueuedTaskSubmission` inserts a Canvas/image `tasks` row and its
zero-value active journal in one transaction before the controller returns 202.
The executor imports that exact identity into `RelayInfo`.

## Lease and recovery

The lease lasts 45 seconds and live ownership refreshes every 10 seconds.
`StartTaskSubmissionHeartbeat` performs a synchronous database CAS before
starting the Canvas executor. Billing sessions start the same loop immediately
after the reservation transaction, whose journal write already proves and
renews ownership; this avoids a second fallible database step before the
session is published. The loop stops on context cancellation, explicit
completion, CAS loss, transfer, settlement or release. Transient database
errors do not spawn replacement goroutines.

The independent accounting recovery loop processes expired submissions before
terminal task decisions, cache reconciliation and log outboxes. Under a row
lock it rechecks expiry/state, refuses to refund a row that already has a
`TaskAccounting` owner, and releases only the journaled funding/token amount.
It does not change initial user/channel used counters because those counters are
created only at handoff. A known nonterminal Canvas/image task becomes FAILURE
in the same transaction. A generic submission without a task remains as a
released internal record, and recovery never contacts or retries the provider.

Subscription release uses the existing `reservation_reset_time` rule: an
old-period reservation is not credited into a newer reset period. Soft-deleted
tokens are adjusted through unscoped access but stay deleted; physically absent
tokens are not recreated. Cache invalidation is repeatable through
`cache_pending` and never replays money.

## Handoff and ambiguous commit

`service.HandoffTaskBilling` reconciles the accepted quota, persists the task,
creates `TaskAccounting`, initial counters and the initial log event, then marks
the submission `transferred`, all in one primary transaction.

If the caller observes a transaction error, it queries by the immutable
submission ID. Success is accepted only when journal state, lease token,
funding/user/subscription/token identities, accepted quota, task public
identity, and stored `TaskAccounting` owner all match. Exact replay creates no
second task, counter, event or money movement. A definite rollback leaves the
active reservation releasable. A stale live `Refund` can mutate only `active`;
`transferred` and `settled` are database-enforced no-ops.

A synchronous deferred image without an async task marks the journal
`settled`. A request failure marks it `released` in the same transaction as the
refund. If the database remains unavailable, the caller does not infer commit
outcome; the durable row is resolved by a later retry/recovery pass.

## Folded legacy batch reconciliation contract

Task reservation admission may fold pre-existing in-process wallet and token
batch deltas into the same SQL transaction. The journal therefore has one
additional additive nullable `TEXT` column,
`folded_batch_operation_ids`, containing a JSON array of random internal
operation UUIDs. `NULL` or an empty value means that no batch operation has
been folded. An ID is appended in the same transaction only when that
transaction actually folds a nonzero legacy batch delta. The array is
immutable history across later reservation resizes and terminal journal
states. It contains no quota values and no raw token keys.

The corresponding quota values remain in an in-process parked registry only
while a commit result is ambiguous. This does not make the pre-existing legacy
batch queue crash-durable: process loss retains the same volatility boundary as
the original queue. Within a live process, however, an extracted value is
handled as follows:

1. A Begin failure or transaction callback failure is a definite rollback.
   The extracted wallet/token values are restored immediately without querying
   the journal.
2. A successful transaction body followed by a Commit error is ambiguous. The
   extracted values are parked under the folded operation UUID before the
   journal is queried.
3. A readable journal containing that UUID proves commit, so the parked values
   are discarded. A readable journal without it, including a missing row,
   proves rollback, so the values are restored once. An unavailable read leaves
   them parked and returns an error.
4. Receipt lookup uses the immutable UUID array rather than
   `last_operation_id`, so later reservation updates, transfer, settlement, or
   release cannot erase proof of an earlier fold.

`RecoverPendingTaskSubmissionBatches(ctx, limit)` performs bounded independent
classification and is called by both the existing batch updater and task
submission recovery pass. Before a critical reservation extracts new pending
values, it also classifies every parked operation touching the same user or
token. If any such operation remains unreadable, admission stops; its value is
not omitted from the effective quota decision.

Wallet and token SQL batch application use separate process-wide apply locks.
Any path requiring both acquires `user quota apply` before `token quota apply`.
After those locks, code may briefly acquire a batch-store lock or the parked
registry lock, but never holds the registry lock across a database call and
never acquires an apply lock while holding a store/registry lock. Token flush
now retains the token apply lock from map detachment through SQL application,
so a task reservation cannot race a detached in-flight token delta.

The internal model surface added for this contract is:

```go
func RecoverPendingTaskSubmissionBatches(ctx context.Context, limit int) error
```

All newly admitted task reservation changes continue to execute directly in
the journal transaction and never enter either legacy batch queue.

## Compatibility and rollback

The migration is additive in normal and fast primary migration paths and uses
GORM-compatible scalar columns for SQLite, MySQL and PostgreSQL. Existing task
accounting and independent log receipts are unchanged.

Before starting an older image, stop new async submissions and require this
query to return zero:

```sql
SELECT COUNT(*)
FROM task_submissions
WHERE state = 'active';
```

Also retain the existing guards for unapplied task decisions, pending cache/log
delivery, and managed nonterminal tasks. An older image must not process tasks
linked from `transferred` rows. Do not drop the journal or restore a stale
database over later money movements.

## Verification

Dedicated GitHub Actions fixtures cover real initial reservation flow, live
lease exclusion, same-identity resize, one-time expired wallet release,
reset-epoch subscription release without a token, queued Canvas crash recovery,
expiry versus late handoff/refund, exact owner replay after a simulated
unobserved commit, no duplicate task/counters/log event/money, and definite
rollback followed by one refund. Pending-batch fixtures additionally use real
transactions with injected pre-commit rollback and post-commit errors, an
unavailable receipt read, later one-time classification after journal
advancement, admission blocking, and coordinated token flush/admission. Model
fixtures use SQLite and opt into PostgreSQL/MySQL through
`NEW_API_TEST_POSTGRES_DSN` and `NEW_API_TEST_MYSQL_DSN`.

No tests or builds are run on the production checkout. Formatting and diff
checks are local; executable verification belongs to GitHub Actions.
