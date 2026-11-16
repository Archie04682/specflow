# Design

## Context

See `proposal.md` — Why. What matters for the approach:

- The repository is the bootstrap shell: `backend/internal/harness` contains only a `doc.go`, there is no orchestrator, and the only durable capability is `single-binary-distribution`.
- `AGENTS.md` binds the orchestrator to an adapter interface and the product to one executable. Both constrain this design.
- Pi is **not installed** on the development machine, so the adapter must be verifiable without it and any real-Pi check must be optional.
- Pi's integration surface is documented and versioned: `pi --mode rpc` speaks JSONL commands on stdin and records on stdout, correlates `command`/`response` by `id`, emits session events, and forwards extension dialogs through an `extension_ui_request`/`extension_ui_response` sub-protocol. Stdout is reserved for protocol records; diagnostics go to stderr; Pi honors stdout backpressure; an orderly shutdown starts when stdin closes, and escalates on SIGTERM/SIGHUP.

Prior art considered and corrected: the ideation chat's `HarnessAdapter` sketch. It proposed `Start`/`Stop`/`Run` on a long-lived adapter, a flat `Event.Data map[string]any`, and event types `progress`, `diff`, `done`, and `error`. Each is wrong for a harness we do not control: `progress` is not knowable by a harness, `diff` is not something Pi emits, `done`/`error` duplicate the return value, and `any` forces every consumer to re-parse strings. Its example invocation (`pi --json --print --prompt`, `--allowedTools`) does not exist in Pi's CLI. It also assumed Pi asks the user by itself, which it does not.

## Goals / Non-Goals

**Goals:**

- One contract that an adapter for a different runtime could satisfy without touching the orchestrator.
- Completion, cancellation, questions, and injection have exactly one defined mechanism each.
- The contract is testable with no harness installed, and the Pi adapter is testable with no Pi installed.
- A node's requirements are validated against the harness before it runs.
- Single-binary distribution is preserved, with Pi remaining a separately installed harness.

**Non-Goals:**

- No DAG execution, session manager, SSE, persistence, or UI. These consume the contract in later changes.
- No run-scoped or resumable harness sessions; every execution is independent.
- No subprocess wire protocol for third-party adapters. The message shapes are JSON-stable so it can be added later, but nothing in this change implements it.
- No mid-execution model or tool reconfiguration.
- No sandboxing. Pi's own tool permissions are not a substitute for isolation; isolation is a separate concern.

## Decisions

### D1 — In-process Go contract; JSON-stable messages

The orchestrator talks to adapters through a Go interface, and adapters are compiled into the binary. Every message type (`Request`, `Event`, `Ask`, `Answer`, `Result`, `Failure`) marshals to stable JSON.

- **Why:** the only adapter is Go around a subprocess. A wire protocol between orchestrator and adapter would add a second process, a second set of lifecycle failures, and multiplexing of `Ask` over a transport — for no capability this change needs.
- **Alternative rejected — subprocess adapters now:** language-agnostic and crash-isolated, but every adapter would still be a translator for the foreign harness, so the wire protocol buys distribution, not integration. It is a later change, and JSON-stable messages keep it open.
- **Cost:** third-party adapters must be Go and compiled in.

The interface:

```go
type Adapter interface {
    Descriptor() Descriptor
    Capabilities() Capabilities
    Probe(ctx context.Context) (ProbeResult, error)
    Run(ctx context.Context, req Request, host Host) (Result, error)
}

type Host interface {
    Emit(Event) error
    Ask(ctx context.Context, ask Ask) (Answer, error)
    Injections() <-chan Injection
}
```

`Run` covers one node execution end to end. `Host` carries everything the adapter cannot decide alone: telemetry, questions, and user messages. `Emit` is allowed to block; that is how backpressure reaches the harness, which Pi honors — the orchestrator is responsible for consuming promptly.

**Invariant:** `Run` returns a non-nil error exactly when `Result.Status != completed`, and that error wraps a typed `*Failure`. No other error path exists.

### D2 — One exchange per node execution, one process per exchange

Each node execution starts a fresh Pi process and ends it when the node settles.

