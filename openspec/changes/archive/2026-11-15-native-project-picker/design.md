# Design

## Context

See `proposal.md` for motivation and `specs/run-dashboard/spec.md` for the
behavior contract. Constraints that shape the approach:

- The browser cannot yield an absolute path. `FileSystemDirectoryHandle.name` is
  a display name; `webkitRelativePath` is root-relative. The API needs an
  absolute project root for `workspace.Open`.
- The server is local: `./specflow` serves the UI and binds `:8080`. It has no
  authentication and no privileged filesystem surface today.
- The project path is already client state: typed into a field, persisted under
  `specflow.project`, and sent as `?project=`.
- Platform support must not complicate the single binary: no cgo, no GUI toolkit
  dependency. Each OS already ships a way to open a folder dialog from a shell.
- `make test` must not open a real dialog.

## Goals / Non-Goals

**Goals:**

- A real folder-picker experience on the machine running the server.
- A typed, testable endpoint with a clean cancellation path and a fallback.
- No directory-listing API and no new filesystem attack surface beyond a
  loopback-only dialog invocation.

**Non-Goals:**

- Browsing or enumerating directories from the web UI.
- A server-owned project or a recent-projects registry.
- Supporting remote servers, SSH sessions, or headless containers; those fall
  back to typing.
- Selecting individual files; this is a project directory chooser.

## Decisions

### D1 — A native dialog, not a directory browser

Alternatives: an in-app directory browser backed by a new listing endpoint, or a
server default plus recents.

A directory-listing endpoint on an unauthenticated server bound to all
interfaces would expose the host filesystem to the network. Restricting it to
loopback and a root allowlist is possible, but it is still a new capability to
secure and reason about. The native dialog invokes an OS affordance that already
exists, returns one path, and adds no enumeration. It fails cleanly where no
desktop session exists. The in-app browser remains a possible later capability
for remote deployments.

### D2 — One interface, per-OS implementations behind build tags

`internal/picker` exposes:

```go
var ErrCancelled = errors.New("folder selection cancelled")
var ErrUnsupported = errors.New("no native folder dialog is available")
func Pick(ctx context.Context) (string, error)
```

- macOS: `osascript -e 'POSIX path of (choose folder with prompt "...")'`.
- Linux: `zenity --file-selection --directory`; a missing `zenity` is
  `ErrUnsupported`, not a crash.
- Windows: PowerShell `System.Windows.Forms.FolderBrowserDialog`.
- Other platforms: `ErrUnsupported`.

The platform files are compile-checked with `GOOS` builds; the API tests stub the
function so no dialog opens during `make test`. No user-controlled text reaches
the shell, so quoting is fixed.

### D3 — The endpoint is POST, loopback-only, Origin-checked, and bounded

`POST /api/project/pick`:

- POST prevents link prefetching and accidental navigation from opening a dialog.
- Reject a non-loopback `RemoteAddr` (`127.0.0.1`, `::1`, `localhost`) with
  `403`.
- When an `Origin` header is present, require a loopback host (any port, so the
  Vite dev proxy on `:5173` works) with `http`/`https`. Refuse otherwise.
- Run the dialog with a two-minute context deadline; a hung dialog cannot pin a
  request forever.
- Outcomes: `200 {"path": "/abs"}`; `200 {"cancelled": true}`; `501 {"error"}`
  for unsupported; `500 {"error"}` otherwise.
- No `project` parameter, no project mutation, no directory listing.

### D4 — The manual field stays the primary contract

The browse button writes into the same project state the field edits, so
persistence, request parameters, and the start/deep-link flows are unchanged. A
cancelled or unavailable dialog leaves the typed value in place. The field shows
a compact explanation when the endpoint fails, and the operator can always type
the path.

### D5 — Test seams

The handler calls a package-level `pickFolder` variable initialized to
`picker.Pick`; tests replace it with a stub. Handler tests override
`RemoteAddr` (httptest defaults to `192.0.2.1`, which must be refused) and cover
success, cancellation, unsupported, failure, remote refusal, and foreign Origin.
The OS packages are built, not invoked, by tests.

## Risks / Trade-offs

| Risk | Mitigation |
| :--- | :--- |
| The server has no desktop session (SSH, container, launchd daemon) | The endpoint returns `501`/`500`; the UI keeps manual entry and says why. |
| A malicious local page POSTs to the loopback endpoint to open a dialog | Origin is required to be loopback when present; the response is unreadable cross-origin; the dialog is cancellable. |
| A dialog hangs or is never dismissed | Two-minute context deadline; the command is killed with the request. |
| Linux has no `zenity` | Report unsupported; no partial or surprising fallback. |
| Platform command output differs or includes a trailing newline | Trim whitespace; treat an empty path as cancellation. |
| Tests accidentally open a real dialog | The handler depends on a stubbed function; no test calls `picker.Pick`. |

## Migration Plan

Additive endpoint and UI affordance. No persistence or API changes. Rollback is
the route and component removal; typed paths continue to work throughout.

## Open Questions

None. A remote directory browser is a deliberate non-goal, not a deferred
decision needed for this change.
