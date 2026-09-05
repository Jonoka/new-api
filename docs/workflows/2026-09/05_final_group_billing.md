# Final-group billing reservation

## Goal and scope

Cross-group retries must price and admit the selected group before sending that
attempt upstream. This change is limited to synchronous relay and task-submit
reservation handling. It does not change post-use arrears policy, task terminal
settlement, retry eligibility, channel locking, or the rule that output already
sent to a client cannot be retried.

## Processing contract

1. Parse and estimate the request once. Billing-expression source, request body,
   request headers, prompt estimate, and configured output estimate become
   request facts and remain frozen across retries.
2. Select the channel and effective group for an attempt.
3. Recompute group-dependent `PriceData` and the tiered snapshot from the frozen
   request facts. Model identity and media/task additional ratios are preserved.
4. Resize the live reservation to the selected group's required target. A resize
   may increase, decrease, reach zero, or be a no-op for the same group.
5. Send the upstream request only after the resize succeeds. A failed increase
   leaves the previous reservation intact and stops the paid attempt.
6. Final settlement and logs use the last admitted group's price data, group,
   tiered snapshot, and reservation amount.

Free initial pricing creates no funding session. A later paid group establishes
the funding source through the normal user preference (`subscription_first`,
`wallet_first`, or the corresponding exclusive mode). Once selected, that
source remains attached to the live session while the reservation is resized.

## Trust and reservation targets

Trust is the existing wallet policy evaluated through `GetTrustQuota`: its
threshold must be positive, the token must be unlimited or above that threshold,
the wallet balance must be above it, and `ForcePreConsume` must be false. A session that
meets those checks has reservation target zero for every paid retry and settles
the actual usage afterward. Subscription sessions and asynchronous tasks never
use the trust bypass. A free group by itself is not trust: because it creates no
session, a later paid attempt must perform normal funding selection and trust or
balance admission before the upstream send.

## State and atomicity

Group resize is separate from final settlement and refund, so lowering a target
does not close the session. Wallet or subscription reservation and token quota
move in one primary-database transaction. Pending batched quota deltas for the
same user/token are folded into that transaction once; on rollback they are
restored to their batch stores. After commit, user and token caches are
invalidated so the database-backed result becomes authoritative.

For subscription funding, the existing request-scoped pre-consume record is
resized with the subscription and token rows. This keeps later refund behavior
consistent with the live reservation. An additive nullable
`reservation_reset_time` column stores the subscription reset epoch for new
reservations. When maintenance resets the period, a live retry establishes its
complete target in the new period. Releasing an expired reservation does not
credit unrelated new-period usage. Existing records with no epoch retain the
legacy clamping policy; no historical balances are inferred or rewritten.

Final settlement and failed-request refund also commit funding and token changes
together. The session closes only after commit, so a failed transaction leaves
the live reservation intact. Post-use settlement preserves wallet/token arrears
policy; pre-attempt admission always checks available quota. Refund now runs
synchronously and releases the current target once.

## Compatibility and safety boundaries

- Uses GORM transactions and row locks, with SQLite's existing transaction
  behavior and `FOR UPDATE` on MySQL/PostgreSQL.
- The nullable subscription reservation epoch column is additive and is migrated
  by the existing subscription model migrations. No new configuration, public
  API fields, or external dependencies.
- Repeated startup migration is idempotent. An older binary ignores the nullable
  column; rollback does not drop it. In-memory relay sessions do not survive a
  process cutover, so deployments must drain active requests rather than expect
  either version to resume a live group reservation.
- Pricing errors, quota overflow, insufficient wallet/subscription/token quota,
  and transactional adjustment failures are admission failures.
- Same-group retries produce a zero delta and no duplicate pre-consume.
- Locked task channels retain their current retry policy and group.

## CI test plan

GitHub-hosted tests must cover real database/session flows for free-to-paid,
paid-to-free, paid-to-paid, same-group, all-failed, insufficient additional
funds, wallet, subscription, no-token, trust, rollback, and final snapshot/log
identity. Pricing fixtures cover token ratio, fixed price, tiered expressions,
task media/additional ratios, and the assertion that admission failure performs
no upstream send. Database fixtures run on SQLite and, when their CI DSNs are
provided, MySQL and PostgreSQL. No source build or executable test is run on the
production VPS.
