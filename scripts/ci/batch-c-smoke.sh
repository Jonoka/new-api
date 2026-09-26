#!/usr/bin/env bash
set -euo pipefail

test "${GITHUB_ACTIONS:-}" = true
test -n "${CANDIDATE_IMAGE:-}"
test -n "${PREVIOUS_IMAGE:-}"
test -n "${EXPECTED_REVISION:-}"
test -n "${PREVIOUS_REVISION:-}"
test -n "${TEST_POSTGRES_CONTAINER:-}"
container=newapi-batch-c-smoke
cleanup() { docker rm -f "$container" >/dev/null 2>&1 || true; }
trap cleanup EXIT
query() {
  docker exec "$TEST_POSTGRES_CONTAINER" psql -XAt -v ON_ERROR_STOP=1 -U postgres -d newapi_candidate_smoke -c "$1"
}
diagnose_failure() {
  docker logs "$container" 2>&1 | tail -80 || true
  query "SELECT id,name,status,models,to_jsonb(c)->>'group' FROM channels c WHERE name='c-alpha-channel'"
  query "SELECT to_jsonb(a) FROM abilities a WHERE model='c-alpha-public'"
  query "SELECT to_jsonb(b) FROM channel_groups b WHERE channel_id IN (SELECT id FROM channels WHERE name='c-alpha-channel')"
}
trap diagnose_failure ERR
start_image() {
docker run -d --name "$container" --network host \
  --health-cmd "wget -q -O - http://127.0.0.1:3000/api/status | grep -Eq '\"success\"[[:space:]]*:[[:space:]]*true'" \
  --health-interval 5s --health-timeout 10s --health-retries 3 \
  -e SQL_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
  -e SESSION_SECRET=disposable-ci-session-only -e UPDATE_TASK=false \
  "$1" >/dev/null
ready=false
for attempt in $(seq 1 90); do
  if curl -fsS --max-time 2 http://127.0.0.1:3000/api/status > "$RUNNER_TEMP/batch-c-status.json" &&
    [ "$(docker inspect --format '{{.State.Health.Status}}' "$container")" = healthy ]; then
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
check_image() {
  local image="$1" expected_revision="$2"
  docker pull "$image"
  test "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$image")" = linux/amd64
  test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$image")" = "$expected_revision"
}
check_image "${CANDIDATE_IMAGE,,}" "$EXPECTED_REVISION"
check_image "${PREVIOUS_IMAGE,,}" "$PREVIOUS_REVISION"
start_image "${CANDIDATE_IMAGE,,}"
node -e 'const fs=require("node:fs");const s=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));if(s.success!==true)process.exit(1)' "$RUNNER_TEMP/batch-c-status.json"
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
test "$(docker inspect --format '{{.State.Status}}/{{.State.Health.Status}}/{{.RestartCount}}' "$container")" = running/healthy/0
docker logs "$container" > "$RUNNER_TEMP/batch-c-candidate.log" 2>&1
cleanup
start_image "${PREVIOUS_IMAGE,,}"
node -e 'const fs=require("node:fs");const s=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));if(s.success!==true)process.exit(1)' "$RUNNER_TEMP/batch-c-status.json"
assert_accounting
test "$(docker inspect --format '{{.State.Status}}/{{.State.Health.Status}}/{{.RestartCount}}' "$container")" = running/healthy/0
docker logs "$container" > "$RUNNER_TEMP/batch-c-previous-image.log" 2>&1
printf 'Candidate Alpha search charged one tool call; failure released its reservation.\n'
