# Design

## Context

The contract was probed against the real Pi 1.5.2 binary before this design was
written, with no model calls:

| Probe | Result |
| :--- | :--- |
| `buildArgs` flags | All present in `pi --help` |
| Bridge extension via `-e` | Loads; emits `specflow-bridge:ready` |
| `get_state` | Answers; `data.sessionId` is present |
| `--no-skills --skill <dir>` | Registers `skill:openspec-explore` from the absolute path |
| Prompt with no credentials | `success:false`, "No API key found for the selected model" |
| Prompt with an invalid model | Pi exits at startup; stderr names the model |
| Malformed `SKILL.md` (no description) | Pi silently loads nothing; no stderr warning |

The last row is the important one: the compile step only checks that the skill
directory exists, so a directory that Pi refuses to load passes preflight and
the prompt names a skill that does not exist.

Environment constraints for any run started from this workspace:

- `PI_CODING_AGENT_DIR` must point at a writable directory (the sandbox blocks
  `~/.pi`); use `/tmp/specflow-pi-agent` or equivalent.
- `GOCACHE` must point outside `~/Library/Caches/go-build` for the same reason.
- `make test` must stay hermetic: no Pi, no credentials, no network.

See `proposal.md` for motivation. The requirements are in
`specs/harness-pi-adapter/`, `specs/harness-protocol/`, and
`specs/workflow-orchestration/`.

## Goals / Non-Goals

**Goals:**

- Every path that can end a real execution is classified truthfully: provider
  error, authentication, configuration, timeout, cancellation, or crash.
- A node that cannot do what it was configured to do fails before prompting.
- No execution runs without a deadline, and no deadline is reported as a user
  cancellation.
- An opt-in ladder proves the full path — HTTP, SSE, worktree, checkpoints,
  interactions — against real Pi, at a cost that starts at one model call.

**Non-Goals:**

- Changing the HTTP surface, the workflow schema, or the persistence layout.
- Running real Pi in `make test` or CI by default.
- Runtime gating of `specflow_ask` on `interactive: false`; that is a separate
  product decision (see Open Questions).
- Supporting a second harness.

## Decisions

### Detect in-band errors in the message stream, not in the process exit

Pi settles normally after a provider error: `agent_settled` carries only
`aborted`, and `agent_end` carries `willRetry`. The reliable signal is the
assistant message: `message_end.message.stopReason == "error"` with
`errorMessage`. The translator records the first such error (also from the
`message_update` `error` variant, which Pi admits but normally rewrites) and the
terminal result becomes a failure at settle.

Alternative rejected: failing on any `agent_end` would kill retries, overflow
recovery, and queued work, which the contract explicitly allows after
`agent_end`. Alternative rejected: scanning assistant text for error phrases;
it is unbounded and model-dependent.

Classification is a small, tested table over the error text: credentials and
401/403 → `auth`, non-retryable; permanent request errors (400/404, unknown
model, invalid request) → `config`, non-retryable; everything else → `provider`,
retryable. The adapter sets `Retryable` explicitly rather than deriving it from
the class, because provider errors are the one class whose retryability varies.

### Add `provider` to the contract's failure classes

`harness.FailureClass` currently has no honest class for "the model provider
returned an error after accepting the prompt". Reusing `protocol` would make a
429 look like a malformed record, and reusing `config` would make a transient
outage look permanent. A ninth class is additive for string consumers and is
required by the retry policy.

### Trust Pi's command registry, not the filesystem, for skill binding

After `get_state` and before `prompt`, an execution with a bound skill calls
`get_commands` and requires `skill:<name>` in the response. Pi is the authority
on whether a skill loaded; a `SKILL.md` without a description is silently
ignored (observed), and static parsing would duplicate Pi's rules. The check
costs one RPC round trip and no model call, and it catches a frontmatter `name`
that disagrees with the directory name — a case where `/skill:<dir>` would not
resolve.

If `get_commands` is unsupported (an older Pi), the adapter emits a warning
notice and continues, so the minimum supported version is unaffected.

### The engine owns the agent deadline; the adapter only distinguishes it

`runAgent` wraps the execution context with the node's compiled timeout and
passes it to `adapter.Run`. The adapter already stops Pi when its context ends;
it now checks whether the context ended by deadline (`StatusTimedOut`,
`FailureTimeout`, retryable) or by cancellation (`StatusCancelled`,
`FailureCancelled`, not retryable). The engine's existing check on its own
context still decides run cancellation, so a node deadline does not cancel the
run.

Alternative rejected: a watchdog goroutine in the adapter. Context is the
project's cancellation convention, it composes with run cancellation, and it
covers startup and prompt acceptance for free.

### One filter for the bridge marker, in the translator

The translator's `handleUI` ignores a `notify` whose message is
`BridgeMarker`. Both the normal event loop and the cancellation drain use it, so
the handshake stops appearing as `node.event` telemetry in either path. `Probe`
keeps its own detection loop.

### Opt-in, in-process e2e tests with deterministic fixtures

The ladder lives in a separate `backend/internal/e2e` package and builds the
real router, engine, registry, and Pi adapter in-process, behind `httptest`.
That gives the test access to node records, goroutine counts, and process IDs
while still exercising the HTTP and SSE surfaces. It skips unless `PI_E2E=1`
and `PI_E2E_MODEL` are set, and logs `pi --version` on every run.

