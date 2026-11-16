# Design

## Context

See `proposal.md` for motivation and `specs/` for the required behavior.

The repository already contains the layer this change sits on. `backend/internal/harness` defines `Adapter` (`Descriptor`, `Capabilities`, `Probe`, `Run(ctx, Request, Host) (Result, error)`), a single terminal `Result`, a `Failure` with a `Retryable` flag, and a `Host` with `Emit`, `Ask`, and `Injections`. `harness.Validate(Needs, Capabilities)` already answers preflight. `backend/internal/harness/pi` implements all of it. `backend/internal/{orchestrator,session,openspec}` and `workflows/standard-feature.yaml` exist, but the first three are documentation stubs and nothing reads the workflow file.

Constraints that shape the approach:

- Single binary, no new runtime dependency, cross-compilation must keep working.
- The orchestrator must never learn which harness executes a node.
- Behavior changes are specified here before they are built.
- The target project is the user's repository, not this one; this repository is the tool.

## Goals / Non-Goals

**Goals:**

- Execute the canonical OpenSpec flow end to end against a real project, with an isolated workspace and a resumable record of what happened.
- Make node success a repository fact, not an exit code.
- Keep the engine's data model small enough that adding a harness or a node kind does not disturb it.

**Non-Goals:**

- The graph editor. This change exposes the API a UI would consume; it does not build the UI.
- Concurrent execution of independent branches. The graph may declare them, but this change runs one node at a time.
- Loops, condition nodes, and node-internal worker fan-out.
- SQLite, queues, Docker, multi-user, or any remote execution.
- Sandboxing. The harness runs with the user's privileges inside the worktree, as it does today.

## Decisions

### The repository is the state, so edges carry ordering only

A node receives a rendered prompt, a working directory, and a resolved skill. The contract says the adapter never learns about the workflow or prior nodes, so everything an agent needs must already exist on disk or in the prompt. Passing a JSON context map between nodes would duplicate what the files are, grow token cost with every edge, and require an expression language to be useful.

Alternatives: a Dify-style context map with `{{path.to.value}}` interpolation (rejected: duplicates repository state, unbounded prompt growth, needs a templating language); per-node typed ports with schema validation (rejected: more machinery than eight nodes justify).

The run context is therefore a fixed record:

```text
objective   the user's goal, from the run request
change      the OpenSpec change, resolved after propose
run_id      the run identifier
workdir     the node's working directory
```

The prompt renderer substitutes exactly these and rejects any other placeholder before the node starts, so a typo fails fast instead of reaching a model as literal braces.

### Two node kinds, and gates are commands

`agent` runs through a harness. `command` runs a shell command in a working directory and produces an exit code.

A gate must be deterministic, so `verify` is a `command` node. If the agent executed the gate, a failing gate could be "fixed" by the thing being gated, which defeats it. This also matches the project's standing decision that verify is a gate, not a skill, and corrects the node list in `workflows/standard-feature.yaml`, which currently gives `tests` and `verify` both a command and a harness.

Alternatives: one node kind with flags (rejected: every consumer would branch on the flags anyway); a dedicated `gate` kind (rejected: it is a command whose result is trusted, and a second kind buys nothing).

### Phases are configuration, not node types

A block named `propose`, `apply`, or `verify` carries no behavior in its name. Everything that makes it that phase — the skill, the harness, the model, the postcondition, the approval requirement — is a field in the workflow file. Rename a block and the flow still works as long as its fields and edges are kept; delete its `skill` and a specific phase becomes a generic agent step.

The canonical flow demonstrates it. `propose` is special only because it carries `postcondition: new_change`, and that postcondition is what writes the resolved change into the run context. Nothing in the engine knows what a proposal is, and the block could be called `plan` without any other change.

This is what makes a saved flow portable and lets a user add a phase — a security review, a migration check — without touching Go. The boundary is deliberate: a new *phase* is YAML, a new *behavior* is code. Prompt variables stay a closed set of four and postcondition types a closed set of five; opening either would mean building a template language and a plugin system for eight nodes.

