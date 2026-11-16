# Proposal

## Why

Choosing a project today means typing an absolute path. A browser cannot supply
one: `showDirectoryPicker()` exposes a directory handle whose only readable name
is the folder's display name, and `webkitdirectory` returns paths relative to the
chosen root. The server, however, runs on the same machine as the browser in the
local single-binary deployment, so it can open the operating system's folder
dialog and return the absolute path the API already requires.

## What Changes

- Add `POST /api/project/pick`: a loopback-only endpoint that opens the native
  folder dialog (macOS `osascript`, Linux `zenity`, Windows PowerShell) and
  returns the chosen absolute path, or a cancellation.
- Add a **Browse…** action beside every project path field. On success it fills
  the field; on cancellation it changes nothing; when the dialog is unavailable
  or fails, manual entry remains and the UI explains the fallback.
- Keep the manual path field and its `localStorage` persistence exactly as they
  are. Do not add a directory-listing endpoint and do not mutate the project.
- Document the endpoint and the desktop-session requirement.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `run-dashboard`: project selection gains a native folder dialog, backed by a
  local-only endpoint that never lists directories.

## Impact

- `backend/internal/picker`: a small per-OS package with build tags; unsupported
  platforms report the capability as unavailable.
- `backend/internal/api`: a new handler and route, its loopback/Origin/timeout
  checks, and `httptest` coverage. Existing run and workflow endpoints are
  unchanged.
- `frontend/src`: a `ProjectPickerField` used by the start screen and the
  deep-link project prompt, plus the API call and tests.
- `README.md`: API table row and dashboard usage.
- Operational: the dialog needs the server to share the user's desktop session.
  Over SSH, in a container, or on a server without a desktop, the endpoint
  returns `501` and the UI falls back to manual entry.
