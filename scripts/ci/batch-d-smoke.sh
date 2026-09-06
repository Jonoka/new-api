#!/usr/bin/env bash
set -euo pipefail

test "${GITHUB_ACTIONS:-}" = true
test -n "${CANDIDATE_IMAGE:-}"
test -n "${PREVIOUS_IMAGE:-}"
test -n "${TEST_POSTGRES_CONTAINER:-}"
test -n "${TEST_REDIS_CONTAINER:-}"
cleanup() { docker rm -f newapi-batch-d-a newapi-batch-d-b >/dev/null 2>&1 || true; }
trap cleanup EXIT
query() {
  docker exec "$TEST_POSTGRES_CONTAINER" psql -XAt -v ON_ERROR_STOP=1 -U postgres -d newapi_candidate_smoke -c "$1"
}
diagnose() {
  docker logs newapi-batch-d-a 2>&1 | tail -60 || true
  docker logs newapi-batch-d-b 2>&1 | tail -60 || true
  query "SELECT username,quota,used_quota,request_count FROM users WHERE username LIKE 'd-wallet-%' ORDER BY username"
  query "SELECT name,remain_quota,used_quota FROM tokens WHERE name LIKE 'd-wallet-%' ORDER BY name"
}
trap diagnose ERR
wait_ready() {
  for attempt in $(seq 1 90); do
    if curl -fsS --max-time 2 "http://127.0.0.1:$1/api/status" >/dev/null; then return; fi
    sleep 2
  done
  return 1
}
start() {
  docker run -d --name "$1" --network host \
    -e PORT="$2" \
    -e SQL_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
    -e REDIS_CONN_STRING=redis://127.0.0.1:6379/13 \
    -e SESSION_SECRET=d-ci-only-session -e CRYPTO_SECRET=d-ci-crypto-only \
    -e BATCH_UPDATE_ENABLED=true -e BATCH_UPDATE_INTERVAL=1 -e SYNC_FREQUENCY=3600 \
    -e NO_PROXY=127.0.0.1,127.0.0.2,localhost -e UPDATE_TASK=false \
    "$3" >/dev/null
  wait_ready "$2"
}

docker exec "$TEST_REDIS_CONTAINER" redis-cli -n 13 FLUSHDB >/dev/null
start newapi-batch-d-a 38001 "${CANDIDATE_IMAGE,,}"
start newapi-batch-d-b 38002 "${CANDIDATE_IMAGE,,}"
NEW_API_TEST_COMPATIBILITY_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
NEW_API_TEST_REDIS_ADDR=127.0.0.1:6379 \
  go test ./model -run '^TestWalletConcurrencyCompatibilityStaleCache$' -count=1
timeout 240s node scripts/ci/batch-d-gateway.mjs
test "$(query "SELECT count(*) FROM task_submissions WHERE state='active' OR cache_pending")" = 0
test "$(query "SELECT count(*) FROM task_accountings WHERE NOT money_applied OR cache_pending")" = 0
test "$(query "SELECT count(*) FROM task_accounting_events WHERE NOT delivered")" = 0
test "$(query "SELECT count(*) FROM balance_cache_repairs WHERE repaired_at=0")" = 0
snapshot="$(query "SELECT username,quota,used_quota,request_count FROM users WHERE username LIKE 'd-wallet-%' ORDER BY username")"
token_snapshot="$(query "SELECT name,remain_quota,used_quota FROM tokens WHERE name LIKE 'd-wallet-%' ORDER BY name")"
docker logs newapi-batch-d-a > "$RUNNER_TEMP/batch-d-recovery.log" 2>&1
docker logs newapi-batch-d-b > "$RUNNER_TEMP/batch-d-second-process.log" 2>&1
cleanup
docker pull "${PREVIOUS_IMAGE,,}"
start newapi-batch-d-a 38001 "${PREVIOUS_IMAGE,,}"
test "$(query "SELECT username,quota,used_quota,request_count FROM users WHERE username LIKE 'd-wallet-%' ORDER BY username")" = "$snapshot"
test "$(query "SELECT name,remain_quota,used_quota FROM tokens WHERE name LIKE 'd-wallet-%' ORDER BY name")" = "$token_snapshot"
test "$(docker inspect --format '{{.State.Status}}/{{.RestartCount}}' newapi-batch-d-a)" = running/0
docker logs newapi-batch-d-a > "$RUNNER_TEMP/batch-d-previous-image.log" 2>&1
printf 'D multi-process admission, ordinary restart recovery and drained C rollback passed.\n'