Alternatives: a node type per phase (`explore`, `propose`, `apply`, …) (rejected: every consumer would branch on the type, and a user's extra phase would need a code change); free-form prompt variables and postconditions (rejected: no bounded surface to validate, and the engine would have to interpret user expressions).

### Each run gets a worktree on its own branch

`git worktree add -b specflow/run-<id> .specflow/worktrees/<id> <base>` gives the run a private checkout without moving the user's. Checking out a branch in place would hijack whatever the user is editing, and a clone would be far more expensive for large repositories.

The branch cannot be renamed once `propose` reveals the change name: the worktree has it checked out, and a move buys nothing that a `change` field in the metadata does not already provide.

Alternatives: `git checkout -b` in the project (rejected: mutates the user's checkout, risks uncommitted work); clone (rejected: cost, and the worktree already shares the object store).

### Checkpoints are commits after nodes, never before

A node's starting state is the previous node's checkpoint, so only one commit is needed per node and the base commit covers the first node. Committing before every node would create empty commits whenever a node changed nothing.

`git add -A` followed by a commit when the index is dirty captures whatever the node did, including work the agent committed itself. When the node staged nothing, the preceding checkpoint is recorded instead. Checkpoint commits are attributed to a fixed identity so a fresh environment without `user.name` configured does not fail the run.

The same commits give the UI per-node diffs for free: `git diff <pre>..<post>`.

### Postconditions are evaluated against the repository

Harness exit codes are weak evidence: an agent can exit zero having changed nothing. Each node may therefore declare a postcondition, and the engine evaluates it after the adapter reports success. A failed postcondition is an orchestration failure, not a harness failure — the adapter finished, the repository disagrees.

The vocabulary is deliberately small: `file_exists`, `new_change`, `tasks_complete`, `exit_zero`, and `none`.

`new_change` is relative, so it needs a baseline. The change list is snapshotted before the run starts and stored in the run record; the postcondition compares against that stored baseline rather than re-deriving it. This is what makes a resumed run able to re-check `propose` without finding "no new change". When several changes appeared, the node fails as ambiguous; picking one silently would attach every later step to the wrong change.

`tasks_complete` reads unchecked items from the change's `tasks.md` with a line-anchored `- [ ]` match. It is the most brittle check in the set, which is why it is limited to that one form and covered by tests rather than being made clever.

### Retry has three entry points, and only one of them is automatic

- **Fix-forward** re-runs the node on the current workspace with the previous failure included in the prompt, bounded by the node's `retry` limit. This is the default because an agent that sees its own error often repairs forward better than it restarts.
- **Clean retry** restores the node's starting checkpoint (`git reset --hard` plus removal of untracked files, never `-x`, so ignored files survive) and runs the node again. It exists for the case where the attempt left the workspace worse than it found it.
- **Resume** re-enters the graph at the failed node. Completed nodes are not re-executed: their effects are already in the branch and their results in the run record.

Automatic retry applies only when the adapter classified the failure as retryable and attempts remain. A postcondition failure is always a user choice, because the right answer depends on why the repository disagrees.

Alternatives: failure edges to a fixer node (rejected for this change: needs cycles and guards, and the agent already loops internally); retrying the whole run (rejected: wasteful and it re-applies completed work).

### The session manager is the `Host`

One `session.Session` exists per node execution and implements `harness.Host`. `Emit` appends to the run's event log and publishes to subscribers. `Ask` publishes a question, stores its pending channel, and blocks until an answer, a cancellation, or the deadline. `Injections` is fed by the API and closed when the node ends.

The run's status is derived from what it is waiting on: `waiting_approval` before a gated node, `waiting_answer` while a question is outstanding, otherwise `running`.

### Events are sequenced per run and replayable

The harness already numbers its events per execution, which is not usable across nodes. Each run therefore owns its own counter, and every event from any source is stamped with it before being stored. The SSE endpoint accepts the standard `Last-Event-ID` header and replays the retained events after it, so a refresh or a dropped connection does not lose the middle of a run.

Events are appended to `.specflow/runs/<id>/events.jsonl` and retained in memory for the live subscribers. The file is the replay source after a restart.

### Run metadata is JSON, written atomically

`.specflow/runs/<id>/run.json` holds status, node results, checkpoints, the resolved change, and the workspace facts. Each transition writes the whole record to a temporary file and renames it, so a crash cannot leave a half-written record.

Alternatives: SQLite (deferred: adds a dependency and cgo concerns for a local single-user tool; the JSON record is sufficient for tens of runs, and the store is behind an interface).

### Worktrees are excluded by `.specflow/.gitignore`

`.specflow/.gitignore` contains `worktrees/`. Git honours nested ignore files, so the worktree contents never appear in `git status` while the rest of `.specflow/` stays committable if the user wants run history in version control. This avoids editing the user's root `.gitignore`.

### Merge is the last node, and it runs in the project

`git merge` cannot run in the run worktree, which already has the run branch checked out; git refuses to check out the source branch twice. The merge node therefore declares `workdir: project`, runs a plain `git merge --no-ff specflow/run-<id>`, and requires approval. The engine refuses the command when the project is on a different branch or has uncommitted changes, because a merge into an unexpectedly dirty checkout is how work gets lost.

Alternatives: a button after the run (rejected: the flow would no longer finish the job); updating the ref directly with `git fetch` (rejected: leaves the user's checkout stale).

### One node at a time

DAG ordering decides which node is eligible; the engine then runs one node at a time. Independent branches are represented in the graph and documented as sequential for now. Two agents editing the same worktree concurrently would corrupt each other's work, and a worktree per branch is a separate problem from running the flow end to end.

### Packages

```text
internal/workspace     project, .specflow layout, git worktree, checkpoints, merge preconditions
internal/orchestrator  workflow model, validation, engine, postconditions, retry, run store
internal/session       harness.Host implementation: events, questions, injections
internal/openspec      skill name resolution and the validation gate
internal/api           run endpoints and the SSE stream
```

`workspace` is a new package because it is testable without an engine and the `run-workspace` capability is independent of `workflow-orchestration`. The engine depends on it; it does not depend on the engine.

### HTTP surface

```text
POST   /api/runs                     create a run from {project, workflow, objective}
GET    /api/runs?project=            list runs for a project
GET    /api/runs/{id}                run, nodes, checkpoints, resolved change
GET    /api/runs/{id}/events         SSE, honours Last-Event-ID
POST   /api/runs/{id}/answers        resolve a pending question
POST   /api/runs/{id}/injections     deliver a clarification
POST   /api/runs/{id}/cancel         cancel the run
POST   /api/runs/{id}/approve        resolve a pending approval
POST   /api/runs/{id}/retry          {nodeId, mode: forward|clean}
POST   /api/runs/{id}/resume         re-enter at the failed node
DELETE /api/runs/{id}                discard the run's worktree
```

Every failure response is JSON and names the run and the node it concerns, matching the existing `/api/*` error shape.

## Risks / Trade-offs

- **No sandbox.** An agent can write anywhere the user can, including outside the worktree. Mitigation: the worktree bounds accidental damage, the harness keeps its own tool policy, and node-level `tools` can narrow it; real containment is a later change.
- **`tasks_complete` parses markdown.** A different checkbox convention silently passes. Mitigation: line-anchored match, a scenario per convention that is supported, and an explicit failure naming the offending task.
- **Worktrees cost disk and inodes on large repositories.** Mitigation: remove on completion, and  a worktree shares the object store so the cost is a checkout, not a clone.
- **Checkpoint commits can fail without a git identity.** Mitigation: fixed fallback identity on checkpoint commits only, never on user commits.
- **A crash between the adapter finishing and the checkpoint being written loses that node's result.** Mitigation: the run record is written before and after each node, and resume re-enters at the failed node; the worst case is re-running one node, with its code changes still on disk.
- **Sequential execution makes a fan-out graph slower than it looks.** Mitigation: documented, and the graph still models the intended structure; concurrency is a later change with its own workspace story.
- **Merging into the wrong branch.** Mitigation: the source branch is recorded at run creation and checked before the merge; a mismatch refuses rather than guesses.

## Migration Plan

No data migration. The first run in a project creates `.specflow/` and its `.gitignore`. Rolling back is deleting `.specflow/` and the `specflow/run-*` branches; code changes made in a worktree that was merged are ordinary commits and are not affected.

## Open Questions

- Whether `GET /api/runs` should filter by status or time range. Deferred until the UI exists.
- `.specflow/config.yaml` currently carries only `skills`; default harness/model/thinking are taken from the workflow file, not the project. Revisit if per-project overrides become useful.
