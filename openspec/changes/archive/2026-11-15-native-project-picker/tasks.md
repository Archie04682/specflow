# Tasks

## 1. Native picker backend

- [x] 1.1 Add `backend/internal/picker` with `Pick`, `ErrCancelled`, and `ErrUnsupported`, plus the macOS implementation that runs `osascript` and classifies the cancelled dialog; verify `go build`/`go vet` and that the command output is trimmed.
- [x] 1.2 Add the Linux (`zenity`), Windows (PowerShell), and unsupported-platform implementations behind build tags; verify `GOOS=linux`, `GOOS=windows`, and `GOOS=darwin` builds of `./backend/...`.
- [x] 1.3 Add `POST /api/project/pick` with the loopback check, loopback-only Origin check, two-minute deadline, and the `200`/`501`/`500` outcomes; register the route; verify `httptest` cases for success, cancellation, unsupported, failure, non-loopback remote, and foreign Origin.
- [x] 1.4 Add the endpoint row to the README API table; verify the row matches the handler path and outcomes.

## 2. Browse in the dashboard

- [x] 2.1 Add the `pickProject` API call and its unit test for a selected path, a cancellation, and an error body.
- [x] 2.2 Build `ProjectPickerField` with the existing label, input, hint, a Browse button, an in-flight state, and a failure explanation that keeps manual entry; add the CSS for the input/button row; verify Testing Library tests for success, cancellation, unavailable, error, and disabled-while-picking.
- [x] 2.3 Use `ProjectPickerField` on the start screen and the deep-link project prompt; verify the existing start-screen tests still pass and both screens expose the Browse action.
- [x] 2.4 Document Browse and the desktop-session requirement in the README dashboard section; verify the documented flow matches the UI.

## 3. Integration

- [x] 3.1 Run `make test` and `make build`, and serve the built binary; verify the start screen renders with Browse and the endpoint refuses a non-loopback request.
- [x] 3.2 Run `openspec validate native-project-picker --strict` and `openspec validate --changes --strict`; verify both pass.
- [ ] 3.3 On macOS with the server in the desktop session, open Browse, choose a folder, and confirm the path appears; cancel and confirm the field is unchanged. Record the result.

## Workflow follow-up

- Archive the change after the manual dialog check on a desktop session is recorded.
- Keep the manual project path and the `501` fallback documented for headless servers.
