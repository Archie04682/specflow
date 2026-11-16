# Design

## Context

See `proposal.md` for motivation. The current state the approach builds on:

- `backend/internal/orchestrator/workflow.go` parses YAML into `Workflow`; `LoadWorkflow` reads a path and falls back to that path as the identity when the file declares none.
- `validate.go` checks identifiers, kinds, edges, cycles, timeouts, and postconditions. `Engine.Create` calls it; `Engine.load` (used by resume, retry, and every run lookup) does not.
- Resolution is spread out: `prepare.go` owns `ResolveModel`, `ResolveWorkdir`, and `Preflight`; `workflow.go` owns `EffectiveHarness`, `EffectiveInteractive`, `EffectiveInject`, `TimeoutDuration`, and `Order`; `engine.go` resolves the skill inside `runAgent`.
- `Run` stores `WorkflowPath` only, so `load` reparses whatever is on disk. `Store.Save` writes `run.json` atomically, and `.specflow/runs/<id>/` already holds `run.json` and `events.jsonl`.

Constraints that shape the design: the orchestrator stays harness-agnostic; the single binary gains no dependency; execution is one node at a time; the repository and run record are the state; the workflow YAML schema does not change.

## Goals / Non-Goals

**Goals:**

- One compile point: authored YAML in, executable plan out, and the engine sees only the plan.
- Fail run creation before a worktree exists when the definition cannot run in the project's current environment.
- Pin each run to the definition it was created with, so resume cannot execute a different graph.
- Keep resolution rules tested once, instead of exercised lazily at every node.

**Non-Goals:**

- Adding a `load` command or workflow HTTP endpoints; the plan is made JSON-friendly, but no surface is built here.
- Persisting the compiled plan. The snapshot is the durable artifact.
- Checking that the harness name exists in the registry at creation; that remains preflight's job.
- Changing prompt rendering, postconditions, retry, approvals, sessions, or capability preflight.
- Adding workflow-level defaults for working directory, timeout, retry, or tools.

## Decisions

### A separate, flattened `WorkflowPlan`

Compilation produces a new type; it does not mutate `Workflow` in place:

```go
type WorkflowPlan struct {
    ID, Name, Description string
    Version int
    Source  string     // the path the authored source is known by
    Nodes   []PlanNode // declaration order
    Order   []string   // topological order
}

type PlanNode struct {
    ID, Title string
    Kind      NodeKind
    Harness   string          // concrete; empty for command nodes
    Skill     *harness.Skill  // absolute paths, nil when none
    Model     *harness.Model  // nil means "harness chooses"
    Interactive, AllowInject bool
    Workdir   string          // "run" or "project"
    Timeout   Duration        // marshals as "20m", not nanoseconds
    Retry     int
    Tools     []string
    Prompt, Command string
    RequireApproval bool
    Postcondition *Postcondition
}
```

`Workflow` stays the parse model: it is what validation reads and what a future editor or API can show as authored. The plan carries only effective values and JSON tags.

Alternatives considered: normalizing the `Workflow` in place (rejected: authored and derived fields become indistinguishable, and the engine can still read the wrong one); keeping lazy resolution and adding a resolved view (rejected: two resolution paths that drift, and no load-time failure); embedding the raw `Node` in `PlanNode` (rejected: promoted fields would let the engine read an unresolved `Harness` by accident).

### Compile from the exact bytes that get snapshotted

The compile entry point takes bytes and a source label:

```go
func ParseWorkflow(data []byte, source string) (*Workflow, error)
func CompileWorkflow(data []byte, source string, ctx NormalizeContext) (*WorkflowPlan, error)
type NormalizeContext struct { SkillRoots []string }
```

`LoadWorkflow(path)` remains a thin `os.ReadFile` + `ParseWorkflow` for tests and other read-only callers. `Engine.Create` reads the file once and hands the same slice to compilation and to the snapshot write, so the bytes that were resolved are the bytes that are retained. The `source` label is what an empty workflow `id` falls back to; resume passes the original path even though it reads the snapshot, keeping the fallback identity stable.

Alternatives considered: compile by path, then copy the file (rejected: an edit between the two steps makes the snapshot differ from what ran); write the snapshot first and compile it (rejected: needs a run directory before the definition is known to be valid, and leaves an orphan on every compile failure).

### Normalization composes the existing resolution helpers

Nothing about the resolution rules is new; only their timing moves. Normalization calls the helpers that already have tests and changes their results into concrete plan fields:

| Plan field | Source of truth |
|---|---|
| `Harness` | `EffectiveHarness` (node, then defaults) |
| `Model` | `ResolveModel` (node, then defaults, sentinel dropped; nil when empty) |
| `Interactive`, `AllowInject` | `EffectiveInteractive`, `EffectiveInject` |
| `Workdir` | node's mode, `run` when omitted |
| `Timeout` | `TimeoutDuration` |
| `Skill` | `ResolveSkill` over `ctx.SkillRoots`, nil when unnamed |
| `Order` | `Workflow.Order()` |

The working directory is stored as a mode, not an absolute path: the worktree path is created later and belongs to the run, not the definition. At node start the engine binds `run` to the worktree and `project` to the project root. Model identifiers stay unvalidated. No new defaults are introduced; the omitted values above resolve to the defaults the engine already applied.

Validation runs first. A structurally invalid workflow returns the existing aggregated `ValidationError` and is not normalized. A structurally valid workflow is normalized, and every missing skill is collected and returned in one `ValidationError` as well, so callers and tests keep a single error shape.