- **Why:** node boundaries become process boundaries. Retry, cancellation, and resource cleanup are trivial; no node inherits context the orchestrator did not deliberately pass; harness state cannot silently cross phases; parallel nodes are independent processes.
- **Alternative rejected — one long-lived session per run:** better context continuity and lower startup cost, but node boundaries blur (`agent_settled` is not a node boundary), single-node retry requires session forking, and two harnesses cannot be mixed within a run. That defeats the product's harness-per-node premise.
- **Cost:** ~1–2 s process startup per node, and inter-node context must be rendered into prompts by the orchestrator — which is the intended architecture anyway.

### D3 — Completion is a return value, not an event

There is no `done` and no `error` event. `Run` returns exactly one `Result` with a status of `completed`, `failed`, `aborted`, `timed_out`, or `cancelled`, plus text, turns, tool count, usage, and file changes.

- **Why:** two sources of truth produce two answers. A stream can end abruptly and still look finished; a status in the stream can be missed by a slow consumer.
- **Consequence:** the orchestrator publishes node status from the `Result`, never from an event. `agent_end` in particular is *not* terminal — Pi documents that retry, overflow recovery, compaction retry, steering, and follow-up work can follow it. Only `agent_settled` terminates an execution.

### D4 — Three classes of signal

| Class | Members | Contract |
|---|---|---|
| Terminal | `Result` | mandatory, exactly one |
| Interaction | `Ask`/`Answer`, `Injections`, ctx cancellation | gated by capability |
| Telemetry | text/thinking deltas, turns, tool lifecycle, file changes, usage, notices, status labels | best-effort, never required |

- **Why:** a harness that emits nothing but a result must still be usable, and the UI must degrade to "running" rather than the node failing. It also keeps adapters honest: an adapter cannot be required to invent a percentage or a patch it does not have.
- **Alternative rejected — flat 10-event list with mandatory `progress`/`diff`:** forces fabrication and couples the contract to one harness's abilities.
- **Consequence:** unrecognized record types become one notice per type and are otherwise ignored, so a newer Pi cannot break the adapter.

### D5 — Capabilities are declared, then enforced at preflight

Adapters declare `streaming`, `thinking`, `tool_events`, `file_events`, `questions`, `injection`, `abort`, `skills`, `model_select`, `tool_policy`, `plugins`, `usage`, `session_resume`. A node's configuration derives *needs* (`interactive: true` → questions; `allow_inject: true` → injection; `skill:` → skills; `model:` → model selection; `tools:` → tool policy). Preflight compares the two and refuses to start on a miss.

- **Why:** discovering an unmet requirement at node 5 of 8 wastes a run and the user's attention. `Probe` returns the effective capabilities rather than mutating the adapter — a component that failed to load (for example the bridge extension) is reported as `false` in the probe result, so preflight and the discovery endpoint both see one honest answer without shared mutable state.
- **Alternative rejected — fail at node start:** simpler, but turns a declarative graph into a sequence of surprises.
- **Cost:** the node schema must expose its needs, which the existing `workflows/standard-feature.yaml` already does.

### D6 — Questions require an embedded Pi extension

Pi's dialog sub-protocol only fires when an extension calls `ctx.ui.select/confirm/input`. The stock agent has no ask-user tool. Therefore the adapter ships `specflow-bridge.ts`, embedded with `go:embed`, materialized to a content-addressed path under the user cache directory (mode `0600`, atomic write), and passed as `-e <path>`. The extension registers one tool, `specflow_ask`, whose parameters are a plain JSON Schema object; it uses only a type-only import, so it adds no runtime dependency to the user's Pi installation.

- **Why:** this is the only supported path from "the model wants to know something" to "the web UI shows a question". Without it, `interactive: true` nodes are decorative.
- **Alternative rejected — let the model end its turn with a question and parse it:** heuristic parsing of prose, no reliable answer channel, no way to distinguish a question from a completed turn.
- **Alternative rejected — ship the extension as an npm package:** adds a second distribution artifact and a version-skew surface; embedding keeps one executable.
- **Cost:** Pi's extension API is an external contract. A Pi release can change it. Mitigation: `Probe` loads the bridge and reports `questions: false` when it cannot, and the failure names the incompatibility. The extension is loaded explicitly (`-e`), not auto-discovered, so it cannot collide with the user's own extensions.

### D7 — Skill binding is explicit

