# Production billing hotfix and A-D integration

Production d05e01e2 fixed final-success-group billing on the earlier source line.
The A-D candidate already owns selected-attempt admission and durable money flow.
Their integration keeps the A-D owner rather than stacking the old getChannel
preconsume path onto it. Tiered retries retain initial non-group estimation facts
while refreshing selected-group metadata and reservation targets. Actual-usage
settlement preserves its separate existing policy.

Production routing regressions run against A-D fixtures and assert its current
reservation target rather than the superseded initial-reservation invariant.
The frozen-estimate case must retain the original expression, input estimate,
quota-before-group, quota unit and tier through retries and global config changes.

## Validation and release

All source tests/builds run on GitHub-hosted Actions. Existing A-D Go/race/database,
Redis, two-frontend, image startup and recovery gates remain mandatory. The final
startup fixture first boots the production image to create its own schema, adds
disabled synthetic wallet/token and legacy terminal task rows, then starts the
candidate twice. Legacy money stays unchanged and no accounting owner is inferred.
The final
D rehearsal also checks rollback to actual production image
`ghcr.io/jonoka/new-api@sha256:1611754fd5d229d91292089a760f58ca64e8e2cc8770b0096c418f587eaa5134`
with source `d05e01e2d69a1141cdb68360b84f9542ff12a048`.
The production bootstrap, candidate, A/B/C rollback and production rollback all
inject the effective Compose status-success healthcheck, require
`running/healthy` with zero restarts, and verify `linux/amd64`. The rollback
images also bind their immutable digests to their expected source revisions:
A `69444ce731494da6010885b517ab3d8130c59553`, B
`80b7d65697d1421caff06d6e4a6062c9c221fccd`, C
`9b4c4bf6d3de412fb17a8ff40b77be78ccea71d2`, and the production source above.

C and production start against separate clones of the drained candidate database;
an intermediate old-image migration must not mask incompatibility. Financial
snapshots include users, tokens, channel usage, subscriptions, tasks, logs, durable
accounting and retained repair receipts. Rehearsal success is not production
acceptance: release needs fresh runtime inventory, exact override backup, verified
drain/rollback, bounded real-gateway accounting acceptance and health observation.

No historical money adjustment, official upstream migration or unrelated SSO is
part of this release. Pending settlements and active managed tasks block rollback;
an image revert never reverses committed money. After minimum 15-minute health
acceptance, observe at least one 24-hour business cycle before official patches.
