# Proposal

## Why

Ask, inject, and approval are wired end to end, but the interactive lifecycle is not hardened. Approval is attached to a node instead of an attempt, approval has no deadline, inject can block when its queue fills, and a waiting interaction is invisible in the run record. Without those properties a run can wait forever, a retry can run on a stale decision, and a reconnecting client cannot tell what the run is waiting for.

## What Changes

- Approval is requested per **attempt**: every automatic or user-initiated retry re-enters the approval gate. The request carries a unique id, the attempt number, the node action, and a deadline.
- Approval gains a configurable timeout. Expiry fails the node and the run with a timeout failure; rejection keeps its current meaning (the attempt never runs; a node that never ran is reported skipped, and the run ends cancelled).
- Ask becomes one active question per node: a second question waits for the first to resolve instead of overwriting it. Ask gains a configurable timeout (node/workflow default), and its resolution is observable as answered, timed out, or cancelled instead of one generic event.
- Inject becomes non-blocking on both the API and engine sides. Pending messages accumulate in a bounded queue; overflow is rejected with an explicit reason instead of blocking the request. Delivery acknowledgement records the adapter's disposition.
- The run record gains a single `pending` interaction snapshot (kind, id, node, attempt, question/action, requested time, deadline) so a reconnecting client can reconstruct the wait without replaying the journal.
- **BREAKING** The SSE event taxonomy gains the missing lifecycle events and splits the generic question resolution: `question.resolved` is replaced by `question.answered`, `question.timed_out`, and `question.cancelled`; `injection.sent` is replaced by `injection.received`, with `injection.acknowledged` and `injection.rejected` added; `approval.granted` and `approval.timed_out` are added. There is no frontend consumer yet, so this is the low-cost moment to align the names.
- Interactive endpoints validate required fields and map outcomes to 200/400/404/409/410. Inject answers `queued` (delivery is asynchronous), not `delivered`.
- Timeouts are configured per workflow and overridden per node: `defaults.ask_timeout` and `defaults.approval_timeout`, with `ask_timeout`/`approval_timeout` on a node, compiled into the plan. The default is 30 minutes.
- The `harness.Host` interface does not change; all work is in the host implementation, engine, run record, and HTTP layer.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `workflow-orchestration`: approval is per attempt and timed; questions are serialized, timed, and resolved observably; injection is non-blocking with a bounded queue and recorded disposition; a pending interaction is visible in run state; the interactive event lifecycle is complete.

## Impact

- Code: `backend/internal/session` (ask serialization, race-free resolution, inject queue), `backend/internal/orchestrator` (approval per attempt, pending state, timeouts, events), `backend/internal/api` (validation, status mapping, payloads), `workflow.go`/`compile.go`/`plan.go` (timeout fields).
- API/SSE: new request fields (`decision`, optional `attempt`), new/renamed events, `410 Gone` for resolved interactions, inject returns `queued`.
- Configuration: two optional timeout fields at workflow defaults and node level; existing workflows keep working.
- Tests/docs: engine, session, API, and SSE tests; README event and endpoint tables.
- No new dependencies, no adapter-contract change, no WebSocket, no multi-user support.
