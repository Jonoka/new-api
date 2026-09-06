# Wallet Reservation Concurrency

Batch D closes required-admission gaps on the existing B/C accounting baseline.
The primary database decides whether wallet and finite-token funds cover the
selected attempt. Configured wallet trust remains an exemption using the same
threshold and token rules; a stale Redis balance cannot grant that exemption.
Post-use settlement keeps its existing arrears policy.

## Ownership

Ordinary billing sessions reuse the existing task_submissions operation/lease
journal for create, resize, settle and release. Task/image/Alpha ownership remains
unchanged. Redis is a projection repaired after committed money, never a second
reservation authority. Financial balance changes must not be acknowledged only
in volatile batch maps. Statistical counters may retain batching.

Direct balance writes record a `balance_cache_repairs` projection marker in their
transaction. Successful invalidation sets `repaired_at`; the record remains as
commit evidence so another process cannot erase proof before a lost reply is
classified. Pending repair means `repaired_at = 0`, not every retained row.

Realtime cumulative completed usage belongs to the same reservation that final
settlement closes. Charged Midjourney submit/swap must reserve before provider I/O
and hand accepted work to durable task accounting; explicitly free actions and
legacy wallet-only funding are preserved. Historical unlinked Midjourney failures
retain their old status-CAS then wallet-credit routine without inferred token
refunds or new owners; new free/rejected rows store zero refundable quota. This old
routine is not status/refund atomic and is not a substitute for linked accounting.
Violation fees keep their existing trigger/tariff and post-use policy.

## Verification

All executable verification runs in GitHub-hosted Actions. The D image rehearsal
uses two candidate processes sharing PostgreSQL and Redis, with a local mock
gateway on a different loopback host from the client. It covers wallet-limited
and token-limited K-of-N admission, stale-high cache trust, configured true trust,
failed upstream refund and abandoned ordinary reservations across process restart.
Assertions cover exact upstream-call counts, authoritative balances, used quota,
request count, logs and durable owner state. Unit/integration fixtures inject
commit replies, database rollback and Redis failures on all supported databases.

Rollback to C requires drained active or settlement_pending reservations, managed tasks, pending cache
repair and undelivered accounting events. No image rollback reverses committed
financial movements. Production operation and real paid probes require their
separately reviewed candidate and authorization.
