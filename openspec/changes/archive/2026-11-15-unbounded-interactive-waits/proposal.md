# Proposal

## Why

A run fails when the operator takes longer than 30 minutes to answer a question: the node's execution
timeout keeps running while the harness is blocked on a human, and Pi reports the expired deadline as
`timeout: execution timed out: context deadline exceeded`. The same fallback makes an approval gate
fail the run after 30 idle minutes. Operators expect an agent harness to wait for them — DeepSeek
Harness waits indefinitely — not to lose a run because they stepped away.

## What Changes

- **Interactive waits have no fallback deadline.** Questions and approval gates wait until the
  operator answers or decides, or the run is cancelled. A configured `ask_timeout` or
  `approval_timeout` (node or workflow default) still bounds the wait, and a question still honors
  the harness's suggested timeout when nothing is configured.
- **The execution timeout counts work, not waiting.** While a question is pending, an agent node's
  remaining execution time pauses and resumes when the question resolves. Waiting can no longer
  consume the node timeout.
- **An unbounded interaction is visible as such.** The pending interaction and its event omit the
  deadline instead of carrying a placeholder; the interaction cards already render "no deadline".
- **README documents the new wait semantics**, and the fallback constants disappear.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `workflow-orchestration`: interactive waits resolve deadlines only from configuration (no 30-minute
  fallback), and an agent node's execution timeout pauses while a question is pending.

## Impact

- **Backend**: `internal/session` (no default question deadline), `internal/orchestrator`
  (approval default removed, new paused execution clock, pending/event deadline omitted), and their
  tests.
- **Frontend**: no behavior change; the ask and approval cards already omit an absent deadline, and
  tests pin that.
- **Docs**: `README.md` interactive-waits section; an AGENTS.md orchestrator note.
- **Wire**: `deadline` already optional in the pending record; it is now omitted when unbounded.
