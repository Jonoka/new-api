# Canvas 2 SSO

This additive integration preserves the existing canvas launcher and relay billing.
It is enabled only with `CANVAS_SSO_ORIGIN`, an exact HTTPS origin without a
path, user info, query or fragment. `CANVAS_SSO_LAUNCH_ENABLED` defaults to false
and controls only publication of the separate Canvas 2 command in both themes.
Validated login resumption works while the command is hidden. Explicit default
port `:443`, empty ports, zero, noncanonical leading zeros and ports above 65535
are rejected at startup, matching the browser launch origin contract.

## Identity Exchange

- `POST /canvas/auth/authorize` requires the configured exact browser Origin,
  JSON and the existing New API session. Input: `state`, `code_challenge`,
  `code_challenge_method: "S256"`, `audience` equal to the configured origin.
  Success: `{success:true,data:{code,state,expires_in:60},message:""}`.
- `POST /canvas/auth/exchange` takes `code`, `state`, `code_verifier`, `audience`.
  It requires no browser cookie; possession of the browser-bound verifier is
  required. Success returns only `{id,username,display_name,role,status}` under
  the normal `data` envelope. Roles 1, 10 and 100 are accepted, without remapping.
- A random 256-bit opaque code is stored under its SHA-256 Redis key for 60
  seconds. Redis 7 `GETDEL` consumes it once, including invalid proof attempts.
  There is no memory fallback. Redis operations use a two-second deadline and
  never the generic value-logging wrappers. An SSO-only client copies the existing
  Redis connection options but disables command retries; a consumed code whose
  reply is lost produces 503 without a second `GETDEL`. Other clients retain their
  existing retry policy. Role/status are read directly from
  the database when authorizing and consuming, not the user cache.
- Authentication payloads are bounded to 2 KiB and responses are `no-store`.
  Missing session is 401, invalid proof/input is 400, disabled identity/origin
  is 403, and unavailable Redis/database is 503. No API key, code, verifier,
  or cookie is placed in a URL or logged.

## Browser Contract

The separate command opens the fixed origin with `newapi_launch=1` and `group`.
The Canvas 2 application owns the cookie-backed PKCE challenge and the SSO
account. On a New API 401 it returns through `/canvas/auth/launch` with
`canvas_resume=1`, `canvas_next` (relative destination) and optional `group`.
That endpoint selects `/canvas` for Default or `/console/canvas` for Classic.
The authenticated launcher consumes the resume marker, then returns with
`newapi_returned=1`, which Canvas 2 must enforce as a one-roundtrip limit.
Logout uses the same launcher endpoint without a resume marker. It never calls
New API logout. Invalid or external destinations are discarded, including paths
whose raw or encoded dot segments normalize to a protocol-relative destination.
The final launch URL origin is checked again after resolving the destination.

`/api/status` exposes `canvas_sso_origin` and `canvas_sso_launch_enabled` only.
Both themes preserve login return context through password, 2FA and OAuth flows.

## Models, Verification And Rollback

`POST /canvas/v1/responses` uses the existing session, group preparation,
distribution, model rate limit and Responses relay. All other old-canvas routes,
group selection and model billing remain unchanged.

GitHub-hosted Actions must run Go checks with isolated Redis 7, concurrent
redemption and stale-role tests, both frontend launch contract checks and both
frontend builds. Integrated browser verification belongs to the paired canvas
change. No production identities or paid providers are test fixtures.

Deploy the additive endpoints first, validate the paired Canvas 2 application,
then enable the launcher only under a separately approved deployment. Disabling
the launch flag hides the new command; clearing the origin disables SSO. No
database migration or legacy-canvas data change is required on New API.
