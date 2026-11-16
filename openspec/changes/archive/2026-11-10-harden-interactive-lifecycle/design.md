# Design

## Context

See [proposal.md](proposal.md) for motivation and the delta spec for the behavior contract.

The current units of ownership are already correct and stay:

- `harness.Host` is the adapter-facing contract: `Emit`, `Ask`, `Injections`. It does not change.
- `session.Session` is the host for one node execution. It owns the pending question map and the injection channel.
- `orchestrator.Engine` owns run state, the approval channel, node attempts, and event publication.
- `api.runHandlers` validates HTTP input and calls engine methods; it never touches a channel directly.

Constraints that shape the design:

- One node runs at a time, in one engine goroutine. Approval, questions, and injection all belong to that single execution.
- Runs are compiled before creation and pinned. Interaction timeouts must therefore be compiled into the plan, not looked up at wait time.
- Events are the replayable record. Every new state transition must be emitted after the state is saved, so a reader never sees an event for a state it cannot read.
- The existing test helpers drive real engine goroutines with a scripted mock. Timeout tests must use millisecond deadlines, not sleeps near 30 minutes.
- The frontend has no event consumer yet; the cost of the event rename is README, tests, and the API contract.

## Goals / Non-Goals

**Goals:**

- Make approval, question, and injection waits deterministic under timeout, cancellation, retry, and concurrent requests.
- Make every wait observable both as an event and in the run record.
- Keep the adapter contract and the engine's single-goroutine execution model untouched.

**Non-Goals:**

- Reconciling waiting runs orphaned by a server restart (separate follow-up; see Risks).
- Reworking the run scheduler, adding concurrency between nodes, or changing retry policy.
- Persisting a full injection transcript; only enough to show pending and disposition.
- Adding new question kinds or approval for injections.

## Decisions

### 1. Approval belongs to the attempt, after preparation

`executeNode` currently gates once, before `runAttempts`. Move the gate into the attempt loop, after workdir resolution and agent preflight, and move `beginNode` behind the gate on the first attempt so a rejected node is never reported as running. `node.started` is emitted only for an approved attempt.

Why:

- Preparation can fail (bad workdir, unmet capability). Asking the user to approve work that cannot start is noise.
- Per-attempt approval is the contract in the delta spec; the attempt number must identify the decision.
- User-initiated retry already re-enters `executeNode`, so only automatic retries change behavior. This is intended: an approval-gated node is attended.

Request record:

```
approvalRequest {
  id       string   // unique per request
  nodeID   string
  attempt  int
  action   string
  deadline time.Time
  resolved bool     // guarded by liveRun.mu
  decision chan bool
}
```

`liveRun` keeps `approval *approvalRequest` instead of a bare channel plus node string. `Approve` matches the request id (or node plus attempt for compatibility) and refuses a mismatched or already-resolved request. Clearing the pointer under the mutex, rather than closing a channel, avoids send-on-closed panics and prevents a late decision from reaching the next attempt. A rejected first attempt marks the node skipped; a rejected retry preserves the node's failed status. A small `lastApproval` record (id, node, attempt, outcome) is kept until the next request so a duplicate decision maps to 410 rather than an ambiguous 409.

Alternatives considered:

- *Keep once-per-node approval*: smaller diff, but violates the per-attempt requirement and lets a retry run on an old decision.
- *Attempt number only, no request id*: a stale decision for attempt 2 can approve attempt 3. The id makes the resolution exact.
- *Close the channel on timeout*: a concurrent `Approve` send panics. A resolved flag under the mutex is safer.

### 2. Questions get a single owner with an explicit resolution

Replace `pending map[string]chan harness.Answer` with `pending map[string]*pendingQuestion`:

```
pendingQuestion {
  ask      harness.Ask
  channel  chan harness.Answer
  resolved bool   // guarded by Session.mu
}
```

`Answer` locks, checks `resolved`, marks it resolved, sends, and unlocks. The timeout/cancel path locks, checks `resolved`, marks it resolved, and returns `Cancelled`. Whichever path wins, the other becomes a no-op and cannot report a delivery the execution did not receive. This removes the current race where a timeout and an API answer can both appear to win. Resolved entries stay in the map for the session's lifetime so a later answer is reported as already resolved rather than unknown; the map is cleared when the session closes.

