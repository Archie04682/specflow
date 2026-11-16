# Design

## Context

See `proposal.md` for motivation. The current implementation has three layers:

- `orchestrator.Hub` owns a run's sequence, in-memory event slice, append-only
  `events.jsonl`, and subscriber channels.
- `Engine` publishes at run/node transitions and exposes the hub through
  `Engine.Events()`.
- `api.runHandlers.events` checks the run status, then either replays a terminal
  run or subscribes to the hub.

`workflow-orchestration` already requires sequenced, replayable events. Runs can be
retried or resumed after `failed` or `cancelled`, so a terminal stream must be able
to become live again. The journal is the replay source after a restart. There is no
frontend consumer yet, so additive event fields are safe.

Constraints:

- Single process, no database, no WebSocket.
- Event order and replay are part of the durable spec.
- `events.jsonl` stays the source of truth; no per-node event storage.
- Harness telemetry is best-effort and must not fail a node.
- Go 1.27 `net/http`; the handler can use `http.NewResponseController`.

## Goals / Non-Goals

**Goals:**

- End every live stream when its run reaches a terminal status.
- Replay terminal history without preserving an open journal handle or the full
  event slice for every run the process has ever read.
- Make subscription atomic: no `Get`-then-`Subscribe` race.
- Bound per-subscriber and per-run memory; keep a slow subscriber from blocking
  execution.
- Make journal replay robust after an unclean shutdown and explicit about failures.
- Keep idle streams alive and bound stalled writes.
- Add the transition payload fields the UI will need.
- Cover the lifecycle with unit, HTTP, and end-to-end tests.

**Non-Goals:**

- No journal retention, rotation, or compaction. Deleting old events would weaken
  the replay guarantee and needs a gap/reset contract; that is a separate change.
- No coalescing of text deltas. It would reduce event volume but changes live
  granularity and needs its own buffering design.
- No WebSocket, database, authentication, or rate limiting.
- No harness adapter change and no rename of event types.
- No replacement of `events.jsonl` with `Run.NodeRuns[].Events`.

## Decisions

### 1. Terminal transition: publish, close, evict

`runFrom` always ends through `finishCompleted`, `finishCancelled`, or `failRun`.
Each publishes its terminal event and saves the run, then calls `Hub.Seal`:

1. `Seal` marks the log terminal, closes every subscriber channel, and clears the
   subscriber map. The journal and the replay cache stay readable.
2. The execution goroutine's deferred cleanup then, if the run is terminal, calls
   `Hub.Close` to close the file and remove the log, removes the run from
   `Engine.live`, and only then marks `running = false` and closes `done`.

Sealing immediately after the terminal event means a subscriber that connects at that
instant gets replay plus EOF, not a wait for an event that cannot come. Closing later
releases the file handle. Removing the live entry before `running = false` prevents a
concurrent resume from grabbing an entry that is about to disappear.

The journal file stays on disk. Replay after cleanup reopens it on demand.

**Alternatives rejected:**

- *Keep the log open forever.* This is the current resource leak: one file handle and
  the full event slice per run, including runs only inspected through `GET`.
- *Close the log immediately instead of sealing first.* A subscriber can race the
  close and either miss the terminal event or register on a dead log. `Seal` is the
  small state transition that makes the end of the stream observable atomically.
- *Delete the journal on terminal.* It would make page reload and `Last-Event-ID`
  replay impossible after completion.

### 2. Subscription is an engine operation

Add:

```go
type Stream struct {
    Replay  []Event
    Events  <-chan Event
    Cancel  func()
    Sealed  bool
    LastSeq uint64
}

func (e *Engine) Subscribe(ctx context.Context, projectRoot, runID string, after uint64) (*Stream, error)
func (e *Engine) Replay(ctx context.Context, projectRoot, runID string, after uint64) ([]Event, error)
```

`Subscribe` loads the run (live entry or `run.json`), ensures the journal is open,
snapshots the replay, and either registers a live subscriber or returns a sealed
stream. If it had to create the log on demand and the run is already terminal, there
is no execution left to publish, so it seals the new log before reading replay.
`Events()` is removed from the public engine surface; the hub stays internal.

The hub returns replay separately from the live channel:

```go
func (h *Hub) Subscribe(runID string, after uint64) (replay []Event, events <-chan Event, sealed bool, cancel func(), err error)
```

`sealed` is true when the log has been sealed or closed. A sealed subscription still
returns replay but never registers a live channel.

The subscriber channel has a fixed buffer (256). Replay is written by the HTTP
handler before it starts draining the channel. If live events arrive while replay is
being written, they wait in the buffer; if more than 256 arrive, the subscriber is
dropped and reconnects. No subscriber allocates a channel as large as the run.

**Alternatives rejected:**

- *Keep preloading replay into the channel.* A late subscriber allocates
  `len(replay)+256` entries, duplicating the retained history.
- *Keep the API's `Get` plus status check.* A run can terminate between the check and
  `Subscribe`, which either misses the terminal event or leaves the handler waiting
  for a stream that will never close.

### 3. No journal work on plain reads

