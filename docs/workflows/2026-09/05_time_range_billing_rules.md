# Time Range Billing Rules

Batch A fixes the visual request-rule editors in Default and Classic. Previously every range used OR, so 09:00 to 12:00 applied its multiplier all day.

Both editors now generate a half-open AND range when start <= end and OR when start > end. Equal bounds are empty. Hour, minute, weekday, month and day values must be integers within their function's domain. Compound parameter/time rules parse back without losing guards.

Stored within-day OR expressions are ambiguous: they may be legacy generated defects or deliberate always-true pricing. They remain in raw mode; opening or saving the editor must not silently convert them to AND or duplicate their multiplier. No database options or historical charges are rewritten by this fix.

Incomplete/invalid visual rules retain the last valid parent draft and disable saving until corrected or removed. Generation is all-or-nothing, so an invalid condition cannot disappear while leaving the rest of a pricing rule enabled. Raw-mode expressions retain the existing backend validation path.

## Validation

GitHub-hosted Actions runs `bun test web/default/src/features/pricing/lib/time-rule-expr.test.mjs`, covering both implementations with identical fixtures, all 24 hours, wrap/equal boundaries, domains, compound guards and raw-expression preservation. Both frontend builds and Default typecheck are also required. Test results and candidate identity belong to the batch's CI evidence; this document does not claim production deployment.

Rollback is a source/image revert after normal compatibility checks. It restores the previous editor defect. Existing saved expressions are unchanged and require separate explicit edits to correct their intended tariff.

The new lint gate exposed a pre-existing global `brace-expansion=2.1.1` override incompatible with ESLint 10's minimatch 10 dependency, which requires the 5.x named `expand` export. Removing only that override lets the lockfile preserve each consumer's compatible major. This is a necessary development-tool correction, not a general dependency upgrade. Lockfile resolution uses `--lockfile-only --ignore-scripts`; executable validation remains on GitHub-hosted Actions.