One question at a time: a one-slot semaphore (`askSlot chan struct{}`, capacity 1) is acquired before registering the question and released after resolution. A second `Ask` waits in the adapter's own goroutine, which is already where the adapter blocks; the engine goroutine is unaffected. The effective deadline starts when the question becomes active, not while queued. Duplicate IDs are rejected with an error rather than overwriting the earlier channel.

Alternatives considered:

- *Reject concurrent asks*: simpler, but drops a question the harness may need and hands the adapter a cancellation it did not ask for.
- *A queue owned by the session*: more state for the same behavior; the adapter goroutine already provides the queue.

### 3. Timeouts are compiled configuration, not engine defaults

Add to `Defaults`:

```yaml
defaults:
  ask_timeout: 30m
  approval_timeout: 30m
```

Add `ask_timeout` and `approval_timeout` to `Node`. Compile them into `PlanNode` as `Duration` fields, where zero means no configured deadline. Precedence:

- Approval: node value, else workflow value, else 30 minutes.
- Question: node value, else workflow value, else the harness's `Ask.Timeout`, else 30 minutes. A configured value wins over the harness hint because the orchestrator owns the wait.

The configured values travel in the plan, so a run created before an edit keeps its deadlines. The 30-minute fallback is applied at the wait site, because a question must consider the harness hint first. The session receives its configured ask deadline at construction (`session.New(nodeID, sink, askTimeout)`); the engine reads the approval deadline from the plan.

Timeout outcomes:

- Approval expiry clears the request, marks the node failed with class `timeout`, publishes `approval.timed_out`, and returns a typed error so `runFrom` fails the run. It must not go through `errApprovalRejected` (which means cancelled).
- Question expiry returns `Cancelled` to the harness and publishes `question.timed_out`; the node's outcome is the harness's response, matching `harness-protocol`.

Alternatives considered:

- *Engine flags or environment variables*: not pinned to the run and invisible to the workflow author.
- *No default*: a missing configuration would wait forever, which is the bug being fixed.
- *Host-side default answer*: the contract says the harness receives a cancellation; inventing an answer would hide the timeout.

### 4. Injection uses a bounded, non-blocking queue

Keep a channel but make the send non-blocking:

```
select {
case s.injections <- injection:
    return id, nil
default:
    return "", ErrInjectionQueueFull
}
```

Raise the capacity from 16 to 256. The queue is a delivery buffer, not a transcript; 256 is generous for interactive use and still bounds memory. On overflow the engine publishes `injection.rejected` with reason `queue_full` and returns a typed error mapped to 409. The API reports `queued`, never `delivered`.

Disposition: the adapter already emits `KindInjectionAck` through `Host.Emit`. `sessionSink.Emit` recognizes that payload, publishes `injection.acknowledged`, and updates the run's injection record. This keeps the adapter contract unchanged.

Run record: add a bounded `Injections []InjectionRecord` (`id`, `nodeId`, `text`, `at`, `disposition`) capped at 100 entries, oldest dropped. The pending interaction snapshot is separate.

Alternatives considered:

- *Unbounded queue with a pump goroutine*: true accumulation, but adds a goroutine per session, a close protocol, and goleak surface. The bounded queue is simpler and fails honestly.
- *Blocking send with a larger buffer*: still violates the non-blocking requirement under sustained load.
- *Drop oldest silently*: loses a user message without telling anyone.

### 5. One pending interaction snapshot on the run

Add to `Run`:

```go
Pending *PendingInteraction `json:"pending,omitempty"`

type PendingInteraction struct {
    Kind        string     // "question" | "approval"
    ID          string
    NodeID      string
    Attempt     int
    Title       string
    Message     string
    Options     []string
    Action      string
    RequestedAt time.Time
    Deadline    *time.Time
}
```

A run can only wait for one blocking interaction at a time: approval precedes execution, questions happen during execution, and only one question is active. A single field is therefore sufficient and avoids two mutually exclusive pointers.

Write it under `liveRun.mu` and call `save` before publishing the event. Clear it on answer, decision, timeout, cancellation, failure, and completion. This is what lets a reconnecting client answer without replaying the journal.

### 6. Events are renamed and completed now

Use the existing dot convention:

