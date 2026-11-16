# Tasks

## 1. Contract types

- [x] 1.1 Create `backend/internal/harness/contract.go` with `Adapter`, `Host`, `Request`, `Result`, `Event`, `Ask`, `Answer`, `Injection`, `Capabilities`, `Descriptor`, `Failure`, and the `Status`/`Kind`/`FailureClass` constants from `design.md`; verify `go build ./backend/internal/harness/` and `go vet ./backend/internal/harness/` pass.
- [x] 1.2 Enforce the D3 invariant in code: document on `Adapter.Run` that the error is non-nil exactly when `Result.Status != completed`, and add a helper `func (r Result) Err() error` returning a wrapped `*Failure`; verify a unit test asserts `Err()` is nil for `completed` and errors for every other status.
- [x] 1.3 Add `contract_test.go` asserting JSON stability for every message type: round-trip `Request`, `Event` (one per `Kind`), `Ask`, `Answer`, `Result`, `Failure`; verify `go test ./backend/internal/harness/ -run TestContractJSON` passes and that a golden-key check fails if a field's JSON name changes.

## 2. Registry and preflight

- [x] 2.1 Add `backend/internal/harness/registry.go` with `Registry` (`Register(factory)`, `Lookup(id)`, `List()`), rejecting duplicate identifiers and returning a typed error for an unknown harness id; verify table-driven tests cover register, duplicate, lookup, unknown, and list ordering stability.
- [x] 2.2 Add node-needs derivation and preflight validation: `Needs` from node configuration (`interactive`, `allow_inject`, `skill`, `model`, `tools`) and `Validate(Needs, Capabilities) *Failure`; verify tests cover each unmet need returning `unsupported` naming the missing capability, and the fully satisfied case returning nil.
- [x] 2.3 Add a registry test proving capabilities are available without executing work — `Capabilities()` and `List()` must not start a process or call `Run`; verify with a factory that panics if `Run` is called.

## 3. Scripted mock adapter

- [x] 3.1 Add `backend/internal/harness/mock` implementing `Adapter` with a scriptable event sequence, an optional ask, and a terminal result; verify a test drives `Run` with a recording `Host` and observes events in order plus the expected `Result`.
- [x] 3.2 Verify blocking semantics with the mock: a `Host.Emit` that blocks for a fixed duration must slow the adapter without data loss or reordering; verify the test asserts all events are received in sequence order.
- [x] 3.3 Verify ask and injection semantics with the mock: `Host.Ask` returning an answer reaches the adapter return value, and an `Injections` channel delivery produces one acknowledgement event; verify both tests pass with a cancelled-ask variant that must not deadlock.

## 4. Pi transport

- [x] 4.1 Add `backend/internal/harness/pi/client.go` implementing a transport over an `io.ReadWriteCloser`: write one JSON object plus `\n`, read records split on `\n` only; verify table tests cover plain LF, CRLF input, and a JSON string containing `U+2028`/`U+2029` that must not be split.
- [x] 4.2 Implement command/response correlation by `id` with per-command timeout: a response with an unknown id is ignored, a response without an id is ignored unless it is a parse error, and a missing response returns a timeout-class failure; verify with an in-memory pipe and a table of sequences.
- [x] 4.3 Implement bounded stderr capture: a ring buffer that keeps the most recent N bytes and never blocks the reader; verify a test writes stderr far larger than the bound and asserts the buffer holds the tail and the reader completes.
- [x] 4.4 Raise the record-size limit for stdout lines: verify a 4 MiB single record is read intact (a tool result can be large) and that the read path does not silently truncate.

## 5. Pi record translation

- [x] 5.1 Add `backend/internal/harness/pi/translate.go` translating text and reasoning deltas, `text_end`/`thinking_end` authoritative blocks, and turn boundaries; verify table tests over recorded fixtures assert the exact `Kind` sequence and payloads.
- [x] 5.2 Translate tool lifecycle to started/progress/finished with `toolCallId` correlation, and derive a file-changed telemetry event when the finished tool is `write` or `edit` and its arguments carry a path; verify fixtures cover read (no file event), write, edit, and a failed tool (`isError: true`) producing a finished event with the error flag.
- [x] 5.3 Translate cumulative usage, `compaction_*`, retry records, and `agent_end` into telemetry or notices — and assert `agent_end` does **not** terminate the execution; verify a fixture sequence ending in `agent_end` without `agent_settled` keeps the translator in the running state.
- [x] 5.4 Translate `extension_ui_request` dialogs (`select`, `confirm`, `input`, `editor`) into `Ask` values, and `notify`/`setStatus` into notice/status; verify fixtures assert the ask kind, options, timeout, and that fire-and-forget methods produce no `Ask`.
- [x] 5.5 Translate unknown record types into one notice per type: verify feeding the same unknown type three times yields exactly one notice event.

## 6. Pi adapter lifecycle

