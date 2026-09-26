#!/usr/bin/env bash
set -euo pipefail

# This disposable rehearsal runs only in GitHub-hosted Actions.
test "${GITHUB_ACTIONS:-}" = true
candidate_image="${CANDIDATE_IMAGE,,}"
test -n "${EXPECTED_REVISION:-}"
test -n "${PRODUCTION_IMAGE:-}"
test -n "${PRODUCTION_REVISION:-}"
test -n "${TEST_POSTGRES_CONTAINER:-}"
cleanup() { docker rm -f newapi-batch-a-smoke >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker pull "$candidate_image"
test "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$candidate_image")" = linux/amd64
test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$candidate_image")" = "$EXPECTED_REVISION"
docker exec "$TEST_POSTGRES_CONTAINER" createdb -U postgres newapi_candidate_smoke

query() {
  docker exec "$TEST_POSTGRES_CONTAINER" psql -XAt -v ON_ERROR_STOP=1 -U postgres -d newapi_candidate_smoke -c "$1"
}
history_snapshot() {
  query "SELECT quota,used_quota,request_count FROM users WHERE username='upgrade-history';
    SELECT remain_quota,used_quota FROM tokens WHERE name='upgrade-history';
    SELECT status,quota FROM tasks WHERE task_id='upgrade-legacy-terminal';"
}

# Bootstrap the schema with the actual production image, not candidate AutoMigrate.
docker pull "${PRODUCTION_IMAGE,,}"
test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "${PRODUCTION_IMAGE,,}")" = "$PRODUCTION_REVISION"
test "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "${PRODUCTION_IMAGE,,}")" = linux/amd64
docker run -d --name newapi-batch-a-smoke --network host \
  --health-cmd "wget -q -O - http://127.0.0.1:3000/api/status | grep -Eq '\"success\"[[:space:]]*:[[:space:]]*true'" \
  --health-interval 5s --health-timeout 10s --health-retries 3 \
  -e SQL_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
  -e SESSION_SECRET=disposable-ci-session-only -e UPDATE_TASK=false \
  "${PRODUCTION_IMAGE,,}" >/dev/null
production_ready=false
for attempt in $(seq 1 90); do
  if curl -fsS --max-time 2 http://127.0.0.1:3000/api/status > "$RUNNER_TEMP/batch-a-production-status.json" &&
    [ "$(docker inspect --format '{{.State.Health.Status}}' newapi-batch-a-smoke)" = healthy ]; then
    production_ready=true
    break
  fi
  sleep 2
done
test "$production_ready" = true
test "$(docker inspect --format '{{.State.Status}}/{{.State.Health.Status}}/{{.RestartCount}}' newapi-batch-a-smoke)" = running/healthy/0
node -e 'const fs=require("node:fs");const s=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));if(s.success!==true)process.exit(1)' "$RUNNER_TEMP/batch-a-production-status.json"
test "$(query "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('task_submissions','task_accountings','task_accounting_events','task_accounting_log_receipts','balance_cache_repairs')")" = 0
query "INSERT INTO users(username,password,status,quota,used_quota,request_count,aff_code) VALUES('upgrade-history','not-a-login-hash',2,125000,75000,3,'upgrade-history');
  INSERT INTO tokens(user_id,key,status,name,remain_quota,used_quota) SELECT id,'upgrade-disabled-synthetic-token',2,'upgrade-history',90000,75000 FROM users WHERE username='upgrade-history';
  INSERT INTO tasks(user_id,task_id,platform,status,quota) SELECT id,'upgrade-legacy-terminal','ci-legacy','FAILURE',50 FROM users WHERE username='upgrade-history';" >/dev/null
history_before="$(history_snapshot)"
cleanup

catalog_digest() {
  docker exec "$TEST_POSTGRES_CONTAINER" psql -XAt -U postgres -d newapi_candidate_smoke -c \
    "SELECT table_name,column_name,data_type,is_nullable,coalesce(column_default,'') FROM information_schema.columns WHERE table_schema='public' ORDER BY table_name,ordinal_position" | sha256sum | cut -d ' ' -f 1
}

for start in 1 2; do
  docker run -d --name newapi-batch-a-smoke --network host \
    --health-cmd "wget -q -O - http://127.0.0.1:3000/api/status | grep -Eq '\"success\"[[:space:]]*:[[:space:]]*true'" \
    --health-interval 5s --health-timeout 10s --health-retries 3 \
    -e SQL_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
    -e SESSION_SECRET=disposable-ci-session-only \
    -e UPDATE_TASK=false \
    "$candidate_image" >/dev/null
  ready=false
  for attempt in $(seq 1 90); do
    if curl -fsS --max-time 2 http://127.0.0.1:3000/api/status > "$RUNNER_TEMP/batch-a-status.json" &&
      [ "$(docker inspect --format '{{.State.Health.Status}}' newapi-batch-a-smoke)" = healthy ]; then
      ready=true
      break
    fi
    sleep 2
  done
  if [ "$ready" != true ]; then
    docker logs newapi-batch-a-smoke
    exit 1
  fi
  test "$(docker inspect --format '{{.State.Status}}/{{.State.Health.Status}}/{{.RestartCount}}' newapi-batch-a-smoke)" = running/healthy/0
  node -e 'const fs=require("node:fs");const s=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));if(s.success!==true)process.exit(1)' "$RUNNER_TEMP/batch-a-status.json"
  test "$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/v1/models)" = 401
  test "$(history_snapshot)" = "$history_before"
  test "$(query "SELECT count(*) FROM task_accountings a JOIN tasks t ON a.task_row_id=t.id WHERE t.task_id='upgrade-legacy-terminal'")" = 0
  test "$(query "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('task_submissions','task_accountings','task_accounting_events','task_accounting_log_receipts','balance_cache_repairs')")" = 5
  current_catalog="$(catalog_digest)"
  if [ "$start" = 1 ]; then
    first_catalog="$current_catalog"
  else
    test "$current_catalog" = "$first_catalog"
  fi
  docker logs newapi-batch-a-smoke > "$RUNNER_TEMP/batch-a-start-$start.log" 2>&1
  if grep -Eiq 'panic:|fatal error:|failed to initialize database|failed to migrate' "$RUNNER_TEMP/batch-a-start-$start.log"; then
    cat "$RUNNER_TEMP/batch-a-start-$start.log"
    exit 1
  fi
  cleanup
done
printf 'Candidate %s passed two starts; column catalog %s\n' "$EXPECTED_REVISION" "$first_catalog"
printf 'Production-to-candidate schema upgrade preserved legacy money and did not infer accounting owners.\n'
