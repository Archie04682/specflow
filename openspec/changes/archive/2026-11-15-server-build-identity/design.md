# Design

## Context

See `proposal.md` for motivation. The facts that shape the approach, all
verified in this checkout:

- `make dev` runs `go run ./backend/cmd/api` once and never watches Go files;
  Vite serves the page from source and proxies `/api` to that process. The
  page can be current while the engine is arbitrarily old.
- A `go run` executable carries **no** version-control settings in its build
  info — the dev server started on Nov 14 reported only `(devel)` — while a
  `go build` executable does carry `vcs.revision`, `vcs.time`, and
  `vcs.modified`. Any identity that must work for the dev loop cannot rely on
  `debug.ReadBuildInfo` alone.
- The Go timer clock on Darwin excludes system sleep, so a node's active-work
  budget can survive a five-hour clamshell sleep and expire seconds after the
  operator answers. This is why a stale *engine* produces a timeout that looks
  like the answer's fault.
- `/api/health` is a pinned requirement and a deliberately dumb probe
  (`{"status":"ok"}`). `/api/` already serves a number of JSON routes; the
  frontend has one typed client in `frontend/src/api/client.ts`.
- `git` is already a product dependency (worktrees, checkpoints), and the
  repository is a single Go module (`example.com/specflow`) with a flat
  `openspec/specs/` layout.

## Goals / Non-Goals

**Goals:**

- Make "which build is serving this page?" answerable from the running page.
- Detect the two stale cases that actually occur in the dev loop: a checkout
  that committed past the running build, and uncommitted backend source edits
  the running build never compiled.
- Keep the detection server-side, bounded, non-fatal, and invisible when it
  does not apply.
- Add no dependency, no second executable, and no new persistent client state.

**Non-Goals:**

- No auto-restart, no file watcher, no build orchestration in the server.
- No remote version check, no update mechanism, no telemetry.
- No change to `/api/health`, port resolution, or the run lifecycle.
- No detection of a stale *embedded UI*: in development Vite serves the UI
  live, and `make build` rebuilds the frontend and re-stamps the binary, so a
  newer `frontend/src` file does not make the API stale. Frontend sources are
  therefore not part of the staleness inputs.

## Decisions

### Decision: a separate `GET /api/server`, not an extension of health

Health answers "is the process listening"; identity answers "which build is
it, and is it behind this checkout". Extending health would edit a pinned
requirement and couple liveness probes to build metadata; a separate route
keeps both contracts simple and lets the identity shape evolve. The route is
registered in the same router as health, before the `/api/` not-found
fallback, and returns JSON on `GET` only.

Response shape (all unknown values are omitted, never `""` or `null`):

```json
{
  "service": "specflow",
  "version": "0.1.0",
  "revision": "fecd6ec...",
  "revisionTime": "2026-11-15T13:07:33Z",
  "modified": true,
  "builtAt": "2026-11-15T13:30:12Z",
  "startedAt": "2026-11-15T16:11:03Z",
  "checkout": {
    "revision": "e914c59...",
    "staleReason": "checkout-ahead",
    "newerSources": 0
  }
}
```

`staleReason` is `checkout-ahead` (the checkout's HEAD is a descendant of the
build) or `sources-newer` (a backend input changed after the build); an empty
or absent `staleReason` means current. `checkout` is omitted entirely when the
server did not start from a recognized SpecFlow checkout. No absolute paths,
file names, or PIDs are exposed: the server binds all interfaces, and the
response should not disclose host paths. A revision hash is intentionally
included — it is the whole point.

### Decision: stamp identity through `-ldflags`, fall back to `debug.ReadBuildInfo`

`make dev` and `make build` pass `revision`, `revisionTime`, `modified`, and
`builtAt` as `-ldflags -X` values into a small `internal/buildinfo` package;
`Info` reads the injected values and falls back to `debug.ReadBuildInfo` for
anything absent. This is required because `go run` emits no VCS settings: the
dev loop is exactly the case that must report an identity.

Alternatives rejected: a generated Go file (codegen churn for four strings);
a `cmd/devstamp` helper (a second executable for a shell expression);
documenting "always use `go build`" (does not fix the existing dev workflow).

The shell computes the values once per build:

```make
LDFLAGS := -X .../buildinfo.revision=$(git rev-parse HEAD) \
           -X .../buildinfo.revisionTime=$(git show -s --format=%cI HEAD) \
           -X .../buildinfo.modified=$(test -n "$$(git status --porcelain)" && echo true || echo false) \
           -X .../buildinfo.builtAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)
```

