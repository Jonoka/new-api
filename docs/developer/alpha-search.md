# Alpha Search and Request Replay

Batch C implementation contract. Candidate validation and real-gateway production
acceptance are pending. This document evolves with the implementation.

## API and Compatibility

`POST /v1/alpha/search` uses normal token authentication, model/group restrictions,
distribution and pinning. The body must be a JSON object with a nonempty string
`model`. Only synchronous requests are accepted; `stream: true` is rejected.
Unknown values remain intact, including integers larger than 2^53. Model mapping
starts from the original body each attempt; public billing/log identity stays
unchanged. A conflicting parameter override of the mapped model is rejected.

Native Codex (type57) uses its OAuth/account headers and
`/backend-api/codex/alpha/search`. OpenAI-compatible gateway (existing type1) uses
its gateway Bearer key and `/v1/alpha/search`. A gateway base ending in `/v1` is
supported. The gateway must actually implement Alpha; model listing or Responses
support alone does not establish this. Existing channels need no type conversion.

The pinned Codex client `rust-v0.144.1` expects JSON `output` (string) and optional
`encrypted_output` (string). Empty output is a valid no-result response. The relay
reads at most 8 MiB plus one sentinel byte, rejects larger responses, and forwards
the validated raw bytes intact, including unknown fields, only after settlement.
This bound is intentionally far below the existing 128 MiB request default while
leaving room for the pinned client's two string fields. HTML, malformed JSON,
error objects and empty bodies cannot count as success. Upstream JSON Content-Type
is retained; other media types normalize to application/json. Content-Length is
recalculated. No redirects are followed
for Alpha. No Responses conversion, SSE or synthetic token usage is introduced.

Both success and error bodies use this cap. Actual provider result sizes remain
unverified until the approved canary. Parameter overrides cannot enable streaming,
and final request Accept/Content-Type remain application/json after header overrides.

## Accounting

A successful Alpha request charges one configured `web_search_preview` call using
the original model's configured tool price, final selected group ratio and
applicable tool multipliers. It adds no model fixed request fee, token fee or
tiered expression base charge. Zero model ratio does not waive a positive tool
cost. Explicit zero tool price/group ratio records a free successful request.
Admission precedes upstream I/O, settlement precedes client success, and failures
release the reservation. Existing non-Alpha billing is preserved.

If a client disconnects while publishing an already validated and settled search,
the completed search remains charged once and is never replayed. A transport or
response-read failure before validated success is not blindly retried and releases
the active reservation. Explicit provider error statuses retain channel retry
rules; Alpha never follows redirects.

## Replay and Lifetime

Each stored final outbound body supplies independent offset-zero readers for
transport retry. Memory readers share immutable bytes; disk readers use separate
descriptors without loading whole files into RAM. Child close leaves the owner
alive. New readers fail after owner close. Content length and replay metadata are
reset each channel attempt. Transport replay does not broaden application retry
eligibility or allow retry after downstream output.

## Verification and Release

Source tests/builds run only in GitHub-hosted Actions. Required fixtures cover
HTTP/2 REFUSED_STREAM after full upload, memory/disk lifecycle, exact transformed
bytes, route/auth and lossless mapping, native/gateway headers and URLs, response
validation, redirects, disconnects and search-only accounting across pricing and
group transitions. SQLite, PostgreSQL and MySQL retain B accounting guarantees.

Production acceptance additionally requires a separately approved bounded real
gateway canary with output and accounting evidence. B's accounting drain and
rollback preconditions apply to this candidate. No production change is implied
by endpoint metadata or passing mock fixtures.

The image workflow runs `scripts/ci/batch-c-smoke.sh` against the disposable CI
PostgreSQL database and a Node standard-library mock gateway. It verifies normal
auth/model restrictions, local stream rejection, lossless model mapping, success
and failure accounting, then starts the B image after the C journal/outbox drains.
The configured model fixed price in this fixture is deliberately positive, proving
that only the one search-tool charge reaches the persisted counters/log.
