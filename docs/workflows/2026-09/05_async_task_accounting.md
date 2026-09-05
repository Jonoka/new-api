# Durable async task accounting

## Goal and scope

Async task submission and terminal accounting now use durable ownership keyed by
the internal `tasks.id`. The change covers task submission, timeout, Suno and
generic polling, Gemini/Vertex direct fetch, Canvas image completion/handoff,
and direct async image submission. It does not repair historical terminal rows
or change the global wallet credit policy.

## Persistence contract

The primary database contains three additive records:

- `task_accountings` has exactly one row per managed `tasks.id`. It freezes the
  charged quota, funding source, user/subscription/token/channel identities,
  initial log facts, the first accepted terminal decision, and money/cache
  progress markers.
- `task_accounting_events` contains immutable initial-consumption and terminal
  adjustment projections. Each event has a random UUID and remains until it is
  delivered.
- `task_accounting_log_receipts` lives in `LOG_DB`. Its UUID primary key and
  transaction claim token deduplicate the corresponding `logs` insert even
  when the process stops after the log transaction commits but before the
  primary event is acknowledged.

JSON payload columns use `TEXT` and the repository JSON wrappers. Task response
data needed for restart is kept in the terminal decision. Accounting log facts
never contain token keys, channel credentials, provider response bodies, or raw
provider failure diagnostics.

All models are included in normal and fast primary migrations. The receipt is
also included in shared-primary and separate `LOG_DB` migrations. Migrations
are additive; completed records are retained because deleting a receipt while
its source event can replay would remove the deduplication guarantee.

The existing token `group_ratio_limits` TEXT field no longer declares an SQL
empty-string default, which MySQL rejects. Application inserts already write
its string value explicitly; reads preserve empty/no-limit behavior. This
portability correction changes the default metadata only and does not rewrite
stored token limits.

## Initial ownership handoff

`service.HandoffTaskBilling` holds the live `BillingSession`, then calls
`model.WithReconciledGroupReservation` and
`model.PersistAsyncTaskHandoffTx` in one primary transaction. That transaction:

1. reconciles the already durable reservation to the accepted charged quota;
2. inserts a new task or conditionally updates an existing Canvas task;
3. creates accounting ownership keyed by `tasks.id`;
4. increments user/channel used quota and user request count once;
5. creates the immutable initial log event.

The session is closed only after commit. Failure leaves the previous live
reservation refundable and does not create a task owner. Task-shaped image
requests set `DeferTaskBilling`, so the normal synchronous image settlement and
log path does not duplicate this handoff.

The shared image submit wrapper also receives ordinary synchronous image
responses. It freezes the actual charge, usage and log facts once. When the
response has no task ID, a settlement callback commits funding/token changes,
user/channel counters, the submission's `settled` state, and the log event in
one transaction. Failure returns a gateway error without publishing the image;
the active reservation remains releasable. Ordinary synchronous images retain
configured wallet trust; queued tasks and tasks-endpoint channels require full
admission.

Tasks-endpoint image admission retains `QuotaToPreConsume` when tiered pricing
has not assigned a final `Quota` yet. A queued submit response with a returned
task identity is accepted without requiring deliverable image data; the same
data validation still applies to ordinary synchronous image responses.

Synchronous image log events use `task_row_id = 0` and their indexed
`submission_id`, because no public task is created. They retain ordinary usage,
model/group, caller and pricing fields. Receipt delivery remains idempotent;
the existing optional quota-data export remains best effort and runs only for
a newly inserted consume log. Request/attempt metrics finalize after accounting,
and settlement failures are gateway failures rather than channel penalties.
Without Redis, user-quota helpers skip cache workers and performance samples
update their in-memory counters before returning, so no no-op background work
retains request/configuration state after completion.
Canvas captures username, token name, request IDs and privacy-permitted IP
before its Gin context ends so background consume/refund events retain those
facts without retaining the request credentials.

## Terminal decision and application

`service.FinalizeTaskAccounting` uses two durable primary transactions:

1. `model.AcceptTaskTerminalDecision` accepts the first SUCCESS or FAILURE
   decision under the expected nonterminal status. It stores the desired task
   fields and final quota but leaves the public task status and charged quota
   unchanged.
2. `model.ApplyTaskAccountingDecision` atomically applies
   `delta = final_quota - charged_quota` to the selected wallet or subscription,
   token balances, user/channel used quota, final task quota/status, and the
   money-applied marker. Terminal deltas never increment request count.

An explicit final quota of zero is valid and produces a full refund. The legacy
adapter result `AdjustBillingOnComplete(...) == 0` still means no adjustment;
callers that know an actual zero pass it directly to the terminal owner.
Token-based recalculation uses only the frozen task billing context, never
current mutable model or group ratios.

Nonterminal `Task.Update` and `Task.UpdateWithStatus` reject writes after a
pending decision exists. Unfinished-task queries also exclude pending
decisions. Recovery can therefore complete money application without another
provider request and without exposing an unpaid terminal result.

Wallet terminal corrections retain the existing post-use behavior and may
enter arrears. Subscription corrections retain the existing subscription
bounds/reset behavior. Soft-deleted tokens are adjusted with `Unscoped` while
remaining deleted; a physically absent token is confirmed as absent and is not
recreated. User, channel, and required funding lookup errors abort the whole
application transaction.

## Recovery and logs

`service.StartTaskAccountingRecovery` starts after primary and log databases
initialize and runs independently of provider polling configuration. Each
bounded pass:

1. applies accepted decisions without a money marker;
2. retries repeatable user/token cache invalidation;
3. projects ready log events through the independent receipt transaction.

Redis is a derived projection. Recovery retries deletion/invalidation and never
replays quota increments. A cache or separate log database failure does not
roll back committed money. Multiple nodes may run recovery because database
ownership and UUID receipts provide idempotency.

## Compatibility and rollback

Existing nonterminal tasks without `task_accountings` are not automatically
adopted because their durable initial debit cannot be proven. The explicit old
timeout cutoff remains a no-refund legacy path. Missing accounting on a newer
charged task is surfaced and the public row remains nonterminal for review.

An older image can ignore the additive tables but cannot safely process B-created
in-flight tasks. Before code rollback, stop new async submissions, drain all
rows with a decision and `money_applied = false`, reconcile every ready
undelivered event and pending cache marker, and verify no managed task is still
nonterminal. Do not restore a stale database over later money movements.

## Verification

Focused fixtures accept `NEW_API_TEST_POSTGRES_DSN` and
`NEW_API_TEST_MYSQL_DSN`; SQLite always runs. They cover real initial handoff,
full/partial/zero and duplicate settlement, first-decision ownership, pending
decision restart, transaction rollback/recovery, independent log DB delivery,
request-count invariance, and frozen pricing. All Go tests and builds run only
in GitHub Actions under this repository's production-host restriction.