Rungs 1–5 use purpose-built fixture skills whose `SKILL.md` instructs a single
tool call, with `tools:` narrowed so the model has little choice. This isolates
plumbing from model judgment; a failure means the path is wrong, not that the
model was creative. The real OpenSpec skills are reserved for the final rung.

```
rung  fixture / vehicle                     proves
----  ------------------------------------  ---------------------------------
 0    GET /api/harnesses                    binary, version, bridge
 1    prompt "reply ok", no skill           argv, RPC, SSE, shutdown
 2    fixture skill -> write a file         skill binding, tool/file events,
                                            checkpoint, file_exists
 3    two nodes; node 2 creates a change    worktree persistence, {{change}},
                                            new_change / tasks_complete
 4a   skill forced to call specflow_ask     question round trip
 4b   slow bash tool + injection            steer vs follow_up, ack
 4c   require_approval node                 pause, approve, reject
 5    shim: SIGKILL, bad model, cancel      classes, resume, no orphans
 6    standard-feature.yaml (PI_E2E_FULL=1) the product scenario
```

### A `pi` shim as the adapter binary, not a PATH trick

For crash and slow-process cases the test constructs the adapter with
`Binary: <shim>`; the shim records its PID and `exec`s the real `pi`. No PATH
mutation, no second server, and the adapter's `LookPath` contract is untouched.
The shim is a shell script; tests skip on Windows.

### An SSE auto-operator, because the full workflow is interactive

`propose` and `merge` are approval-gated and `explore`/`apply` are interactive.
The e2e package runs a goroutine that reads the SSE stream and answers
`question.asked` from a rule table, approves or rejects `approval.required`, and
optionally injects on `node.started`. This is the same component for rungs 4a,
4c, and 6; it also proves the API round trip rather than bypassing it.

## Risks / Trade-offs

- **Error-text classification is brittle** → keep the table small, default to
  `provider` with the full text in `Detail`, and cover it with fixtures captured
  from real Pi. Revisit if Pi exposes structured error codes.
- **`get_commands` is an extra round trip on every skill node** → it is cheap
  and only runs when a skill is bound; a future Pi without it degrades to a
  warning.
- **A 30-minute default now kills agent nodes** → the default is unchanged and
  already compiled; nodes can set `timeout`. Document it in the README.
- **Real-Pi tests are slow, flaky, and cost money** → opt-in env, pinned cheap
  model, small fixtures, one subtest per rung, and a separate `PI_E2E_FULL` gate
  for the full workflow. `make test` remains hermetic.
- **Provider errors may arrive as `length` or `aborted` stop reasons** → only
  `error` fails the execution; `length` is a normal completion with truncated
  output, and `aborted` is already handled by the settle flag.
- **Pi versions drift** → the adapter's minimum-version gate and the smoke test
  stay; the e2e helper logs the version in every failure message.

## Migration Plan

No data migration. Failure classes are strings in existing payloads; `provider`
is additive. Rollback is a revert. Runs created before the change keep their
snapshots and behave as before on resume.

## Open Questions

- Should a non-interactive node reject `specflow_ask` at runtime instead of
  answering it? Today `interactive` is a preflight declaration only. This does
  not change the specs or the ladder, so it can be decided after the first real
  runs.
- Should the ladder run rung 1 in CI with a secret key? Deferred; the default
  stays opt-in.
- Should the run record store the harness version for reproducibility? Deferred;
  logged by the tests for now.

## Implementation Findings

Real runs found four things the stand-in could not:

- Pi sends tool arguments on `tool_execution_start` and omits them from
  `tool_execution_end`. File-change telemetry was therefore empty against the
  real binary; the translator now remembers each call's arguments.
- The canonical template passed neither `{{objective}}` nor `{{change}}` to the
  nodes that needed them, so explore, propose, apply, and archive started with
  no context. The template now renders both.
- Bare `openspec validate --strict` reports "Nothing to validate" even with an
  active change. The canonical verify gate now validates the run's change
  (`openspec validate "{{change}}" --strict`); the repository-wide gate is
  `--changes`.
- The archive node was model-dependent: the skill completed without archiving
  on one run and archived on another. Archiving is now a command
  (`openspec archive "{{change}}" --yes`) with an `exit_zero` postcondition, so
  a completed run cannot silently skip it.

These are template and adapter fixes, not new capabilities; the specs already
require run context and accurate file telemetry.

## Verification Evidence

- `go test ./backend/internal/e2e/ -count=1 -v` with `PI_E2E=1
  PI_E2E_FULL=1 PI_E2E_MODEL=deepseek-flash`: every rung passed, including
  `TestFullStandardFeature` (269.13s in the full sweep; 137.48s alone).
- The full run completed all eight nodes, resolved the change
  `add-farewell-function`, merged it onto `main`, passed `go test ./...` in the
  merged tree, and left no active change after archive. The same run's event
  stream covered every node.
- `go test ./... -count=1`, `go vet ./...`, `pnpm exec tsc --noEmit`, and
  `openspec validate --changes --strict` all pass.
- Environment: Pi 1.5.2, DeepSeek `deepseek-flash`, agent directory at
  `PI_CODING_AGENT_DIR=/tmp/specflow-pi-agent` because the sandbox cannot write
  `~/.pi`.
