#!/usr/bin/env bash
set -euo pipefail

test "${GITHUB_ACTIONS:-}" = true
test -n "${CANDIDATE_IMAGE:-}"
test -n "${PREVIOUS_IMAGE:-}"
test -n "${TEST_POSTGRES_CONTAINER:-}"
container=newapi-batch-c-smoke
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
ready=false
for attempt in $(seq 1 90); do
  if curl -fsS --max-time 2 http://127.0.0.1:3000/api/status > /dev/null; then
    ready=true
    break
  fi
  sleep 2
done
if [ "$ready" != true ]; then
  docker logs "$container"
  exit 1
fi
}
start_image "${CANDIDATE_IMAGE,,}"
timeout 90s node scripts/ci/batch-c-gateway.mjs
assert_accounting() {
test "$(query "SELECT quota||':'||used_quota||':'||request_count FROM users WHERE username='c-alpha-user'")" = 95000:5000:1
test "$(query "SELECT remain_quota||':'||used_quota FROM tokens WHERE name='c-alpha-token'")" = 95000:5000
test "$(query "SELECT used_quota FROM channels WHERE name='c-alpha-channel'")" = 5000
test "$(query "SELECT count(*) FROM logs WHERE username='c-alpha-user' AND type=2 AND quota=5000 AND model_name='c-alpha-public' AND prompt_tokens=0 AND completion_tokens=0 AND (other::jsonb->>'web_search_call_count')='1'")" = 1
test "$(query "SELECT count(*) FROM task_submissions WHERE model_name='c-alpha-public' AND (state='active' OR cache_pending)")" = 0
test "$(query "SELECT count(*) FROM task_accounting_events WHERE NOT delivered")" = 0
}
assert_accounting
test "$(docker inspect --format '{{.State.Status}}/{{.RestartCount}}' "$container")" = running/0
docker logs "$container" > "$RUNNER_TEMP/batch-c-candidate.log" 2>&1
cleanup
docker pull "${PREVIOUS_IMAGE,,}"
start_image "${PREVIOUS_IMAGE,,}"
assert_accounting
test "$(docker inspect --format '{{.State.Status}}/{{.RestartCount}}' "$container")" = running/0
docker logs "$container" > "$RUNNER_TEMP/batch-c-previous-image.log" 2>&1
printf 'Candidate Alpha search charged one tool call; failure released its reservation.\n'
