#!/usr/bin/env bash
set -euo pipefail

test "${GITHUB_ACTIONS:-}" = true
test -n "${CANDIDATE_IMAGE:-}"
test -n "${PREVIOUS_IMAGE:-}"
test -n "${TEST_POSTGRES_CONTAINER:-}"
container=newapi-batch-b-smoke
cleanup() { docker rm -f "$container" >/dev/null 2>&1 || true; }
trap cleanup EXIT
query() {
  docker exec "$TEST_POSTGRES_CONTAINER" psql -XAt -v ON_ERROR_STOP=1 -U postgres -d newapi_candidate_smoke -c "$1"
}
start_image() {
  docker run -d --name "$container" --network host \
    -e SQL_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
    -e SESSION_SECRET=disposable-ci-session-only -e UPDATE_TASK=false \
    "$1" >/dev/null
  for attempt in $(seq 1 90); do
    if curl -fsS --max-time 2 http://127.0.0.1:3000/api/status > "$RUNNER_TEMP/batch-b-status.json"; then
      return
    fi
    sleep 2
  done
  docker logs "$container"
  exit 1
}
assert_settled() {
  test "$(query "SELECT quota||':'||used_quota||':'||request_count FROM users WHERE username='b-compat-user'")" = 800:200:2
  test "$(query "SELECT remain_quota||':'||used_quota FROM tokens WHERE name='b-compat-token'")" = 800:200
  test "$(query "SELECT used_quota FROM channels WHERE name='b-compat-channel'")" = 200
  test "$(query "SELECT status||':'||quota FROM tasks WHERE task_id='b-compat-pending'")" = FAILURE:0
  test "$(query "SELECT status||':'||quota FROM tasks WHERE task_id='b-compat-live'")" = SUCCESS:200
  test "$(query "SELECT status||':'||quota FROM tasks WHERE task_id='b-compat-queued'")" = FAILURE:0
  test "$(query "SELECT status||':'||quota FROM tasks WHERE task_id='b-compat-legacy-terminal'")" = FAILURE:50
  test "$(query "SELECT status||':'||quota FROM tasks WHERE task_id='b-compat-legacy-pending'")" = QUEUED:50
  test "$(query "SELECT count(*) FROM logs WHERE username='b-compat-user'")" = 3
  test "$(query "SELECT count(*) FROM task_accounting_log_receipts r JOIN logs l ON l.id=r.log_id WHERE l.username='b-compat-user'")" = 3
}
rollback_ready() {
  test "$(query "SELECT count(*) FROM task_submissions WHERE state='active' OR cache_pending")" = 0 &&
  test "$(query "SELECT count(*) FROM task_accountings WHERE NOT money_applied OR cache_pending")" = 0 &&
  test "$(query "SELECT count(*) FROM task_accounting_events WHERE NOT delivered")" = 0
}

start_image "${CANDIDATE_IMAGE,,}"
for attempt in $(seq 1 45); do
  if [ "$(query "SELECT count(*) FROM tasks WHERE task_id='b-compat-pending' AND status='FAILURE' AND quota=0")" = 1 ] && \
     [ "$(query "SELECT count(*) FROM task_submissions WHERE state='active' OR cache_pending")" = 0 ] && \
     [ "$(query "SELECT count(*) FROM task_accounting_events WHERE NOT delivered")" = 0 ]; then
    break
  fi
  sleep 2
done
test "$(query "SELECT state||':'||reserved_quota FROM task_submissions WHERE model_name='b-compat-abandoned'")" = released:0
test "$(query "SELECT count(*) FROM task_accountings WHERE NOT money_applied")" = 1
if rollback_ready; then
  printf 'Rollback unexpectedly allowed a managed in-flight task.\n' >&2
  exit 1
fi
NEW_API_TEST_COMPATIBILITY_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
  go test ./model -run '^TestTaskAccountingCompatibilityDrain$' -count=1
for attempt in $(seq 1 45); do
  if rollback_ready; then break; fi
  sleep 2
done
rollback_ready
assert_settled
docker logs "$container" > "$RUNNER_TEMP/batch-b-recovery.log" 2>&1
cleanup

# Old code is only rehearsed after every B-owned task and projection is drained.
# It must never process an in-flight task governed by the new ownership record.
docker pull "${PREVIOUS_IMAGE,,}"
start_image "${PREVIOUS_IMAGE,,}"
assert_settled
test "$(docker inspect --format '{{.State.Status}}/{{.RestartCount}}' "$container")" = running/0
docker logs "$container" > "$RUNNER_TEMP/batch-b-previous-image.log" 2>&1
printf 'Batch B restart recovery and drained previous-image compatibility passed.\n'
