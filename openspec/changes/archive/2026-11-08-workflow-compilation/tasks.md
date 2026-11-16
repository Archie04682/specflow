# Tasks

## 1. Compilation model

- [x] 1.1 Define `WorkflowPlan`, `PlanNode`, and a plan-local `Duration` that marshals as a duration string, with JSON tags and plan node accessors; verify a test marshals a plan and asserts a timeout renders as `"20m"` and an omitted model is absent from the JSON.
- [x] 1.2 Split parsing from file IO: add `ParseWorkflow(data, source)` and keep `LoadWorkflow(path)` as a read plus parse, using the source path as the empty-id fallback; verify tests load the canonical workflow, assert the fallback identifier, and assert malformed YAML is rejected.
- [x] 1.3 Implement `NormalizeWorkflow` by composing the existing resolution helpers for harness, model, interactive, injection, working-directory mode, timeout, and order, and resolving skill names through `NormalizeContext.SkillRoots`; verify table-driven tests cover a node override, a workflow default, the `default` sentinel, an omitted model, both work-directory modes, kind-dependent timeouts, and an absolute skill path.
- [x] 1.4 Implement `CompileWorkflow` as parse, validate, then normalize: structural defects short-circuit, every missing skill is collected into one `ValidationError`, and model identifiers are not checked; verify tests for the canonical flow, a structural defect, two missing skills reported together, and an unknown model accepted at compile.

## 2. Run creation and snapshot

- [x] 2.1 Add `WorkflowSnapshot` to `Run` and a `WorkflowSnapshotFile(runID)` accessor to the project layout; verify a JSON round-trip test and a legacy `run.json` without the field loading with an empty snapshot.
- [x] 2.2 Make `Engine.Create` read the source once, compile it, write the exact bytes atomically to `.specflow/runs/<id>/workflow.yaml`, then create the worktree and persist the run with the original path, the snapshot path, and plan-derived node records; verify a test asserts the snapshot exists and is byte-identical to the source.
- [x] 2.3 Make a compile failure leave no run directory, worktree, or branch, and make a late creation failure remove the incomplete run directory; verify tests assert the absence of all three after a missing skill and after a forced worktree failure.
- [x] 2.4 Document the run directory contents, including `workflow.yaml`, in `README.md` and `AGENTS.md`; verify the documented file list matches a run directory created by a test.

## 3. Execution and resume

- [x] 3.1 Switch `liveRun` to a compiled plan and update `executeNode`, `executeOnce`, `runAgent`, `Preflight`, `newNodeRuns`, the working-directory binder, and retry ordering to use plan nodes and the plan's precomputed order; verify engine tests pass and a mock-adapter test asserts the request carries the plan's harness, model, and skill values.
- [x] 3.2 Split record loading from plan compilation: `Start`, `Resume`, and `Retry` compile the recorded definition and cache it on the live run, while `Get`, `List`, `Cancel`, `Approve`, and `Answer` do not compile; keep injection gated on the cached plan of the active execution; verify a test removes a skill after creation, asserts `GET` still returns the run, and asserts `Resume` fails naming the skill and the searched locations.
- [x] 3.3 Make resume compile `WorkflowSnapshot` with the original path as the source label, fall back to `WorkflowPath` for legacy records, and reject a plan whose node set differs from the recorded nodes; verify tests for an edited source, a deleted source, a legacy fallback, and a mismatched record.
- [x] 3.4 Remove skill and model resolution from `runAgent` so execution reads only plan fields; verify no execution path calls `EffectiveHarness`, `EffectiveInteractive`, `EffectiveInject`, `ResolveModel`, or `ResolveSkill`, and `go test ./...` passes.
- [x] 3.5 Document the pinned-definition behavior and the resume failure mode in `README.md`; verify the documented sequence still resumes a run after its source workflow is edited and names the recorded snapshot path.

## 4. Integration verification

- [x] 4.1 Add an end-to-end HTTP test using the scripted adapter: create a run that fails at a node, edit the source workflow, then resume; verify the run executes the recorded graph, `run.json` names both the source and the snapshot, and the snapshot file is present.
- [x] 4.2 Run `openspec validate --strict` and `make test`; verify both pass.

## Workflow follow-up

- Archive the change after review, syncing both delta specs into `openspec/specs/`, and land implementation, synced specs, and archive in one commit per `AGENTS.md`.
