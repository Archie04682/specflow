# Proposal

## Why

The live event stream already sequences and replays run events, but its lifecycle is
incomplete. A subscriber connected before a run ends is never released after the
terminal event; a finished run's stream closes and reconnects forever; and the process
retains every event and an open journal handle for every run it has ever inspected.
Long tool-call gaps have no heartbeat, a stalled client has no write deadline, and an
unclean shutdown or oversized event can silently truncate replay. These gaps become
client-visible as soon as the UI consumes the stream, so they must be closed now.

## What Changes

- **Terminal stream lifecycle.** On `completed`, `failed`, or `cancelled`, publish the
  terminal event, close every subscriber channel, and keep the journal for replay. A
  subscriber that connects after termination receives the retained events and EOF.
  Resume or retry opens a new live epoch.
- **Atomic subscription.** Replace the API's `Get`-then-`Subscribe` check with one
  engine-level subscribe that snapshots replay, registers the subscriber, and reports
  whether the run is sealed, all under one lock. Removes the terminal race.
- **Tail termination without losing history.** A client whose `Last-Event-ID` is
  already at the terminal tail gets `204 No Content` and no body; a client behind the
  tail gets `200` plus replay then EOF; a fresh client gets full replay then EOF. No
  unconditional `410`, which would make a page reload lose history.
- **Slow-subscriber contract.** A subscriber that cannot keep up is disconnected
  rather than blocking execution, and reconnects from its last sequence number.
  `Subscribe` returns replay as a slice and a fixed-buffer live channel, so a late
  client no longer allocates a channel as large as the entire run.
- **Bounded process resources.** Do not open the journal on a plain run read. Evict
  terminal runs from the live map, close their journal, and reopen it on demand for
  replay. Keep live replay in a bounded cache and read older events from the journal.
- **Journal hardening.** Repair a partial trailing record on open, surface journal
  read and append errors instead of ignoring them, and bound a single event's size
  with an explicit failure.
- **Long-lived stream health.** Send an SSE heartbeat comment on an interval when no
  event is available, and set a write deadline so a half-open connection cannot pin a
  handler.
- **Stable event contract.** Keep the dot-separated event names and envelope; add the
  transition fields clients need: workflow id and objective on `run.started`, attempt
  on `node.started`, retryable on `node.failed`, postcondition outcome on
  `node.completed`, and summary on `run.completed`.
- **Tests.** Add Hub unit tests for fan-out, replay ordering, overflow, seal, resume,
  unsubscribe, and concurrency; live SSE tests for two subscribers, history-then-live,
  disconnect cleanup, and terminal-tail behavior; and an end-to-end mock-harness
  stream test.

Explicitly out of scope: no WebSocket or database, no authentication or rate limiting,
no harness adapter change, no renaming of event types, and no replacement of
`events.jsonl` with per-node event storage.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `workflow-orchestration`: extend the existing run event requirement with terminal
  stream end, slow-subscriber shedding, tail termination, heartbeat behaviour, and the
  transition payload fields; replay remains sequenced and durable.

## Impact

- `backend/internal/orchestrator/events.go` — hub lifecycle, sealing, replay cache,
  journal repair, and subscription API.
- `backend/internal/orchestrator/engine.go` — terminal transitions, live-map eviction,
  and transition payloads.
- `backend/internal/api/runs.go` (and a new `backend/internal/api/sse.go` if the handler
  no longer fits) — atomic subscribe, terminal-tail status, heartbeat, and write
  deadline.
- `backend/internal/orchestrator/events_test.go` and `engine_test.go`, plus
  `backend/internal/api/runs_test.go` — new unit and integration coverage.
- `openspec/specs/workflow-orchestration/spec.md` — delta for stream lifecycle and
  event contract.
- `README.md` and `AGENTS.md` — describe the terminal stream contract and the journal's
  resource bounds.

No new dependency. `events.jsonl` stays compatible: the change adds repairs and bounds
but does not change the on-disk shape.
