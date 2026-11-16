# Design

## Context

See `proposal.md` for motivation. The constraints that shape the approach:

- Run records are persisted atomically at every transition in
  `.specflow/runs/<id>/run.json`; the engine's live set, approval requests,
  harness sessions, and child processes exist only in memory.
- `loadRun` is the single choke point every path uses: it returns the cached
  live run when present, otherwise builds a `liveRun` from the store (caching it
  only for mutations). `inspect` loads without caching.
- `List` reads the store directly and overlays live snapshots; it never loads a
  record into a `liveRun`.
- The dashboard treats the REST run record as the truth for node and run state.
  It renders the pending interaction from that record and already handles `410`
  on an interaction by showing "This interaction already resolved." and
  refetching.
- Emitting events requires the run's hub log to be open; `markFailed` emits
  `node.failed`, `skipRemaining` emits `node.skipped`, and `failRun` emits
  `run.failed` then seals the log. A terminal replay reopens the journal on
  demand.
- The server has no project registry at boot — projects live in the browser —
  so a boot-time sweep cannot discover what to repair.
- Observed failure: after a restart, a `waiting_approval` record stayed on
  screen; approving returned `ErrInteractionNotPending`, and retry, resume, and
  cancel all refuse a waiting status.

## Goals / Non-Goals

**Goals:**

- No persisted run stays non-terminal after a restart once anything observes it.
- A repaired run is an ordinary failed run: worktree, checkpoints, and resolved
  change intact; Retry and Resume work; the record explains the restart.
- Repair is idempotent, safe under concurrent reads, and never touches live or
  terminal work.
- The event narrative stays coherent: `node.failed`, then `node.skipped`, then
  `run.failed`, sealed.

**Non-Goals:**

- No automatic resume or re-execution; the operator chooses Retry or Resume.
- No boot-time sweep across projects (no registry exists server-side).
- No multi-instance ownership of runs.
- No repair of `pending` runs: they never had an execution, and excluding them
  keeps the Create → Start window inside one request safe.
- No new failure class, no API shape change, no frontend change.

## Decisions

### Repair at the load choke point and in the list

`loadRun` repairs a non-live run whose status is `running`,
`waiting_approval`, or `waiting_answer` after loading it from the store and
before returning or caching it. That covers every reader and every mutation —
`Get`, `Subscribe`, `Approve`, `Answer`, `Inject`, `Cancel`, `Discard`, `Retry`,
`Resume`, and `Start` — without touching each method. `Start` loads a
just-created `pending` run, which the status filter excludes, so creation is
unaffected.

`List` reads records directly, so it gets an explicit pass: for each
non-terminal, non-live record, call `loadRun(..., cache=false)` and use the
repaired snapshot. Without it the tree would advertise a waiting run until the
operator opened it, which is the state this change exists to remove.

Alternative rejected: a boot-time sweep. The server cannot enumerate projects
at boot; a lazy repair at observation is the equivalent guarantee for every
state a client can actually see.

### The repair mirrors a normal failure

1. **Resolve the node.** Prefer the pending interaction's node, then the run's
   current node, then the first node with status `running` or
   `waiting_approval`. A node that already finished is never chosen: between
   two nodes the record still names the previous, completed node, and rewriting
   it would lose completed work. If none resolves, fail the run without a node
   failure.
2. **Fail the node** with a retryable `harness.Failure` whose class is
   `harness_exit` ("the harness went away with the server", the fixed class
   closest to the event) and whose message names the restart and what the node
   was doing: waiting for approval, waiting for an answer, or running. Passing
   the failure through `markFailed` keeps the event's class and `retryable`
   flag consistent, and its `Error()` becomes the node and run error.
3. **Skip later unfinished nodes.** The persisted node list is the compiled
   plan's order. Only nodes still `pending` after the interrupted node are
   skipped, emitting `node.skipped`; already skipped or failed nodes are left
   alone so a retried run does not duplicate their events. A failed run must not
   contain pending nodes — the engine already enforces this when a normal
   failure ends a run.
4. **Fail the run** through `failRun`: it clears `Pending`, sets the error and
   `finishedAt`, clears `CurrentNode`, emits `run.failed`, and seals the log.

Alternatives rejected: ending the run `cancelled` with a cancelled node (the
dashboard only shows the run-error banner for failed runs, and Retry disappears,
so the operator sees less); introducing a dedicated failure class (the class set
is fixed by the adapter contract).

### Events are best-effort; the record is authoritative

Before emitting, open the hub log for the run. If it cannot be opened — a
truncated or oversized journal — log the failure and still repair `run.json`;
a broken journal must not keep the run stuck forever. `failRun` seals, after
which the repair closes the log so the engine holds no handle for a run it does
not own; a later subscriber reopens the journal on demand and replays the
terminal event. The repair logs one line naming the run and the interruption
reason.

### One repair at a time

The engine gains a `reconcileMu` mutex. A non-live load that needs repair takes
it, re-reads the record under the lock, and returns without work if another
request already repaired it; live runs return before the lock. The mutex only
serializes loads of runs without a live execution, which is the rare path. The
alternative — a per-run lock map — adds lifecycle management for an event that
happens once per restart.

The test target is N concurrent `Get`s producing exactly one `run.failed`
event and one skip pass.

### No frontend or API change

Once repaired, the record is `failed` with `pending: null`. The interaction dock
disappears, the run-error banner shows the restart reason, and the existing
Retry and Resume controls appear because the run and the interrupted node are
failed. An approval or answer that races the repair reaches the existing
terminal-run branch and returns `410` (`ErrInteractionResolved`), which the
dashboard already turns into a notice and a refetch.

### Reading may write

Repair is a self-healing write on a read path: idempotent, limited to records
with no owner process, and logged once. The alternative — a background
reconciler — needs project discovery, a scheduler, and a retry policy the server
does not have. A read that repairs is cheaper and reaches clients immediately.

## Risks / Trade-offs

- **Two server processes over one project** would each see the other's live runs
  as orphaned and repair them. The product assumes one server per checkout (one
  port, one worktree per run); this change documents that assumption rather than
  adding ownership stamps.
- **`harness_exit` is a proxy** for "the harness ended with the server", not an
  observed adapter fault. The error message names the restart; a precise class
  would mean widening the fixed adapter contract.
- **A `pending` run whose create request died mid-flight stays pending** — a
  one-request window with no execution to interrupt. It is not repaired.
- **The first read after a restart takes a lock and writes.** Later reads are a
  cheap status check; the list pass only touches non-terminal records.
- **An unreadable journal loses the terminal event**, never the repair.

## Migration Plan

No data migration. Existing stuck records — including the `waiting_approval`
run that motivated this change — are repaired on the first read after deploy,
after which Retry and Resume work normally. Rolling back removes the repair;
records already repaired remain ordinary failed runs.
