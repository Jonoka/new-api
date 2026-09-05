# Request-body transport replay

Date: 2026-09-06

## Problem and scope

Large transformed relay bodies are exposed to `net/http` through a type-erased
reader backed by `BodyStorage`. Go cannot infer `ContentLength` or `GetBody` from
that reader. If an HTTP/2 upstream sends `REFUSED_STREAM` after accepting the
complete upload, the transport needs `GetBody` to retry the stream; without it,
the request fails even though the reset is explicitly safe to retry.

This change gives stored bodies independent readers and propagates the exact
final byte length plus a fresh-reader factory through JSON, pass-through,
Responses continuation, image, task and direct-provider request paths. It does
not change channel-selection retry policy and does not permit application replay
after client-visible output.

## Storage and lifetime contract

`BodyStorage.NewReader()` returns an `io.ReadCloser` positioned at byte zero.
Memory readers share an immutable owned snapshot with independent cursors. Disk
readers open independent file descriptors and never load the full cache file
back into memory. Closing a child reader leaves the owner usable. Closing the
owner rejects later readers with `ErrStorageClosed`; readers opened earlier stay
valid until they close. Disk cleanup waits for open child descriptors, which
also keeps cleanup correct on platforms that cannot unlink an open file.

The handler owns outbound storage through the complete upstream transport and
response flow. `net/http` owns and closes readers returned by `GetBody`. Inbound
pass-through storage remains owned by request cleanup middleware.

## Request metadata contract

`RelayInfo.UpstreamRequestBodySize` and
`RelayInfo.UpstreamRequestBodyFactory` describe the same final transformed byte
sequence. Producers set both together. `ApplyUpstreamBodyMetadata` fills only
metadata that `net/http` did not infer, preserving the correct factories created
for concrete `bytes.Reader`, `bytes.Buffer` and `strings.Reader` values.

`RelayInfo.ResetUpstreamRequestBody` runs at channel initialization so a later
attempt cannot retain a factory whose previous owner has closed. Responses
continuation retries replace both fields when they remove `previous_response_id`.
The task helper no longer creates a `GetBody` closure over its already-consumed
reader.

The covered producers are:

- transformed compatible, Claude, Gemini, embedding, rerank, image and Responses JSON;
- chat-completions-via-Responses and native Responses continuation retries;
- original request pass-through, including Claude-aware and image paths;
- opaque Sora task uploads and concrete JSON/multipart task bodies;
- Jimeng's direct signed request constructor.

Other direct provider and task constructors use concrete replayable readers, so
their native `ContentLength` and `GetBody` remain in place.

## Redirect and error boundaries

Alpha Search uses a shallow copy of the selected HTTP client with redirects
disabled for that request only. Ordinary relay modes retain the configured
redirect policy. The original transport, connection pools, timeout and cookie
jar are preserved.

## Verification

GitHub-hosted Actions must run the focused storage and relay packages plus the
repository's full Go suite. The deterministic HTTP/2 fixture reads the complete
first request body, returns `REFUSED_STREAM`, and verifies the transport sends
the exact body on its second stream. Additional fixtures cover independent
memory and disk readers, concurrent reads, owner/child lifetime, cleanup,
metadata reset, native factory preservation, image pass-through and Sora task
pass-through.

No local or production-host compilation or test execution is permitted. Source
formatting may use the pinned workspace `gofmt`; static delivery checks include
`git diff --check`.
