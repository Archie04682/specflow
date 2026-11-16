# Design

## Context

See `proposal.md` for motivation and the delta spec for the contract. The current state that shapes
the approach:

- `runAgent` wraps the whole adapter call in `context.WithTimeout(ctx, node.Timeout)`
  (`backend/internal/orchestrator/engine.go:921`); an agent node defaults to 30 minutes
  (`Node.TimeoutDuration`). A question is answered inside that call, so the operator's thinking time
  is spent from the node's budget.
- `session.Ask` falls back to `DefaultAskTimeout` (30 minutes) when neither the plan nor the harness
  supplies a deadline, and always starts a timer. Timeouts surface as `question.timed_out` and the
  model continues with no answer.
- Approval gates run before the attempt (`awaitApproval` from `beginAttemptCycle`), outside the
  execution context, and fall back to a hard-coded 30-minute deadline.
- The Pi adapter classifies an expired context as `FailureTimeout` by checking
  `errors.Is(ctx.Err(), context.DeadlineExceeded)`, so whatever replaces the deadline context must
  report `context.DeadlineExceeded` from `Err()`.
- The interaction cards already render an absent deadline correctly (`AskCard` shows "no deadline",
  `ApprovalCard` omits the suffix), and the pending record and event carry the deadline as an
  optional field.

## Goals / Non-Goals

**Goals:**

- Human waiting never counts as agent execution time.
- Questions and approvals wait for the operator by default, with configuration as the only bound.
- Adapters and the harness contract stay unchanged; the timeout classification keeps working.

**Non-Goals:**

- Changing command-node timeouts (a command has no human wait).
- Persisting a pending question across a server restart.
- New UI: the interaction cards already express "no deadline"; only tests pin it.
- Changing approval semantics beyond its deadline (still one decision per attempt, rejection cancels).

## Decisions

### D1. A paused execution clock, not a bigger timeout

The agent call receives a custom `context.Context` implementation (`executionClock`) that tracks
*active* time instead of an absolute deadline. `Pause()` stops the timer and banks the remaining
budget; `Resume()` restarts it; when the budget expires the clock cancels its `Done` channel and
reports `context.DeadlineExceeded` from `Err()`. The parent run context is watched so cancellation
still propagates immediately.

*Alternatives:* raising the default timeout only moves the surprise failure; moving the timeout into
the harness contract would widen `Adapter` for a host concern; an adapter-owned timeout would have to
be implemented twice and would still count waiting; extending a `context.WithTimeout` deadline on
resolution is not possible. A custom context keeps the change inside the engine and preserves Pi's
existing timeout classification, because the expiry still reads as `DeadlineExceeded`.

### D2. Zero deadline means unbounded, for both interactions

`session.Ask` only arms a timer when a configured or harness-suggested timeout exists; `awaitApproval`
only arms one when `approval_timeout` resolves. `DefaultAskTimeout` and the approval fallback vanish.
When the deadline is zero, the pending record and event omit it, so the UI's existing "no deadline"
presentation becomes the honest state. A configured timeout remains the escape hatch and keeps the
existing timeout scenarios.

*Alternatives:* a very long default (for example 24 hours) still fails a forgotten run silently and
needs a new arbitrary number; keeping 30 minutes is the reported bug.

### D3. Pause at the session sink, not inside the adapter

`runAgent` creates the clock and attaches it to the node's `sessionSink`. `Question` pauses and
`Resolved` resumes, which brackets exactly the interval the operator is being waited on and needs no
adapter awareness. Approval waits already sit outside the execution context, so they need no pause.

*Alternatives:* pausing inside each adapter duplicates the logic per runtime; pausing on every
telemetry gap would stop the timeout for slow models, which is not the goal.

## Risks / Trade-offs

- **A run can now wait forever** → the run visibly reports `waiting_answer` / `waiting_approval`, and
  cancel, resume, and discard keep working; workflows that want a bound set `ask_timeout` /
  `approval_timeout`. README says so.
- **Context implementation must honor the contract** → `Done`, `Err`, `Deadline` (false), `Value`
  (delegating), and parent cancellation are covered by unit tests, including expiry returning
  `DeadlineExceeded` so the adapter still classifies a timeout.
- **Timer/pause/resume races** → all clock state is mutex-guarded, the timer callback re-checks the
  paused/stopped state, and finishing is guarded so `Done` closes once.
- **Missing deadline in old consumers** → the field was already optional; the frontend asks
  `formatDeadline` only when present and shows "no deadline" otherwise, pinned by tests.
- **Docs drift** → README's interactive-waits section and the AGENTS.md orchestrator note are part of
  the change tasks, not a follow-up.

## Migration Plan

No data migration. Workflows without interaction timeouts simply wait; workflows with them behave as
before. Runs created before the change keep their compiled timeouts and their recorded pending
deadlines. Rolling back restores the 30-minute fallbacks with no data cleanup.
