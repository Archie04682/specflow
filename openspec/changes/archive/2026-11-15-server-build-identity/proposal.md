# Proposal

## Why

In development the page and the engine are different processes: Vite serves the
frontend from source, while `make dev` starts `go run` once and never rebuilds
it. The page therefore looks current while the API underneath can be days old,
and nothing in the product says which build is answering. That blindness has
already cost a real run: a question was answered into a backend built before
the interactive-wait fix, the node failed as timed out for behavior the
checkout had already corrected, and the only evidence was a timeout error with
no hint that the server itself was stale.

## What Changes

- **The API reports its identity.** `GET /api/server` returns the service name,
  version, source revision, revision time, modified flag, and process start
  time. Anything the build cannot determine is omitted; the endpoint never
  fails because a stamp is missing.
- **The API says whether it is behind its checkout.** When the server is
  running from a recognized SpecFlow source tree, the same response reports
  whether that checkout has moved past the running build — a newer HEAD or
  backend sources modified after the build — with a machine-readable reason.
  The probe is local, best-effort, cached, and absent when the server is not
  running from source.
- **The build stamps what the Go toolchain does not.** `make dev` and
  `make build` pass revision, revision time, dirty flag, and build time through
  `-ldflags`, because a `go run` binary carries no VCS settings.
  `debug.ReadBuildInfo` stays the fallback for a plain `go build`.
- **The workbench surfaces it.** The shell reads the identity on mount and on
  window focus. Settings shows the running build, its start time, and the
  checkout state; the workspace shows a dismissible warning when the API is
  behind the checkout, naming the restart action. A failed identity read is
  never an error state.
- **`GET /api/health` is untouched.** Identity is a separate diagnostic
  surface, so the pinned health contract stays as it is.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `single-binary-distribution`: adds the server build-identity endpoint, the
  behind-checkout report, and the build stamping that makes both possible.
  Health and port behavior do not change.
- `harness-shell`: adds the server identity in Settings and the stale-API
  warning in the workbench.

## Impact

- `backend/internal/buildinfo` (new): read `-ldflags` stamps, fall back to
  `debug.ReadBuildInfo`, and probe a checkout for a newer HEAD or newer
  backend sources.
- `backend/internal/api`: `GET /api/server` handler and the identity dependency
  the router needs; `/api/health` and every other route unchanged.
- `backend/cmd/api`: capture the process start time, read the identity, and hand
  it to the router.
- `Makefile`: `-ldflags` stamping for `dev` and `build`; `git` is already a
  product dependency, so no new tool is introduced.
- `frontend/src`: typed `server` API client and type, a shell identity hook, a
  Settings section, the warning banner, styles, and tests.
- `README.md` and `AGENTS.md`: document that the API advertises its build, that
  the shell warns when it is stale, and that `make dev` does not watch Go files.
- No new runtime dependency, no second executable, no auth, no auto-restart,
  and no update mechanism.
