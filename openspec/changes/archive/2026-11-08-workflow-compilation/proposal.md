# Proposal

## Why

SpecFlow resolves a node's harness, model, skill, working directory, and timeout lazily, at the moment the node starts. A workflow can therefore pass validation when the run is created and still fail halfway through on a missing skill, and a resumed run reloads the workflow file from disk, so editing that file changes the graph under a run that already exists.

As workflows become the product's primary artifact and the eventual UI edits them, the system needs one point where YAML becomes an executable, inspectable plan — and a run needs to stay on the definition it started with.

## What Changes

- Compile a workflow before a run is created: parse, validate, and normalize it into a `WorkflowPlan` whose nodes carry concrete values. Compilation resolves harness, model and reasoning effort (dropping the `default` sentinel), interactive and injection permissions, working-directory mode, timeout, absolute skill directories, and topological order. The engine never resolves these again.
- Normalize against the project's configured skill roots. A missing skill fails run creation rather than a node halfway through, and every missing skill is reported together with the locations searched.
- Snapshot the compiled workflow source under the run's metadata directory at creation, and record both the original path and the snapshot path in the run record. Resume compiles the snapshot instead of the live file.
- **BREAKING** (run semantics): editing or deleting a workflow file after a run is created no longer changes that run. Previously, resume reloaded the file.
- Keep harness capability preflight, prompt rendering, postconditions, and retry at execution time. The plan carries concrete requirements; adapter capabilities remain run state.
- Do not change the workflow YAML schema. No new defaults, no expression language, and model identifiers remain the adapter's responsibility.

Out of scope: a `specflow load` CLI, workflow HTTP endpoints, persisting the compiled plan itself, and checking harness registration at run creation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `workflow-orchestration`: workflow loading becomes compilation before a run starts; default, sentinel, work-directory, timeout, and skill resolution move into that step; resume compiles the definition recorded with the run.
- `run-workspace`: run metadata retains a copy of the workflow definition the run was created with, alongside the existing record and workspace facts.

## Impact

- `backend/internal/orchestrator` — new plan and normalization types, a compile entry point; `engine.go`, `prepare.go`, `retry.go`, `run.go`, and `store.go` switch to plan nodes and record the snapshot.
- `backend/internal/workspace` — a snapshot accessor under the existing run directory; no layout change beyond one file per run.
- `backend/internal/api` — no new endpoint; `GET /api/runs/{id}` gains an additive `workflowSnapshot` field.
- Tests next to the orchestrator package, plus the existing canonical workflow and HTTP integration tests.
- `.specflow/runs/<id>/` gains `workflow.yaml`; the directory stays committable, as the existing layout already promises.
- No new dependencies. The snapshot is a plain file write and uses the same atomic-write pattern as the run record.
