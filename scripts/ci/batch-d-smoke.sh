#!/usr/bin/env bash
set -euo pipefail

test "${GITHUB_ACTIONS:-}" = true
test -n "${CANDIDATE_IMAGE:-}"
test -n "${PREVIOUS_IMAGE:-}"
test -n "${PRODUCTION_IMAGE:-}"
test -n "${PRODUCTION_REVISION:-}"
test -n "${TEST_POSTGRES_CONTAINER:-}"
test -n "${TEST_REDIS_CONTAINER:-}"
cleanup() { docker rm -f newapi-batch-d-a newapi-batch-d-b >/dev/null 2>&1 || true; }
trap cleanup EXIT
query() {
  docker exec "$TEST_POSTGRES_CONTAINER" psql -XAt -v ON_ERROR_STOP=1 -U postgres -d "${2:-newapi_candidate_smoke}" -c "$1"
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
    if curl -fsS --max-time 2 "http://127.0.0.1:$1/api/status" >/dev/null &&
      [ "$(docker inspect --format '{{.State.Health.Status}}' "$2")" = healthy ]; then return; fi
    sleep 2
  done
  return 1
}
start() {
  docker run -d --name "$1" --network host \
    --health-cmd "wget -q -O - http://127.0.0.1:$2/api/status | grep -Eq '\"success\"[[:space:]]*:[[:space:]]*true'" \
    --health-interval 5s --health-timeout 10s --health-retries 3 \
    -e PORT="$2" \
    -e "SQL_DSN=postgres://postgres:postgres@127.0.0.1:5432/${4:-newapi_candidate_smoke}?sslmode=disable" \
    -e REDIS_CONN_STRING=redis://127.0.0.1:6379/13 \
    -e SESSION_SECRET=d-ci-only-session -e CRYPTO_SECRET=d-ci-crypto-only \
    -e BATCH_UPDATE_ENABLED=true -e BATCH_UPDATE_INTERVAL=1 -e SYNC_FREQUENCY=3600 \
    -e NO_PROXY=127.0.0.1,127.0.0.2,localhost -e UPDATE_TASK=false \
    "$3" >/dev/null
  wait_ready "$2" "$1"
}

docker exec "$TEST_REDIS_CONTAINER" redis-cli -n 13 FLUSHDB >/dev/null
NEW_API_TEST_COMPATIBILITY_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
NEW_API_TEST_REDIS_ADDR=127.0.0.1:6379 \
  go test ./model -run '^TestWalletConcurrencyCompatibilityCacheOutage$' -count=1
start newapi-batch-d-a 38001 "${CANDIDATE_IMAGE,,}"
start newapi-batch-d-b 38002 "${CANDIDATE_IMAGE,,}"
NEW_API_TEST_COMPATIBILITY_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
NEW_API_TEST_REDIS_ADDR=127.0.0.1:6379 \
  go test ./model -run '^TestWalletConcurrencyCompatibilityStaleCache$' -count=1
timeout 240s node scripts/ci/batch-d-gateway.mjs
test "$(query "SELECT count(*) FROM task_submissions WHERE state IN ('active','settlement_pending') OR cache_pending")" = 0
test "$(query "SELECT count(*) FROM task_accountings WHERE NOT money_applied OR cache_pending")" = 0
test "$(query "SELECT count(*) FROM tasks t JOIN task_accountings a ON a.task_row_id=t.id WHERE t.status NOT IN ('SUCCESS','FAILURE')")" = 0
test "$(query "SELECT count(*) FROM task_accounting_events WHERE NOT delivered")" = 0
test "$(query "SELECT count(*) FROM balance_cache_repairs WHERE repaired_at=0")" = 0
financial_snapshot() {
  query "SELECT json_build_array('user',id,quota,used_quota,request_count) FROM users ORDER BY id;
    SELECT json_build_array('token',id,remain_quota,used_quota) FROM tokens ORDER BY id;
    SELECT json_build_array('channel',id,used_quota) FROM channels ORDER BY id;
    SELECT json_build_array('subscription',id,amount_total,amount_used) FROM user_subscriptions ORDER BY id;
    SELECT json_build_array('task',id,status,quota) FROM tasks ORDER BY id;
    SELECT json_build_array('log',id,type,quota) FROM logs ORDER BY id;
    SELECT row_to_json(s) FROM task_submissions s ORDER BY submission_id;
    SELECT row_to_json(a) FROM task_accountings a ORDER BY task_row_id;
    SELECT row_to_json(e) FROM task_accounting_events e ORDER BY event_id;
    SELECT row_to_json(l) FROM task_accounting_log_receipts l ORDER BY event_id;
    SELECT row_to_json(r) FROM balance_cache_repairs r ORDER BY id;" "$1" | sha256sum | cut -d ' ' -f 1
}
snapshot="$(financial_snapshot newapi_candidate_smoke)"
docker logs newapi-batch-d-a > "$RUNNER_TEMP/batch-d-recovery.log" 2>&1
docker logs newapi-batch-d-b > "$RUNNER_TEMP/batch-d-second-process.log" 2>&1
cleanup
for target in c production; do
  rollback_image="${PREVIOUS_IMAGE,,}"
  if [ "$target" = production ]; then rollback_image="${PRODUCTION_IMAGE,,}"; fi
  # Each old image sees a separate copy of the exact drained candidate database.
  # Starting C must not prepare or repair the schema for the production rehearsal.
  rollback_database="newapi_rollback_${target}"
  docker exec "$TEST_POSTGRES_CONTAINER" createdb -U postgres -T newapi_candidate_smoke "$rollback_database"
  docker pull "$rollback_image"
  test "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$rollback_image")" = linux/amd64
  if [ "$target" = production ]; then
    test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$rollback_image")" = "$PRODUCTION_REVISION"
  fi
  start newapi-batch-d-a 38001 "$rollback_image" "$rollback_database"
  test "$(financial_snapshot "$rollback_database")" = "$snapshot"
  test "$(docker inspect --format '{{.State.Status}}/{{.State.Health.Status}}/{{.RestartCount}}' newapi-batch-d-a)" = running/healthy/0
  docker logs newapi-batch-d-a > "$RUNNER_TEMP/batch-d-rollback-${target}.log" 2>&1
  if grep -Eiq 'panic:|fatal error:|failed to initialize database|failed to migrate' "$RUNNER_TEMP/batch-d-rollback-${target}.log"; then
    cat "$RUNNER_TEMP/batch-d-rollback-${target}.log"
    exit 1
  fi
  cleanup
  printf 'Drained %s rollback preserved financial snapshot %s.\n' "$target" "$snapshot"
done
printf 'D multi-process admission, ordinary restart recovery and drained C/production rollback passed.\n'