- [x] 6.1 Add `backend/internal/harness/pi/testdata/fake-pi` as a Go program speaking Pi's RPC: it answers `get_state` and `prompt`, replays a record script selected by an environment variable, and can be instructed to ignore stdin close for escalation tests; verify `go run ./backend/internal/harness/pi/testdata/fake-pi` answers a `get_state` line piped to stdin with a JSON response.
- [x] 6.2 Add `backend/internal/harness/pi/adapter.go` building argv from `Request` (`--mode rpc --no-session`, model/thinking, `--tools +specflow_ask` or an explicit list, `-ns --skill <abs path>` when a skill is bound, trust flag, `--offline`); verify table tests assert the exact argv for: default node, model+thinking, explicit tools, bound skill, trusted node, and offline node.
- [x] 6.3 Implement `Probe`: locate the executable, read `--version`, parse a semver, compare against the minimum constant, and confirm the bridge extension loads; verify tests cover missing executable (`adapter_missing`, non-retryable), old version (`config`, message names the minimum), and success.
- [x] 6.4 Implement the happy path: start the process, drain stderr, run the `get_state` handshake with a startup timeout, submit the prompt with the skill prefix when bound, consume records until `agent_settled`, and assemble `Result` (text, turns, tool count, usage, file changes); verify against fake-pi that the result text, turn count, and usage match the script.
- [x] 6.5 Implement failure paths with typed classes: exit before settle → `harness_exit` with stderr attached; malformed record before settle → `protocol`; `prompt` response `disposition: "handled"` → `protocol` with a copy-pasteable cause (no hang); handshake never answered → `start`; verify one test per class asserts the class, retryability, and that no goroutine or process leaks.
- [x] 6.6 Implement shutdown escalation: close stdin, wait the grace period, send SIGTERM, wait again, then kill; reap in every path; verify a fake-pi run that ignores stdin close exits within the bound and that no process with the test's pid remains (`syscall.Kill(pid, 0)` returns ESRCH).
- [x] 6.7 Add `smoke_test.go` running the adapter against a real `pi` only when `PI_SMOKE=1` is set and a `pi` binary is on `PATH`, so it skips cleanly otherwise; verify `go test ./backend/internal/harness/pi/ -run TestPiSmoke` reports `SKIP` on this machine and the rest of the suite stays green.

## 7. Interaction: questions, injection, cancellation

- [x] 7.1 Add `backend/internal/harness/pi/bridge/specflow-bridge.ts` registering the `specflow_ask` tool (`question`, `kind`, `options`, `title`, `placeholder`) that calls `ctx.ui.select`/`confirm`/`input`/`editor` and returns the answer as tool text; verify the file uses only a type-only import and declares parameters as a plain JSON Schema object.
- [x] 7.2 Add `backend/internal/harness/pi/bridge.go` embedding the extension with `//go:embed` and materializing it to a content-addressed path under the user cache directory (`0600`, atomic rename, temp-dir fallback); verify a test asserts the file content matches the embedded bytes, the mode is `0600`, and two calls with identical content return the same path.
- [x] 7.3 Wire `Ask`: dispatch dialogs off the reader goroutine, call `Host.Ask`, write `extension_ui_response` (`value` or `confirmed`), and write `cancelled: true` when the ask deadline or context expires; verify a fake-pi ask scenario answers round-trip and that a late answer for a cancelled ask is discarded.
- [x] 7.4 Verify cancellation remains responsive while an ask is pending: cancel the context during an open dialog, assert `abort` is sent, `cancelled: true` is written, and the outcome is `cancelled` within the bound.
- [x] 7.5 Wire injection: translate a queued `Injection` to `steer` while a turn is running and to `follow_up` when idle, and emit an acknowledgement carrying the returned disposition; verify a fake-pi scenario asserts command choice per state and one acknowledgement per injection.
- [x] 7.6 Wire cancellation: on context cancellation send `abort`, wait for `agent_settled` within the grace period, then escalate per 6.6, and report `cancelled` rather than `failed`; verify a fake-pi run cancelled mid-stream yields `cancelled` with no orphaned process.

## 8. HTTP surface and documentation

- [x] 8.1 Add `GET /api/harnesses` returning `{id, name, version, protocol, ready, readyError, capabilities}` per registered harness, calling `Probe` with a short timeout and degrading to `ready: false` with the reason instead of failing the request; verify `httptest` cases for a ready harness, a not-ready harness (reason present), and the JSON not-found handler for an unknown subpath.
- [x] 8.2 Register the Pi adapter factory and the new route in `cmd/api/main.go`; verify `go run ./backend/cmd/api --port 18080` serves `curl -s localhost:18080/api/harnesses` listing `pi` with `ready: false` on a machine without Pi, and that `/` and `/api/health` are unchanged.
- [x] 8.3 Update `README.md`: mark Pi as an optional runtime prerequisite, document `GET /api/harnesses`, and document how to run the real-Pi smoke test (`PI_SMOKE=1 go test ./backend/internal/harness/pi/ -run TestPiSmoke`); verify every command shown runs as written on this machine (smoke test skips).
- [x] 8.4 Update `AGENTS.md`: replace the bootstrap "current phase" statement, describe the harness contract's three signal classes and terminal-result rule, and record the Pi bridge extension as the reason `interactive` nodes work; verify no statement in the file is contradicted by `go doc ./backend/internal/harness` and that the file stays under 220 lines.

## 9. Integration verification

- [x] 9.1 Run `make test` and confirm `go test ./...` plus the frontend typecheck pass; verify no test requires Pi, network access, or credentials.
- [x] 9.2 Run `go vet ./...` and `gofmt -l backend/`; verify both are clean and `git diff go.mod` is empty (no new dependencies).
- [x] 9.3 Run `openspec validate harness-protocol-pi-adapter --strict` and confirm it passes with no warnings.
- [x] 9.4 Confirm the single-binary promise still holds: `make build` produces `./specflow`, copying it to an empty temporary directory and running it there serves `/` and `/api/harnesses`; verify the embedded bridge is materialized at runtime when a Pi execution starts, not at build time.

## Workflow follow-up

- Archive this change after review and a green `openspec validate --strict` and `make test`.
- The next changes consume this contract: the session manager implements `Host.Ask` for real, and the orchestrator implements preflight and the DAG that calls `Run`.
- Install Pi on a development machine and run the smoke test; use the result to tighten the minimum supported version constant and, if needed, the bridge extension against a real Pi extension API.
- Revisit run-scoped harness sessions only if a real workflow exposes a context problem that prompt-passing cannot solve.
