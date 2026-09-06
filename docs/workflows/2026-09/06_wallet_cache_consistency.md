# Wallet balance cache consistency

## Goal and scope

User wallet quota and token remaining/used quota are authoritative in the
primary database. Redis contains derived request-time projections only. This
change removes new monetary writes from the process-local batch queues and
closes stale-fill, rejected-write, cache-outage, and restart gaps without
changing trust, credit, arrears, subscription, or settlement policy.

Statistical counters such as user/channel used quota and request count remain
eligible for batching. The old user/token balance stores and their lock order
remain only to drain values already present during a rolling upgrade and to
support the Batch B folded-delta compatibility contract.

## Direct balance mutation contract

The existing public user and token quota helper signatures remain unchanged.
Every newly requested balance change now:

1. starts a primary-database transaction;
2. proves that the user or token row exists and requires exactly one affected
   row for a nonzero change;
3. updates the authoritative balance and inserts a cache-repair marker in the
   same transaction;
4. classifies a commit error by the exact random marker ID rather than guessing;
5. invalidates the derived Redis projection only after a known commit; and
6. acknowledges only the marker state that was observed and successfully
   invalidated.

A rejected or rolled-back database write never changes Redis. Token writes
update `remain_quota` and `used_quota` together with their existing signs.
Zero-delta calls still prove row existence but do not create repair work.

## Projection repair schema

`balance_cache_repairs` is an additive primary-database table. Each row has a
random operation ID primary key, schema version, optional user ID, optional
HMAC-derived token cache key, creation time, and `repaired_at`
acknowledgement. Exactly one projection target is present. The table contains no
quota value, balance, token secret, funding choice, or replay instruction, so it
is not a wallet ledger.

The token target is the already-derived cache key digest rather than the raw
token. This lets recovery delete the exact projection even if the token is
renamed or deleted later, without persisting credentials. The model is included
in normal and fast primary migrations and uses portable GORM scalar columns for
SQLite, MySQL, and PostgreSQL.

## Recovery and acknowledgement

An immediate post-commit repair first bumps the appropriate cache generation
and deletes the derived hash. If Redis is unavailable, the committed marker is
left pending. The independent task-accounting recovery loop also runs a bounded
`RecoverBalanceCacheRepairs` pass, so restart and cache recovery converge
without replaying any balance change.

After successful invalidation, recovery updates `repaired_at` for the exact
marker ID, schema version, and projection target it read. A concurrent or newer
marker is not acknowledged. A crash after invalidation but before
acknowledgement causes another harmless invalidation. A database failure during
acknowledgement also leaves
the marker retryable. Acknowledged rows remain as commit receipts so another
process can still classify a lost commit reply; recovery selects only pending
rows and does not invalidate acknowledged projections again.

## Cache refill rules

Full user and token snapshots retain their generation fences. A quota-only user
cache miss now returns the database value without scheduling an unfenced scalar
write into a potentially newer full hash. Direct balance helpers invalidate
rather than increment cached balances, so Redis cannot get ahead of a rejected
database write.

## Legacy batch compatibility

No new user/token balance mutation enters the financial batch maps, regardless
of `BATCH_UPDATE_ENABLED`. Flush support remains for values already queued by
an older in-process caller during rolling transition. A definite SQL failure
restores the detached value. A commit error is classified by its exact durable
repair marker; an unreadable result is parked in process and is resolved before
the value can be restored or discarded. Batch B folded-delta receipt handling
and the user-before-token apply-lock order remain unchanged.

The compatibility maps are still process-local historical state; this change
does not invent a second durable wallet queue or claim to repair a value lost by
terminating an older process before it hands that value to the new code.

## Known limit

Acknowledged repair receipts are retained because an unresolved legacy flush in
another process may still need exact commit evidence. Batch D does not add a
pruner: storage grows with direct balance-helper mutations. A later retention
policy would first need a durable proof that no live process can still classify
the receipt; age alone is not such proof.

## Verification and rollback

GitHub-hosted tests cover SQLite and optional MySQL/PostgreSQL balance helpers,
missing rows, rejected writes, batching enabled, legacy flush failure and
commit classification, real Redis stale-fill ordering, cache failure/recovery,
and restart-style repair passes. No executable tests or builds run on the
production checkout.

Rollback must first drain pending (`repaired_at = 0`) `balance_cache_repairs`,
existing Batch B task cache markers, and parked folded batch operations. An
older image ignores the additive repair table but cannot perform its recovery,
so rolling back with
pending rows may leave stale Redis projections until TTL or another explicit
invalidation.