A node's skill is resolved to an absolute directory by the orchestrator (harness-agnostic), passed as `--skill <path>`, and invoked by prefixing the prompt with `/skill:<name>`. When a node binds a skill, `--no-skills` disables ambient discovery so the bound skill cannot lose a name collision to a user's global skill. Nodes without a skill keep ambient discovery.

- **Why:** `/skill:name` without registration silently degrades to plain text, and ambient discovery makes a run depend on the user's dotfiles. A workflow that produces different behaviour on two machines is not a workflow.
- **Alternative rejected — rely on discovery:** undetectable failure mode.
- **Cost:** the orchestrator must know where skills live (`.dsh/skills`, configurable) and pass a path the harness can reach on the same filesystem.

### D8 — No project trust by default

The adapter starts Pi with project-local trust disabled unless the node opts in. Trust is what allows project-local extensions and configuration to load, and those execute arbitrary code.

- **Why:** SpecFlow launches agents in the user's repositories; silently trusting whatever `.pi/` lives there converts opening a repo into code execution.
- **Cost:** some projects configured through `.pi/` behave differently until the node opts in. The failure message names the flag.

### D9 — Configuration is startup-only

Model, thinking level, tools, skills, and trust are argv at process start. No `set_model` or mid-run reconfiguration in this change.

- **Why:** with one process per node, startup configuration is complete and there is exactly one configuration state to reason about.
- **Cost:** if run-scoped sessions arrive later, they will need the RPC configuration commands; that is an additive change behind a new session concept.

### D10 — Test with a scripted Pi, smoke-test with the real one

`backend/internal/harness/pi/testdata/fake-pi` is a small Go program that speaks Pi's RPC: it reads JSONL commands from stdin and replays a script of records selected by an environment variable. Adapter tests exercise it through `exec`, covering happy path, ask round-trip, injection, abort escalation, oversized records, malformed records, and early exit. A separate test runs against a real `pi` and skips when the binary is absent.

- **Why:** `pi` is not installed here and CI is not guaranteed to have credentials. The adapter's contract and failure paths must be testable deterministically; the real binary only proves compatibility.
- **Alternative rejected — dependency injection with in-memory pipes only:** fast, but cannot test process lifecycle, signals, or shutdown escalation, which is where the bugs are. Pipes are still used for the pure translation tests.
- **Cost:** the fake must be kept honest against Pi's docs; a smoke test against real Pi is the counterweight.

## Pi integration

### Launch

```
pi --mode rpc --no-session
   [--provider <p>] --model <m> [--thinking <level>]
   [--tools +specflow_ask | --tools <explicit list>]
   [-ns --skill <abs skill dir>]
   [--append-system-prompt <text>]
   [-na | -a]
   [--offline]
```

Working directory is the node's. `--tools +specflow_ask` adjusts the default selection instead of replacing it; when the node supplies an explicit allowlist, `specflow_ask` must be named inside it, because plain names replace the whole selection. Readiness is established with a `get_state` command, because RPC mode has no explicit ready record.

### Record mapping

| Pi record / command | Contract |
|---|---|
| `turn_start` / `turn_end` | turn started / finished telemetry |
| `message_update` `text_delta` / `text_end` | incremental text / authoritative block |
| `message_update` `thinking_*` | incremental reasoning |
| `toolcall_*` | ignored — model intent, execution follows separately |
| `tool_execution_start` / `_update` / `_end` | tool started / progress / finished |
| `tool_execution_end` with `write` or `edit` | additionally a file-changed telemetry event |
| `message_end` (assistant) | supplies the final text |
| `message_update.usage` | cumulative usage |
| `agent_end` | **ignored as a terminator**; retries may follow |
| `agent_settled` | terminal outcome; `aborted: true` → `aborted` |
| `compaction_*`, retry events | notice |
| `extension_ui_request` `select`/`confirm`/`input`/`editor` | `Host.Ask` |
| `extension_ui_request` `notify`/`setStatus` | notice / status label |
| unknown type | one notice per type, never fatal |
| `steer` / `follow_up` command | injection translation, disposition reported |
| `abort` command | cancellation request |

Two hazards are handled explicitly: `agent_end` is not the end, and a `prompt` response with `disposition: "handled"` means no run started — waiting for it would hang forever, so it is reported as a protocol violation with a copy-pasteable cause.

### Lifecycle