`Engine.load` currently opens the hub and reads the whole journal for `GET /api/runs/{id}`.
That is removed. Only `launch` (execution) and `Subscribe`/`Replay` open the journal.

`Get`, `Subscribe`, `Replay`, `Discard`, `Cancel`, `Approve`, `Answer`, and `Inject`
use a non-caching inspection load. Otherwise a terminal run would be re-added to
`Engine.live` every time it is inspected or rejected, undoing eviction. Only execution
and mutations that can restart a run (`Start`, `Retry`, `Resume`) use the caching
load.

A run loaded from `run.json` is a normal record. A terminal run's journal is opened
for replay, sealed when it had to be recreated after cleanup, read, and closed before
the stream returns. A live run keeps its journal open until terminal cleanup.

### 4. Bounded live cache, journal-backed replay

`eventLog` keeps a bounded cache of the most recent events (default 4096) plus
`cacheStart`, the sequence of the first cached event. `Open` scans the journal to
establish `seq`, repairs the tail, and retains only the last cache window in memory.
`Emit` appends to the journal and the cache; when the cache exceeds its limit, the
oldest entry is dropped and `cacheStart` advances.

`Subscribe`:

1. Locks the log and snapshots `seq`, `cacheStart`, and the cache.
2. Registers the subscriber with a fixed buffer.
3. Unlocks.
4. If the cursor is older than `cacheStart`, reads the journal prefix up to
   `cacheStart-1` and prepends it to the copied cache.
5. Returns the combined replay plus the live channel.

Live events published after the snapshot go to the channel, so there is no gap
between replay and live. Reading the prefix while the journal is being appended is
safe because the prefix ends before any cached event and before the live tail. If
the read fails, the subscription is cancelled and the API returns an error.

`Replay` uses the same cache/prefix logic for clients that do not stream.

`Seal` is idempotent: it sets the terminal flag, closes every subscriber channel, and
clears the subscriber map, but keeps the cache and the file. A later `Emit` is ignored
and logged because a sealed run must not receive more events. `Close` is also
idempotent: it seals first if needed, closes the file, and removes the log. `Open`
after a close reads the journal again, so replay does not depend on the old log
object.

**Alternatives rejected:**

- *Keep every event in memory for every run.* Unbounded memory and a large replay
  allocation; this is the current behavior.
- *Bound the cache and return an error for old cursors.* It would break the existing
  replay guarantee. The journal can serve the prefix.

### 5. Terminal tail policy: 204 at the tail, 200 behind it

After `Subscribe`, the API decides:

| Stream state | Cursor | Response |
|---|---|---|
| sealed | at or beyond `LastSeq` | `204 No Content`, no body |
| sealed | behind `LastSeq` | `200`, replay, EOF |
| sealed | no cursor (`after = 0`) and `LastSeq > 0` | `200`, full replay, EOF |
| open | any | `200`, replay, then live events |

A subscription that races the seal is treated as open: it receives the terminal event
from the channel and then EOF. The next reconnect sees the sealed state.

`204` is the standard EventSource stop signal: the browser does not reestablish the
connection, so a finished run does not produce a reconnect loop. A fresh page load
still gets the full history.

An unconditional `410 Gone` is rejected because it makes a page reload lose history.
`410` would only be useful when the cursor is at the tail, where `204` is already the
conventional response.

**Alternative considered:** always `200` plus replay and close. It works for a
client that closes on the terminal event, but a page that keeps the EventSource open
reconnects forever. The tail check avoids that.

### 6. Fixed subscriber buffer and overflow

Each subscriber channel is fixed at 256 events. `Emit` uses a non-blocking send; if
the channel is full, the hub closes that channel, removes the subscriber, and logs a
warning with the run and node. The stream ends; the client reconnects with
`Last-Event-ID` and catches up from the journal.

The alternative — blocking `Emit` until the subscriber reads — would let one slow
client stall node execution, which violates the streaming contract.

### 7. Journal repair and error reporting

`Open` uses a line reader with explicit offsets, not a scanner that hides errors:

- A trailing record that is not newline-terminated and is not valid JSON is
  discarded; the file is truncated to the last complete record.
- A trailing record that is valid JSON is kept and the file is terminated with a
  newline.
- A malformed record in the middle of the journal fails `Open` with its line number;
  it is not silently skipped.
- A record larger than `maxEventBytes` (4 MiB) fails `Open` with a clear error.
  Silently skipping it would make replay shorter than the live stream.
- `Emit` builds the event with the next sequence number, marshals it, and commits the
  sequence only after the size check and append succeed. An oversized or unencodable
  event returns an error without consuming a sequence number, writing, or
  publishing.
- If the append fails, `Emit` truncates back to the previous size, returns the
  error, and does not publish. The engine logs it; telemetry is best-effort, so the
  node and run continue.
- After `Seal`, `Emit` is ignored and logged; a sealed run never receives more
  events.
- A journal that exceeds 256 MiB logs a one-time warning per run. It is not
  truncated; cleanup stays with run discard.

**Alternatives rejected:**

- *Keep ignoring scanner errors.* A single oversized event silently truncates replay
  and can make new events reuse sequence numbers.
