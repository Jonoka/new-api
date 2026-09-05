# Canvas Image Edit Action Regression

## Goal And Scope

Restore image-to-image submission through `/canvas/v1/images/edits` on the
production baseline `6507f07940e0b35dc7aa5d3df55dcd7b7cba376b` in an isolated
worktree. Preserve text-to-image requests and explicit task action selection.
No channel configuration, model mapping, billing, schema, frontend, or production
runtime change is part of this source repair.

## Observed Failure

Request `202609050324325292774308268d9d6FmVaHXHl` entered the Canvas edit route
on 2026-09-05. Task 483 was persisted as `images/generations` and failed with
the distributor's default model `dall-e`; recorded task quota was zero.

`CanvasImageTaskSubmit` reads only the optional `action` query. Without that
query, the normalizer defaults to generations even when the URL is an edit
route. The background relay then sends the multipart body to the generation
path, which skips multipart model extraction and falls back to `dall-e`.

The earlier fix `18ea6828475a1166cfdb08ffb837c8d92557ef96` and its submit-level
regression test are absent from the current production ancestry. The action
helper and its helper-only test remain, but do not prove that submission uses
the helper. A passing helper test therefore did not protect the deployed flow.

## Contract And Implementation

- Reuse `canvasImageTaskAction(c)` in `CanvasImageTaskSubmit`.
- Preserve explicit action precedence and existing generic-task defaults.
- Persist and replay the same derived action.
- Preserve the multipart boundary, model, prompt, and image bytes.
- Keep session authentication, group selection, distribution, rate limits,
  billing, task retention, and regular token API behavior unchanged.
- Restore the existing asynchronous-start test hook so submit tests can capture
  the request without starting a background provider call.

## Verification Plan

Exercise the actual submit handler with direct edit and generation routes,
generic task defaults, explicit actions, and explicit action precedence.
Assert the stored task action and the resulting replay path. Include a
multipart edit round trip that retains the selected model and image bytes.
Use the existing in-memory test database helper and stub relay handler; no
paid provider requests or production test workloads are required.

Run the repository Go tests and existing frontend/image gates in the
GitHub-hosted `hermes-build-image.yml` workflow with `push_latest=false`.
Check Go/Markdown formatting, document links, and `git diff --check`.

## Integration And Deployment Boundary

This worktree starts at the production commit so the fix can be reviewed or
cherry-picked independently of the concurrent New API upgrade. Any later
release must retain both this submit-level regression test and the latest
approved production fixes. Commit ancestry alone does not prove preservation
when fixes have been cherry-picked; inspect the resulting code and tests.

Production deployment requires separate approval and a freshly verified
immutable image, current runtime inventory, and byte-preserving Compose
override backup. If deployment is approved later, change only the New API
image and retain the established rollback procedure. Existing failed tasks
are not automatically retried by this repair.

## Verification Results

Prettier, the new developer-document link, and `git diff --check` passed.
The repaired controller matches the previously verified controller at
`18ea6828475a1166cfdb08ffb837c8d92557ef96` byte-for-byte in Git.

- Validated source: `0c51d93e6f02163835d536de734b647e96808813`.
- [GitHub Actions run 33943253775](https://github.com/Jonoka/new-api/actions/runs/33943253775)
  completed successfully: merge-diff check, Default type-check/build, Classic
  build, `go test ./... -count=1`, and image publication.
- The Go suite includes seven submit-action cases and the multipart edit
  submission/replay test in `controller/canvas_image_task_action_test.go`.
- Candidate tag: `ghcr.io/jonoka/new-api:hermes-canvas-edit-0c51d93e`.
- Immutable candidate:
  `ghcr.io/jonoka/new-api@sha256:bba9a4b80fac9d1b542bb62f087fd433b028bd796effedb8205e9d23a58139bd`.
- Registry inspection confirmed `linux/amd64` and OCI revision
  `0c51d93e6f02163835d536de734b647e96808813`.
- The first CI attempt (`33942891324`) published no image because the new
  platform assertion compared a plain string with the named `TaskPlatform`
  type. The final test uses the explicit named type and passed the full suite.
- During source validation, no production deployment, channel/configuration
  change, or paid generation request was performed. The test evidence covers
  request routing and payload preservation using the test relay.

For the concurrent upgrade, bring both source commits `aa9b4f03` and
`0c51d93e` into its isolated branch, or merge this repair branch. The former
restores routing and adds the regression tests; the latter corrects the test
assertion type. Re-run the upgrade's checks on the combined source before
selecting a production image.

## Production Deployment

The owner subsequently approved independent deployment. On 2026-09-05,
production changed from the verified `6507f079` image to the immutable
`bba9a4b8` image above. Only `services.new-api.image` changed; only `new-api`
was recreated with `--no-build --no-deps`.

Six observations passed between 04:48:59Z and 04:51:30Z: exact image/revision,
running/healthy, restart 0, local/public status HTTP 200, and no critical
runtime log matches. Twenty unrelated containers, including PostgreSQL,
Redis, and Infinite Canvas, retained their identities and runtime state.
Final curl probes of the local and public Canvas root returned HTTP 200.

The restricted byte-preserving backup, deployment evidence, and guarded
rollback helper are under
`/opt/newapi-cutover/newapi-canvas-edit-regression-20260905T044818Z/`.
The override hashes are:

- Before: `f8fbdb9343112029d42d11f6a4cdb7365b19dee3b1758afeeb5a0ef1b3574028`.
- After: `b06200df849383d9b1cf1109cbb58a6cb4887ba7bd23dac337892ada26b1dea8`.

The remote `vps-ops` service page, index, append-only log, and
`runbooks/new-api-canvas-image-edit-routing.md` were updated under the topic
lock with verified preimages and post-write validation. No rollback was
needed. No paid generation probe was sent, so provider image output after
cutover remains unverified. Historical failed tasks must be submitted again.