| Transition | Event |
| :--- | :--- |
| question requested | `question.asked` (kept) |
| answered | `question.answered` |
| deadline expired | `question.timed_out` |
| cancelled by run cancellation | `question.cancelled` |
| injection queued | `injection.received` (replaces `injection.sent`) |
| adapter acknowledged | `injection.acknowledged` |
| injection refused | `injection.rejected` |
| approval requested | `approval.required` (kept) |
| approved | `approval.granted` |
| rejected | `approval.rejected` (kept) |
| deadline expired | `approval.timed_out` |

`question.resolved` is removed. The payload always carries the interaction id and, for approvals, the attempt and deadline. State is saved before the event is emitted.

Alternatives considered:

- *Keep `question.resolved` and add a status field*: fewer events but clients must branch on payload to know what happened; the event type is the branch.
- *Emit both old and new names*: doubles the stream and leaves two contracts to maintain.
- *Snake-case names from the brief*: inconsistent with every other event and the README.

### 7. HTTP contract

- `POST /api/runs/{id}/answers` body `{questionId, value?, confirmed?, cancelled?}`. `questionId` required.
- `POST /api/runs/{id}/injections` body `{text}`. Non-empty text required; max 64 KiB.
- `POST /api/runs/{id}/approve` body `{nodeId, decision: "approve"|"reject", attempt?}`. `approved` remains accepted as a compatibility alias when `decision` is absent; both absent is 400.

Status mapping uses typed engine errors:

- 400 malformed JSON or missing required field.
- 404 unknown run (`ErrRunNotFound`).
- 409 wrong state, node disallows injection, missing capability, full queue.
- 410 interaction already resolved or expired (`ErrInteractionResolved`).

The handlers stay thin: decode, validate, call one engine method, map the error. No HTTP handler writes to a channel directly, preserving the existing boundary.

### 8. Cancellation

`Cancel` already cancels the run context and waits for `done`. Keep that. Add cleanup:

- `awaitApproval` selects on `ctx.Done()`, clears the request, and returns `ctx.Err()`; the existing `finishCancelled` path marks the node cancelled.
- `Session.Ask` already selects on `ctx.Done()` and `done`; with the resolution record it must publish `question.cancelled` only when the cause is run cancellation, not when the session closes after a normal finish.
- Every `finish*` path clears `Run.Pending` and the approval request under the run mutex.

No new goroutines are introduced. The only existing goroutine is the run loop, and `Engine.Close` already cancels it.

## Risks / Trade-offs

- **Automatic retry becomes attended** → expected consequence of per-attempt approval. Document it in the workflow comments and README; operators who want unattended retries should not mark the node `require_approval`.
- **Bounded inject queue can reject under load** → 256 entries, explicit 409 plus `injection.rejected`, and the message is never silently lost. The client can retry.
- **Event rename breaks existing consumers** → there are none in the repository today; update README and tests in the same change. If an external consumer appears, keep `question.resolved` as a deprecated alias instead of breaking it.
- **A crash leaves `pending` in `run.json`** → the run is already stuck in the current code when its process disappears (Cancel requires a live run, Resume accepts only failed/cancelled). This design does not make it worse; reconcile orphaned waits as a follow-up by failing them at startup or allowing them to resume.
- **Timeout tests can be flaky** → use the existing `waitFor`/`waitTerminal` loops with millisecond values, and assert events rather than wall-clock durations. No test may depend on the 30-minute default.
- **Run record growth** → injection history is capped at 100; pending is a single object; question text is already part of the event journal.
- **Saving before emitting adds a file write to every interaction** → runs already save on every node transition; interaction transitions are rare. The clearer ordering is worth the write.

## Migration Plan

- No data migration: new `Run` fields decode as zero on old records; old runs simply have no pending interaction or injection history.
- Add the timeout fields as optional; workflows without them keep the existing fallbacks: 30 minutes for approval, and the harness's suggested question timeout (or 30 minutes) for questions.
- Ship the event rename with the README and test updates in the same commit; there is no external client to stage.
- Rollback is a code revert. Run records written by the new code remain readable by the old code because the added fields are optional.

## Open Questions

None that affect the specs, approach, or task breakdown. Orphaned waiting runs are a known follow-up, not a decision this change needs.
