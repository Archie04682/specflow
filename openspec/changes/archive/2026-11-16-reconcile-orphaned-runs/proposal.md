# Proposal

## Why

Live runs, approval requests, and question gates exist only inside the server
process, while the run record survives a restart. A restart during a wait
therefore leaves a persisted run advertising `waiting_approval` or
`waiting_answer` that nothing can resolve: approving returns "interaction is not
pending", and retry, resume, and cancel all refuse a waiting status. The
operator is left with a run that is visible but unrecoverable. This repository
hit it when a restart interrupted a `propose` approval: the pending card stayed
on screen, the approval could never succeed, and only discarding the run
remained.

## What Changes

- **A restart's in-flight runs are repaired when observed.** When the server
  loads a run whose persisted status is `running`, `waiting_approval`, or
  `waiting_answer` and the process holds no execution for it, it SHALL treat the
  execution as interrupted: clear the pending interaction, fail the interrupted
  node with a retryable reason naming the restart, mark later unfinished nodes
  skipped, end the run failed, and publish the node and terminal run events.
- **The repair mirrors a normal failure.** The run keeps its worktree,
  checkpoints, and resolved change, and stays retryable and resumable exactly
  like any failed run — so the existing Retry and Resume controls work without a
  UI change.
- **Repair is lazy, bounded, and idempotent.** The server has no project
  registry at boot (projects live in the browser), so repair happens where runs
  are observed: run reads, the project's run list, and the interaction, retry,
  resume, cancel, and discard paths. Concurrent observations repair once. A run
  executing in the current process, a terminal run, and a run that never started
  are left untouched.
- **The interaction error becomes accurate.** An approval or answer submitted
  for an interrupted run is refused as already resolved (`410`) instead of
  "interaction is not pending"; the dashboard already turns that response into a
  notice and a refetch.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `workflow-orchestration`: adds the interrupted-run repair requirement with its
  scenarios — approval wait, answer wait, running node, later nodes, live and
  terminal exclusions, idempotency, list repair, and an unreadable journal.

## Impact

- `backend/internal/orchestrator`: repair in the run-loading path and the run
  list; one helper that reuses the existing failure, skip, save, and event
  helpers; an engine-level lock so concurrent observations repair once.
- No API shape change, no frontend change, no new failure class, no new
  dependency.
- `README.md` and `AGENTS.md`: note that a restart interrupts an in-flight run
  and leaves it resumable.
- Tests cover both waits, a running node, later-node skipping, idempotency and
  concurrency, live and terminal exclusion, list repair, the post-repair
  interaction response, and an unreadable journal.