- *Panic or fail the run on journal errors.* Telemetry is best-effort; losing one
  event must not fail the run, but it must be visible in the log.

### 8. Heartbeat and write deadline

For a live stream the handler starts a ticker (15 s default, injectable in tests):

- Every tick writes the SSE comment `: keep-alive\n\n` and flushes.
- Before every event, heartbeat, or flush, it sets a write deadline (30 s default)
  through `http.NewResponseController`. If the deadline cannot be set, it is
  ignored.
- A failed write returns from the handler and cancels the subscription.
- Terminal replay does not start a ticker; it is finite.

This keeps long tool-call gaps observable through proxies and prevents a half-open
connection from pinning a handler indefinitely.

### 9. Event contract

The envelope stays `{seq, at, type, runId, nodeId, data}` and type names stay
dot-separated. Additive fields:

| Event | Added fields |
|---|---|
| `run.started` | `workflowId`, `objective` |
| `node.started` | `attempt`, `kind`, `title` (re-emitted per attempt) |
| `node.failed` / `node.postcondition_failed` | `error`, `class`, `retryable` |
| `node.completed` | `postcondition` (`"met"` or `"none"`), `change`, `checkpoint` |
| `run.completed` | `summary` (human-readable string) |

`node.started` moves from `beginNode` into the attempt loop so each attempt carries
its number. A retry therefore emits another `node.started` with `attempt: 2`.
`retryable` is computed by the existing failure classifier rather than inferred by
the client.

**Alternatives rejected:**

- *Rename to `run_started` and flatten payloads.* No external contract requires it,
  tests and the existing stream use dots, and renaming would be a breaking change for
  no benefit.
- *Leave the fields out.* The UI would have to infer attempt and retryability from
  separate events, and `run.completed` would carry no summary.

### 10. Tests

Hub unit tests (`events_test.go`):

- fan-out to two subscribers;
- replay + live ordering with no gap or duplicate;
- slow subscriber dropped while another subscriber and the engine continue;
- seal and close end streams and keep the journal readable;
- replay after close with a bounded cache and a cursor older than the cache;
- concurrent emit/subscribe/cancel under `-race`;
- partial-tail repair, oversized record, and append failure.

Engine tests:

- terminal cleanup closes subscribers and evicts the live entry;
- replay after eviction still returns history;
- resume/retry reopens the journal and continues the sequence.

HTTP tests (`runs_test.go`, plus a new `sse_test.go` if useful):

- live stream from a mock harness that emits with `EmitDelay`;
- two subscribers receive the same events;
- history + live after `Last-Event-ID`;
- disconnect removes the subscriber;
- terminal tail returns `204`; behind-tail and fresh return `200` plus history;
- unknown run returns `404`;
- heartbeat is sent while idle.

Integration test: mock harness emits ordered events through engine, hub, and SSE;
the client receives them in order and the stream closes at the terminal event.

## Risks / Trade-offs

- **Seal/close/evict race with subscribe.** A subscription can race terminal
  cleanup. Mitigation: `Seal` closes subscriber channels only after fanning out the
  terminal event; a subscription that races the seal is treated as open and receives
  the terminal event then EOF; after a close, `Subscribe` reopens the journal for
  replay. `-race` and concurrent tests cover it.
- **Replay prefix read while live.** The prefix is read outside the log lock and the
  cache can evict concurrently. Mitigation: snapshot the cache and `cacheStart` under
  the lock before reading; the prefix ends at the snapshot boundary, and live events
  go to the buffered channel.
- **`204` surprises clients.** Only a terminal run at the tail gets `204`; a fresh
  page load gets `200` and history. Document it in the README.
- **Eviction changes the engine surface.** `Engine.Events()` disappears; tests and
  the API move to `Subscribe`/`Replay`. This is intentional: the hub is internal.
- **Journal growth remains.** A very long or noisy run grows `events.jsonl` on disk.
  Mitigation: bounded memory, one-time size warning, and cleanup when the run is
  discarded. Retention/compaction is a separate change.
- **Replay and journal open are O(N).** The cache bounds memory, not time: a full
  replay and the initial journal scan still read every record, and a terminal-tail
  `204` opens the journal to learn `LastSeq`. This is accepted for a local
  single-process tool; an offset index is a separate optimization.
- **Large events are dropped.** A tool result over 4 MiB is not journaled or
  published and is logged. This is explicit and bounded; a future change can
  truncate payloads with a marker instead.
- **`node.started` repeats per attempt.** Consumers must treat it as an attempt
  start, not a one-time node transition. No frontend consumer exists yet.

## Migration Plan

- No data migration: existing `events.jsonl` files are read with the new repair
  logic and keep their sequence.
- No dependency change and no change to `run.json` or the workflow schema.
- Deploy the single binary; rollback is safe because the new event fields are
  additive and the old code ignores them.
- Update `README.md` and `AGENTS.md` with the terminal-stream contract, the `204`
  tail behavior, and the journal bounds.
- Verify with `make test` (including `-race`) and `openspec validate --all --strict`.

## Open Questions

None that change the specs or the approach. The cache size, heartbeat interval, and
journal warning threshold are constants with test overrides; they can be tuned
without a spec change.
