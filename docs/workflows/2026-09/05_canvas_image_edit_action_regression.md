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

The first GitHub Actions run, `33942891324`, passed both frontend gates and
compiled the Go packages. The new action table test failed because its
platform assertion compared a plain string with the named `TaskPlatform`
type. The assertion now uses the explicit named type; production code did
not change. That run published no image. Final CI validation is pending.
