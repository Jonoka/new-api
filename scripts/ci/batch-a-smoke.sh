#!/usr/bin/env bash
set -euo pipefail

# This disposable rehearsal runs only in GitHub-hosted Actions.
test "${GITHUB_ACTIONS:-}" = true
candidate_image="${CANDIDATE_IMAGE,,}"
test -n "${EXPECTED_REVISION:-}"
test -n "${TEST_POSTGRES_CONTAINER:-}"
cleanup() { docker rm -f newapi-batch-a-smoke >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker pull "$candidate_image"
test "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$candidate_image")" = linux/amd64
test "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$candidate_image")" = "$EXPECTED_REVISION"
docker exec "$TEST_POSTGRES_CONTAINER" createdb -U postgres newapi_candidate_smoke

catalog_digest() {
  docker exec "$TEST_POSTGRES_CONTAINER" psql -XAt -U postgres -d newapi_candidate_smoke -c \
    "SELECT table_name,column_name,data_type,is_nullable,coalesce(column_default,'') FROM information_schema.columns WHERE table_schema='public' ORDER BY table_name,ordinal_position" | sha256sum | cut -d ' ' -f 1
}

for start in 1 2; do
  docker run -d --name newapi-batch-a-smoke --network host \
    -e SQL_DSN=postgres://postgres:postgres@127.0.0.1:5432/newapi_candidate_smoke?sslmode=disable \
    -e SESSION_SECRET=disposable-ci-session-only \
    "$candidate_image" >/dev/null
  ready=false
  for attempt in $(seq 1 90); do
    if curl -fsS --max-time 2 http://127.0.0.1:3000/api/status > "$RUNNER_TEMP/batch-a-status.json"; then
      ready=true
      break
    fi
    sleep 2
  done
  if [ "$ready" != true ]; then
    docker logs newapi-batch-a-smoke
    exit 1
  fi
  test "$(docker inspect --format '{{.State.Status}}/{{.RestartCount}}' newapi-batch-a-smoke)" = running/0
  node -e 'const fs=require("node:fs");const s=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));if(s.success!==true)process.exit(1)' "$RUNNER_TEMP/batch-a-status.json"
  test "$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/v1/models)" = 401
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