A plain `go build` without the Makefile still reports the toolchain's VCS
stamp, so identity degrades rather than disappearing.

### Decision: two independent staleness checks, computed server-side

Both run in the new `internal/buildinfo` package against the checkout found by
walking up from the process's startup working directory; the root is
recognized only when `go.mod` declares module `example.com/specflow` and a
`backend/` directory exists. Anything else yields no `checkout` report.

1. **Checkout ahead.** Resolve `git rev-parse HEAD`; when the built revision is
   an ancestor of HEAD (`git merge-base --is-ancestor <built> HEAD`) and they
   differ, the running code is behind a commit. A build from unrelated history
   is not comparable and reports nothing — no false "behind" on a different
   branch or checkout.
2. **Sources newer.** Walk `backend/**/*.go` (excluding `_test.go`), `go.mod`,
   and `go.sum`, and compare the newest file mtime with the build reference.
   The reference is `builtAt` when stamped, falling back to the process start
   time when it is not — **not** `startedAt` alone: an edit made after the
   build but before the process started is still missing from the binary, and
   only a build-time reference catches it. `_test.go` is excluded because test
   edits do not change the server.

The probe is local and bounded: one walk over ~200 files and at most two git
invocations, cached for a few seconds so a focus refetch cannot rescan. An
unrecognized tree omits the checkout report; unreadable version-control state
drops only the version-control findings, so a source tree without git still
reports `sources-newer`. Every failure is silent. A `staleReason` from either
check is enough to warn; when both fire, the checkout-ahead reason wins because
it is unambiguous.

Alternatives rejected: a build-time content fingerprint (precise, but the
build-time side would need its own hashing tool and an identical algorithm at
runtime); a filesystem watcher (a long-lived subsystem for a warning that a
request can compute); comparing against the page build (in development the
page has no single build — Vite serves it from source).

### Decision: the shell reads identity once and on focus, and warns in memory

A `useServerIdentity` hook fetches on mount and on `window.focus` (the same
pattern the tree leaves use for catalogs), keeps the last good value, and
treats a failure as "no identity" rather than an error. The workbench owns the
hook and passes the identity to Settings; no second fetch is added.

Settings gains a **Server** section showing the revision (short with the full
hash as a title), build time when present, start time, and the checkout state
(`up to date`, `restart needed` with the checkout's revision, or `not running
from source`). The workbench renders a `Banner tone="warning"` above the view
content when `checkout.staleReason` is non-empty, with the running revision and
the action: restart the SpecFlow server. Dismissal is React state keyed by the
identity's `startedAt` plus `revision`, so a different server process re-shows
the warning, while the pinned `localStorage` keys (`specflow.projects`,
`specflow.project`, `specflow.panelCollapsed`) stay untouched. A reload
re-shows the warning — intentional, since a reload is a good moment to
re-check a running process.

Alternative rejected: persisting dismissal in `localStorage`/`sessionStorage`
(more shell state to reason about for a warning that is cheap to re-show).

### Decision: additive router constructor

`api.NewRouterWithServer(staticFS, registry, engine, server)` accepts the
identity and probe seam; the existing `NewRouter*` constructors delegate with a
zero identity so every current call site and test compiles unchanged, and
`cmd/api` switches to the new constructor. The probe is injected as a function
so handler tests use a fake instead of a real checkout, while `buildinfo`
tests own the git/mtime behavior.

## Risks / Trade-offs

- **mtime false positives** (a `touch`, a branch switch, a formatter that
  rewrites timestamps) → the warning is dismissible, names its reason, and the
  action (restart) is harmless; a reload re-checks.
- **A bare `go run` without the Makefile reports no revision** → the source-mtime
  check still catches uncommitted edits, the toolchain fallback catches
  `go build`, and the Makefile is the documented dev entry point.
- **The probe shells out to git on a page focus** → cached for a few seconds,
  bounded to the recognized checkout, and every failure is silent.
- **A released binary run inside the SpecFlow repository** may report a
  checkout state → correct when it is actually behind; otherwise no report.
  No warning when the tree is unrelated or history is not comparable.
- **Identity discloses commit metadata** → the product is a local single-user
  tool with no auth, and the response carries no paths, file names, or PIDs.
- **Dismissal does not persist** → the warning returns after a reload. This is
  a deliberate trade for avoiding a new stored preference.

## Migration Plan

The route and fields are additive: an older client ignores them, and an older
server makes the new shell show no identity. The Makefile stamp takes effect on
the next `make dev`/`make build`; a process started before the change reports
only what it has (no checkout report). No persisted state, no data migration.
Rollback is a code revert: the route disappears and the shell falls back to
"no identity".