```
start process -> reader (LF-only) + stderr ring
   -> get_state handshake (startup timeout)
   -> injection pump: Injections -> steer | follow_up
   -> prompt (record disposition)
   -> consume until agent_settled | ctx done | process exit
        asks answered on a separate goroutine so cancel stays responsive
   -> shutdown: close stdin -> grace -> SIGTERM -> grace -> kill, reap
   -> Result + typed error
```

Asks are dispatched off the reader goroutine. Pi blocks while a dialog is open, so handling an ask inline would prevent cancellation and injection from being processed — a deadlock class worth designing out rather than debugging later.

### Failure taxonomy

| Class | Trigger | Retryable |
|---|---|---|
| `adapter_missing` | executable not found | no |
| `unsupported` | preflight capability miss | no |
| `config` | bad flags, missing skill path, bridge incompatible | no |
| `auth` | provider credentials missing | no |
| `start` | process failed to start or never answered the handshake | yes |
| `protocol` | malformed or contradictory record | yes |
| `harness_exit` | process exited before settling | yes |
| `timeout` | node deadline exceeded | yes |
| `cancelled` | user abort | n/a |

### Package layout

```
backend/internal/harness/
  contract.go        Adapter, Host, Request, Result, Event, Ask, Answer,
                     Capabilities, Descriptor, Failure, Kind, Status
  registry.go        id -> factory; preflight validation
  mock/              scripted adapter for protocol tests
  pi/
    client.go        LF-only transport, id correlation, stderr ring
    translate.go     pure record -> contract translation
    adapter.go       argv, process lifecycle, Result assembly
    bridge.go        embed + materialize specflow-bridge.ts
    bridge/specflow-bridge.ts
    testdata/fake-pi/main.go
```

`GET /api/harnesses` returns a list of `{id, name, version, protocol, ready, readyError, capabilities}`; it calls `Probe` with a short timeout and degrades to `ready: false` with the reason rather than failing the request.

## Risks / Trade-offs

- **Pi's RPC protocol is versioned by Pi, not by SpecFlow.** → Minimum-version check at probe; unknown records ignored; translation isolated in pure functions with fixtures; the adapter's failure messages name the incompatible piece.
- **Pi's extension API drifts and the ask bridge stops loading.** → Probe loads the bridge and downgrades `questions`; preflight then blocks interactive nodes with an actionable message instead of hanging mid-run.
- **A hang is worse than a failure.** Every wait — handshake, command response, settle, ask — is bounded, and every bound produces a typed failure. → Timeouts are part of the task list, not an afterthought.
- **Backpressure deadlock:** `Emit` blocks, Pi stalls, the orchestrator waits on a node that will never finish. → `Emit` blocking is intentional; the contract states that the host must consume, and a node deadline bounds the damage. Coalescing deltas belongs in the SSE layer, not the adapter.
- **Event volume:** token-level deltas can reach thousands per second. → Fine for a channel, but the future SSE hub must coalesce; recorded as a constraint for that change.
- **Materializing an embedded extension writes a file.** → Content-addressed path, `0600`, atomic rename, cache fallback to a temp dir; the content is compiled into the binary, so its hash pins it.
- **One process per node costs startup time.** → Accepted for isolation and retry simplicity; measured once a real Pi is available.
- **No session continuity across nodes.** → The orchestrator feeds context forward; that is the product's stated design, not a workaround.
- **`pi` is absent on the development machine.** → Fake-Pi tests cover behavior; the real-binary smoke test skips. Compatibility with a real Pi remains unproven until the first manual smoke run, and must not be claimed as verified before then.

## Migration Plan

Purely additive: new packages, one new route, one embedded asset, documentation updates. No persisted data, no schema, no existing endpoint changes. Rollback is reverting the change commit; because nothing else imports the contract yet, a revert cannot strand callers.

## Open Questions

- **Exact minimum Pi version.** The constant starts at `1.0.0` and is tightened once a real Pi is available to run the smoke test against. It does not change the approach.
- **Whether `/skill:<name>` appears in `get_commands` output.** If it does, the adapter verifies skill registration at handshake; if not, it verifies the skill path and relies on expansion. Either way the fallback is the same, and the spec only requires that a missing skill path fails before prompting.
- **Whether run-scoped sessions are ever worth the extra complexity.** Deferred until a real workflow exposes a context problem that prompt-passing cannot solve.
