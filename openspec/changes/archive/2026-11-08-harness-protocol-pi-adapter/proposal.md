# Proposal

## Why

`AGENTS.md` binds the product to one architectural decision above all others: the orchestrator talks to a harness adapter interface, never to a specific runtime. No such interface exists. Until one does, every adapter-shaped question — what "finished" means, how a node is cancelled, how the user answers a question mid-run, what happens when a harness cannot stream — gets answered ad hoc inside the first adapter and then leaks upward into the orchestrator.

The design conversation got the shape right and the details wrong, because it was written before anyone read Pi's actual interface. Its event list includes `progress` (no harness can honestly report it) and `diff` (Pi reports tool calls, not patches); it treats `done` and `error` as events when they are return values; it puts `parallel` on the adapter when parallelism belongs to the scheduler. Most consequentially, it promises interactive Q&A as though Pi asks questions natively — it does not. Pi's dialog subprotocol only fires when an extension calls `ctx.ui.select/confirm/input`, so without a companion extension the flagship feature is dead code.

This change defines the contract and proves it against the first real adapter. It is the riskiest backend assumption in the product — that one contract absorbs a foreign agent protocol without leaking — and it is far cheaper to test now than after the DAG engine depends on it.

## What Changes

- Define the adapter contract in `backend/internal/harness`: `Adapter`, `Host`, `Request`, `Result`, `Event`, `Ask`/`Answer`, `Capabilities`, `Descriptor`, and a typed `Failure` taxonomy. Every message is a stable JSON shape so a subprocess transport can be added later without changing the types.
- Replace the flat event list with a **three-class model**: terminal (`Result` is the return value; no `done`/`error` events), interaction (`Ask`, injection, cancellation — capability-gated), and telemetry (best-effort; an adapter that emits none still works).
- Introduce capability declaration plus **preflight validation**: a node's declared needs are checked against the adapter's capabilities before a run starts, not when the node is reached.
- Add an adapter registry (`id -> factory`) and `GET /api/harnesses` exposing descriptors, capabilities, and probe status.
- Implement the **Pi adapter** against Pi's documented RPC mode: process lifecycle, LF-only JSONL framing, `id`-correlated commands, record translation, injection through `steer`/`follow_up`, cancellation through `abort` then stdin EOF, SIGTERM, SIGKILL, and stderr captured but never parsed as protocol.
- Ship an embedded Pi extension (`specflow-bridge.ts`, `go:embed`-ed and materialized into a cache path at runtime) that registers a `specflow_ask` tool. This is the only mechanism that makes `interactive: true` nodes real, and embedding keeps the single-binary promise.
- Bind a node's skill deterministically: an absolute `--skill` path plus a `/skill:<name>` prompt prefix, rather than relying on ambient discovery.
- Add a scripted fake `pi` binary as a test double so the adapter is fully tested on a machine with no Pi installed.
- Update `AGENTS.md` (current phase, adapter contract summary) and `README.md` (prerequisite and how to verify the adapter).

**Not included:** orchestrator, DAG execution, session manager, SSE, persistence, and frontend. They consume the contract; they do not shape it. The `Ask` path is exercised against a test host in this change and gains its real host in the session-manager change.

## Capabilities

### New Capabilities

- `harness-protocol`: the harness-agnostic adapter contract — three-class event model, capability declaration and preflight, interactive question routing, cancellation and injection semantics, typed failure taxonomy, and harness discovery over the HTTP API.
- `harness-pi-adapter`: SpecFlow's first adapter — launching Pi in RPC mode, deterministic skill and model binding, translation of Pi records into contract events, the embedded ask bridge extension, and orderly shutdown.

### Modified Capabilities

None. `single-binary-distribution` is unaffected: the binary still contains and serves everything; it only gains an embedded TypeScript asset that is materialized at runtime.

## Impact

- **New code:** `backend/internal/harness` (contract, registry, mock), `backend/internal/harness/pi` (client, translation, adapter, bridge), test-only fake `pi` binary. Existing packages are touched only to register the route and to update docs.
- **New API surface:** `GET /api/harnesses`. No existing endpoint changes.
- **Dependencies:** Go standard library only. No new module requirements.
- **Runtime dependency:** the `pi` executable, and only for nodes that select the `pi` harness. Build, tests, and the rest of the product work without it.
- **Distribution:** one extra embedded file inside the binary. Pi itself is not vendored; it remains a separately installed harness.
- **Risk carried forward:** the Pi RPC surface is versioned by Pi, not by us. The adapter pins a minimum version, probes it at startup, and fails with an actionable message rather than degrading silently.
