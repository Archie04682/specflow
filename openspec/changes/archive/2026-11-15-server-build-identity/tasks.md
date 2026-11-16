# Tasks

## 1. Build identity

- [x] 1.1 Add `internal/buildinfo` with an `Info` value (service, version,
      revision, revision time, modified flag, build time) read from injected
      `-ldflags` strings, falling back to the Go toolchain's build settings.
      Verify with table-driven tests covering all stamps present, some absent,
      none present, and a `debug.ReadBuildInfo` seam that returns VCS settings.
- [x] 1.2 Stamp the build from the Makefile: compute revision, revision time,
      modified flag, and build time once and pass them through `-ldflags` to
      both the `dev` (`go run`) and `build` (`go build`) targets. Verify
      `make build` succeeds and `go version -m ./specflow` lists the four `-X`
      settings, and confirm `make -n dev` prints the same flags on the
      `go run` command.

## 2. Checkout staleness probe

- [x] 2.1 Implement the checkout probe in `internal/buildinfo`: locate the
      module root by walking up from a startup directory and requiring
      `go.mod` with module `example.com/specflow` plus a `backend/` directory,
      resolve HEAD with git, and compare the built revision with HEAD using an
      ancestor check. Verify with tests in a temporary git repo: current
      checkout reports nothing, a commit after the build reports
      `checkout-ahead`, unrelated history reports nothing, and a non-module
      directory prevents any probe.
- [x] 2.2 Add the source-recency check: scan `backend/**/*.go` (excluding
      `_test.go`), `go.mod`, and `go.sum`, compare the newest modification
      time with the build-time reference (falling back to process start), and
      report `sources-newer` with a bounded count. Verify with tests for an
      edited file, an edited test file that must not trigger, and a file older
      than the build.
- [x] 2.3 Bound and cache the probe so repeated requests cannot rescan: one
      walk, at most two git invocations, a short cache window, and every
      failure yielding no report. Verify a test calls the probe twice inside
      the window and asserts a single scan (injected scan counter or clock),
      and that a failing git lookup still returns a usable result.

## 3. `GET /api/server`

- [x] 3.1 Add the server identity dependency and the `GET /api/server` handler
      with the documented JSON shape, omitting unknown fields and swallowing
      probe failures. Verify with `httptest` tests covering full identity,
      minimal identity, a stale checkout object, an omitted checkout, and a
      probe that reports an error.
- [x] 3.2 Add the additive router constructor that accepts the identity and
      probe seam, keeping the existing constructors delegating with a zero
      identity. Verify `go test ./backend/internal/api/...` passes, including
      the unchanged health test, and that `GET /api/does-not-exist` still
      returns a JSON `404`.
- [x] 3.3 Wire `cmd/api`: capture the process start time, read the build
      identity once, resolve the startup checkout root, and pass both to the
      router. Verify by starting the server through the stamped dev command on
      a free port and checking `curl /api/server` reports a revision equal to
      `git rev-parse HEAD` and a start time, while `curl /api/health` is still
      exactly `{"status":"ok"}`.
- [x] 3.4 Document the new endpoint in `README.md` (how to read the running
      build and the checkout state) and verify every command and path in the
      new text matches the Makefile and the handler.

## 4. Workbench shell

- [x] 4.1 Add the `server` response type, the typed API client function, and a
      `useServerIdentity` hook that reads on mount and on window focus, keeps
      the last good value, and treats failure as no identity. Verify with hook
      tests for a successful read, a focus refresh that replaces the identity,
      and a failed read that leaves no error state.
- [x] 4.2 Add the Settings **Server** section fed by the workbench: revision
      (short, with the full hash available), build time when reported, start
      time, and the checkout state (`up to date`, `restart needed`, or
      `not running from source`). Verify with a Settings test that renders a
      full identity and the not-running-from-source state.
- [x] 4.3 Add the stale-build warning to the workbench: a warning banner above
      the view content naming the running revision and the restart action,
      dismissed per running process and hidden for a current or unavailable
      identity. Verify with tests for warning when stale, silence when
      current, silence when the identity read fails, dismissal for the same
      process, and a warning returning for a different start time.
- [x] 4.4 Add the styles for the Settings server block and the warning
      placement without changing the workspace layout at the desktop and
      narrow breakpoints. Verify by resizing across 900 px and checking the
      panel, workspace, and dialog show no overlap or clipping, and that
      existing reduced-motion rules are untouched.
- [x] 4.5 Update `README.md` and `AGENTS.md` with the behavior and the dev-loop
      consequence: the API advertises its build, the shell warns when it is
      behind, and `make dev` does not watch Go files. Verify the documented
      commands match the Makefile and the claimed behavior matches the tests
      added in 4.1-4.3.

## 5. Integration

- [x] 5.1 Run `make test` and `openspec validate server-build-identity --strict`;
      verify both pass with no skipped package or typecheck.
- [x] 5.2 Reproduce the original failure in reverse: with the new dev server
      running, edit a backend source file and confirm `/api/server` reports
      `sources-newer` and the shell shows the warning; restart the server and
      confirm the warning clears. Verify the checkout-ahead path with the unit
      test from 2.1 (a temporary repository is enough).

## Workflow follow-up

- Archive the change after `make test` and `openspec validate server-build-identity --strict` pass.
