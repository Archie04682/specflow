# Tasks

## 1. Hub storage, replay, and lifecycle

- [x] 1.1 Rework `eventLog` to keep a bounded replay cache (`cacheStart`, configurable
      limit, default 4096), the current sequence, the journal path, the append file,
      and `sealed`/`closed` flags; verify a table-driven `events_test.go` test opens a
      journal with more events than the cache and asserts `seq`, `cacheStart`, and the
      cached window.
- [x] 1.2 Replace the scanner-based `Hub.Open` with a line reader that tracks offsets,
      repairs a partial trailing record, rejects a malformed middle record with its
      line number, and rejects a record over 4 MiB; verify tests for each case,
      including a valid last record without a trailing newline.
- [x] 1.3 Make `Hub.Emit` build the event with the next sequence number, serialize it,
      and commit the sequence only after the size check and append succeed; reject an
      oversized event without consuming a sequence, append to the journal with a
      rollback to the previous size on failure, skip publication on failure, and log a
      one-time warning when the journal crosses an injectable size threshold; verify
      tests for oversized, unserializable, write-failure, and threshold-warning cases.
- [x] 1.4 Change `Hub.Subscribe` to return replay separately from a fixed-buffer
      (256) live channel, snapshot the cache and `cacheStart` under the lock, register
      before reading an older journal prefix, and report a sealed log; verify replay +
      live ordering tests with no gap or duplicate.
- [x] 1.5 Implement overflow shedding: a full subscriber channel is closed and removed
      without blocking `Emit`, a warning is logged, and other subscribers continue;
      verify a test with one stalled subscriber and one active subscriber.
- [x] 1.6 Implement `Hub.Seal`: mark the log terminal, close every subscriber channel
      after the terminal event has been fanned out, clear the subscriber map, keep the
      journal and cache readable, and ignore later emits with a log entry; verify a
      test that a sealed run replays its terminal event and never registers a live
      subscriber.
- [x] 1.7 Implement `Hub.Close`: seal if needed, close the journal file, remove the log
      from the hub, and leave the journal readable through a fresh `Open`; verify a
      test that closes a live stream, reopens the journal, and replays every event.
- [x] 1.8 Add concurrency tests that emit, subscribe, cancel, seal, and close the same
      run from multiple goroutines; verify with `go test
      ./backend/internal/orchestrator -race` and assert no panic, duplicate, or missing
      replay.

## 2. Engine lifecycle and event contract

- [x] 2.1 Remove journal opening from `Engine.load` and `Engine.Create`; open the
      journal only in `launch`, `Engine.Subscribe`, and `Engine.Replay`; add a
      non-caching inspection load for `Get`, `Subscribe`, `Replay`, `Discard`, `Cancel`,
      `Approve`, `Answer`, and `Inject` so a terminal run is not re-added to
      `Engine.live`; verify a test that repeated `Get` and replay on a terminal run does
      not grow `Engine.live` and that `Get` still returns the run record without leaving
      a journal handle open.
- [x] 2.2 Implement terminal cleanup: after a terminal event, call `Hub.Seal`; in
      `launch`'s deferred block call `Hub.Close`, evict the run from `Engine.live`, then
      mark it not running and close `done`; verify a test that a live subscriber ends on
      terminal, a later replay still returns history, and a concurrent resume does not
      resurrect an evicted entry.
- [x] 2.3 Add `Engine.Subscribe` and `Engine.Replay` with the `Stream` result, seal a
      log created on demand for an already-terminal run before reading replay in either
      path, remove the public `Engine.Events()` surface, and update callers; verify the
      engine and API tests compile and pass with the hub internal to the package.
- [x] 2.4 Add the transition payload fields: `workflowId` and `objective` on
      `run.started`, `attempt` plus `kind`/`title` on a per-attempt `node.started`,
      `retryable` on `node.failed` and `node.postcondition_failed`,
      `postcondition`/`change`/`checkpoint` on `node.completed`, and `summary` on
      `run.completed`; verify engine tests assert each field and that a retry emits a
      second `node.started` with `attempt: 2`.
- [x] 2.5 Add engine tests for terminal cleanup, replay after eviction, and resume
      reopening the journal with the next sequence; verify they pass under `-race`.
- [x] 2.6 Update `README.md` and `AGENTS.md` with the event envelope, the added
      transition fields, the per-attempt `node.started` semantics, and the terminal
      stream lifecycle; verify the documented event names match the emitted events.

## 3. SSE transport

- [x] 3.1 Move the SSE handler into `backend/internal/api/sse.go`, switch it to
      `Engine.Subscribe`, remove the `Get`-then-status branch, and keep the existing
      `Last-Event-ID`/`after` parsing; verify the handler tests still compile and the
      route is registered once.
- [x] 3.2 Implement the terminal response policy: `204 No Content` when the cursor is
      at or beyond the terminal tail, `200` plus replay then EOF when behind it, and
      full replay then EOF for a fresh client; verify a table-driven handler test for
      each case.
- [x] 3.3 Add the live-stream heartbeat (15 s default, injectable) and per-write
      deadline (30 s default) with `http.NewResponseController`; verify a test receives
      a `: keep-alive` comment on an idle live stream and that a terminal replay does
      not start a ticker.
- [x] 3.4 Ensure disconnect and write failure cancel the subscription and release the
      handler; verify a handler test that cancels a live request and a hub test that the
      cancel removes the subscriber and stops further delivery.
- [x] 3.5 Add handler tests for unknown run (`404`), live replay + live delivery,
      terminal tail (`204`), terminal behind-tail (`200` with only later events), and
      fresh terminal replay; verify all pass with `httptest`.
- [x] 3.6 Document the SSE terminal contract (`204` at tail, `200` otherwise),
      heartbeat comments, and the `Last-Event-ID` reconnect behaviour in `README.md`;
      verify the endpoint section matches the handler.

## 4. End-to-end streaming

- [x] 4.1 Add a live integration test using the mock harness with `EmitDelay`, connect
      before completion, and assert the client receives `run.started`, `node.started`,
      `node.event`, `node.completed`, and `run.completed` in order; verify the stream
      closes after the terminal event.
- [x] 4.2 Add a two-subscriber integration test and a history-then-live test using
      `Last-Event-ID`; verify both clients receive the same later events with no
      duplicates.
- [x] 4.3 Add an end-to-end test from mock harness through engine, hub, and SSE to a
      real `httptest` client, including a reconnect at the terminal tail; verify the
      reconnect receives `204` and a fresh connection receives the full history.
- [x] 4.4 Add a repeated connect/disconnect test and a goroutine baseline check; verify
      no handler goroutines or subscriber entries remain after the run terminates
      (use `runtime.NumGoroutine` or `pprof` and allow for scheduler noise).

## 5. Final verification

- [x] 5.1 Run `make test`, `go test ./... -race`, and `openspec validate --all
      --strict`; verify all pass.
- [x] 5.2 Run the streaming tests repeatedly (`go test ./backend/internal/api
      ./backend/internal/orchestrator -run 'Event|Stream' -count=20`) and verify no
      flakes, races, or leaked subscribers.

## Workflow follow-up

- Archive the change after the project review requirements and the strict OpenSpec
  validation pass.
- Verify the archived main spec contains the terminal stream, slow-subscriber,
  heartbeat, event payload, and journal repair requirements.