### Snapshot before the worktree, then persist the run

`Engine.Create` becomes:

1. Resolve the workflow path; read its bytes once.
2. Compile with the project's skill roots.
3. Generate the run identifier.
4. Write the source bytes atomically to `.specflow/runs/<id>/workflow.yaml`.
5. Create the worktree and the run record, with `Nodes` derived from the plan.

A compilation failure happens before step 3, so a rejected definition leaves no run directory and no worktree. If a later step fails, creation unregisters and removes a worktree created before the failure, removes the incomplete run directory, and best-effort deletes the branch. The run record adds:

```go
WorkflowPath     string `json:"workflowPath,omitempty"`     // original, for display
WorkflowSnapshot string `json:"workflowSnapshot,omitempty"` // run-dir copy, resume input
```

Writing the copy before the record means a persisted run cannot lack its definition. The file lives under the run directory, so the existing promise that `.specflow/` metadata may be committed applies to it unchanged.

Alternative considered: write the snapshot after a successful worktree creation (rejected: a failure between the two leaves a run whose definition was never retained).

### Resume compiles the recorded definition; inspection does not

`Engine.load` is split into record loading and plan compilation. `load` opens the project, loads `run.json`, and opens the event hub; `planFor(live)` reads `WorkflowSnapshot`, falls back to `WorkflowPath` for records created before this change, compiles with the current skill roots, and caches the plan on the live run.

Only execution paths call `planFor`: `Start`, `Resume`, `Retry`. `Get`, `List`, `Cancel`, `Approve`, and `Answer` never compile, so a removed skill cannot make a finished run uninspectable. `Inject` uses the plan that is already present on a running execution; a node is only injectable while it runs, and a running node implies the plan was compiled.

Resume passes the original `WorkflowPath` as the source label while reading the snapshot, then compares the plan's node identifiers and kinds with the `NodeRun` records in declaration order. A mismatch is an error naming the run, not an execution of a different node list. A legacy source whose prompts or edges changed cannot be detected without a recorded hash; the snapshot makes that limitation irrelevant for new runs.

The snapshot pins the graph, not the environment. `.specflow/config.yaml`, the skill roots, and the skill contents are read again when the definition compiles, so a skill removed after creation fails resume before any node executes. That is the intended failure: a run may not execute a definition the current project can no longer satisfy.

Alternatives considered: compile inside `load` for every operation (rejected: `GET /api/runs/{id}` would fail because a skill was removed, and every inspection would stat the filesystem); persist the compiled plan and load it directly (deferred: it freezes an internal schema and pins environment-derived paths; the snapshot already pins the graph).

### The engine consumes plan nodes; preparation stays

`liveRun.workflow *Workflow` becomes `liveRun.plan *WorkflowPlan`. `executeNode`, `executeOnce`, `runAgent`, `Preflight`, and the retry paths take `*PlanNode`. `runAgent` reads `node.Harness`, `node.Skill`, and `node.Model` directly; the skill resolution and model resolution calls disappear from execution. `newNodeRuns` is built from the plan, and retry ordering uses `plan.Order`.

What remains at execution time is exactly what depends on run state: adapter lookup and capability preflight, prompt rendering from the run context, postcondition evaluation, checkpoints, retry, and sessions. The resolution helpers stay in the package because normalization and validation call them; no second implementation is introduced.

## Risks / Trade-offs

- **Graph pinned, environment not** → document that resume recompiles the retained definition and can fail on a missing skill; the failure names the skill and searched locations.
- **Raw `Workflow` and `WorkflowPlan` can drift** → table-driven normalization tests over the canonical workflow assert every resolved field, and the mapping lives in one function.
- **An extra file per run** → one small YAML file under the run directory, already covered by the "metadata may be committed" rule; update the documented `.specflow` layout.
- **Create can leave state after a late failure** → best-effort unregistering and removal of a worktree, removal of the incomplete run directory, and deletion of the branch; compilation itself is before any directory is written.
- **Legacy run with no snapshot and an edited source** → the node-list check rejects a changed set, order, or kind; prompt and edge edits cannot be detected without a recorded hash, but new runs carry the snapshot.
- **Inspection and execution can disagree** → `GET` succeeds while `resume` fails on a missing skill; this is deliberate.
- **Empty workflow `id`** → the source label, not the snapshot path, drives the fallback, so the run identity survives resume.
- **Plan JSON shape** → durations marshal as strings and an empty model is omitted; tests cover both so the future surface is not locked into nanoseconds or `{}`.
- **Resume recompiles** → one filesystem walk over the skill roots per resumed execution, and the create-then-start path reuses the in-memory plan.

## Migration Plan

- The run record change is additive; records without `workflowSnapshot` remain readable and resume from their recorded `workflowPath`.
- Newly created runs retain a snapshot; no data migration and no YAML or `.specflow` configuration change.
- Rolling back to the previous binary is safe: it ignores the new field and the snapshot file and keeps loading the original path.
- The API gains one additive field on run responses; no existing field changes meaning.

## Open Questions

- Whether to persist the compiled plan later, so exact effective values survive a config change. Deferred; the snapshot covers graph identity.
- Whether to check harness registration at creation rather than preflight. Deferred; the registry check remains where capabilities are already checked.
- Whether to expose the plan through a CLI or HTTP surface. Deferred; the plan's JSON tags make either cheap.
