# Harness adapters

`backend/internal/harness` defines the contract every adapter implements.
`backend/internal/harness/pi` is the first adapter. Adding a harness must not
change the orchestrator.

## The contract

- **One terminal result per execution.** Completion is the adapter's return
  value, never a stream event. (`agent_end` is not the end of a Pi run — only
  `agent_settled` is.)
- **Capability-gated interaction.** Questions and injection are available only
  when the adapter declares the capability. A node's needs are checked before it
  starts.
- **Best-effort telemetry.** Streamed text, tool calls, and file changes are
  useful but not required; an adapter may omit them.
- **One failure class** from a fixed set: `adapter_missing`, `unsupported`,
  `config`, `auth`, `provider`, `start`, `protocol`, `harness_exit`, `timeout`,
  `cancelled`.
  Authentication and configuration failures are not retryable. Provider, start,
  protocol, harness-exit, and timeout failures are.
- **An in-band error is not a completion.** When Pi settles after an assistant
  message with an error stop reason, the execution fails with that message even
  though the process exited normally.

## Optional catalog

An adapter may implement `harness.CatalogProvider`; the harness listing then
exposes models, thinking levels, and tools to the editor. A catalog failure is
non-fatal and appears as `catalogError` on the listing. Do not widen `Adapter`
for this.

## Pi

Pi runs as a child process in RPC mode. The adapter owns its lifecycle, framing,
and shutdown escalation.

- Interactive nodes work because the binary embeds a Pi extension
  (`backend/internal/harness/pi/bridge/specflow-bridge.ts`) that registers a
  `specflow_ask` tool. The extension is materialized into the user cache
  directory at runtime, which keeps single-binary distribution.
- The adapter builds its catalog from Pi's local agent configuration without
  starting a runtime.
- Every bound skill is confirmed with `get_commands` before prompting. A
  directory Pi accepts but does not load — for example a `SKILL.md` without a
  description — fails as configuration; a Pi too old to answer degrades to a
  warning.
- Pi loads every selected skill path but only expands one `/skill:` command, so
  the adapter inlines every `SKILL.md` body in Pi's skill-block shape before the
  node prompt. A single skill keeps the native `/skill:` expansion.
- Pi sends a tool's arguments on `tool_execution_start` and omits them from
  `tool_execution_end`; the translator remembers them per call so `write` and
  `edit` still produce file-change telemetry.
- Pi's dialog protocol carries no description for `select`, `input`, or
  `editor`, so the translator folds the `specflow_ask` call's `question`
  argument into the ask's message; `confirm` keeps the dialog's own message.
  Tool summaries are bounded at the stream's 16k limit.

## Testing without Pi

The adapter is tested against a scripted `pi` stand-in (`testdata/fake-pi`), so
`make test` needs no Pi installation:

```bash
PI_SMOKE=1 go test ./backend/internal/harness/pi/ -run TestPiSmoke -v
```

It skips unless `PI_SMOKE=1` is set and `pi` is on `PATH`.

## End-to-end ladder

`backend/internal/e2e` drives the real binary through the HTTP API and SSE. It is
opt-in because it makes real model calls:

```bash
export PI_E2E=1
export PI_E2E_MODEL=provider/cheap-model
go test ./backend/internal/e2e/ -v
```

`PI_E2E_MODEL` must name a cheap model; the ladder's cost is driven by that
choice, and every run logs the `pi --version` it used. `PI_E2E_FULL=1` also runs
the full `standard-feature` workflow, which is slower and interactive. The suite
skips without the gate and is never part of `make test`.

When Pi's config directory is not the default, or the default is not writable,
point `PI_CODING_AGENT_DIR` at a writable directory and authenticate against
that same directory:

```bash
export PI_CODING_AGENT_DIR=/tmp/specflow-pi-agent
pi auth login <provider>
```

If the Go build cache is not writable, set `GOCACHE` to a writable path too.

### Success rungs

| Test | What it proves |
| :--- | :--- |
| `TestRung1OneNode` | argv, RPC handshake, SSE telemetry, terminal result, worktree removal, clean shutdown |
| `TestRung2SkillWritesFile` | skill binding, tool and file events, checkpoint commit, `file_exists` |
| `TestRung3ChangeChain` | worktree persistence, `{{change}}`, `new_change`, `tasks_complete`, checkpoint order |
| `TestRung4aAsk` | `question.asked` → `question.answered`, the bridge round trip, and Pi continuing with the answer |
| `TestRung4bInject` | `injection.received` → `injection.acknowledged`, a steer delivered into a running tool call |
| `TestRung4cApproval` | `approval.required` before `node.started`; approval runs the node, rejection cancels without spawning Pi |

The interactive rungs use an SSE auto-operator: it approves gates, answers
questions, and injects when a turn starts, all through the same HTTP endpoints a
client uses.

### Failure rungs

| Test | Scenario | Class | Recovery |
| :--- | :--- | :--- | :--- |
| `TestRung5StartupFailures` | unknown model / missing credentials | `config` / `auth`, not retryable | fix the model or authenticate |
| `TestRung5Cancel` | cancel a live node | `cancelled` | resume |
| `TestRung5Timeout` | node deadline | `timeout`, retryable | retry or raise `timeout` |
| `TestRung5Crash` | harness killed mid-run | `harness_exit`, retryable | retry forward or resume |
| `TestRung5Postcondition` | harness succeeds, repository disagrees | `postcondition`, not retryable | fix the work; no automatic retry |

The product scenario runs the canonical template against the toy OpenSpec
project with the real OpenSpec skills, answers and approves through the API, and
asserts that the merged tree passes its own tests and the change is archived:

```bash
PI_E2E_FULL=1 go test ./backend/internal/e2e/ -run TestFullStandardFeature -v
```

It takes minutes and makes many model calls; it is never part of `make test`.

## Adding an adapter

1. Implement the `harness.Adapter` interface in `backend/internal/harness/<name>`.
2. Register it in the harness registry; do not add harness-specific branches to
   the orchestrator.
3. Map failures onto the fixed class set and mark retryability honestly.
4. Optionally implement `CatalogProvider` for editor dropdowns.
5. Test against a scripted stand-in so `make test` never needs the real tool.
