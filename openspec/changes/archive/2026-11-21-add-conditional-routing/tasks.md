# Tasks

## 1. Workflow model and validation

- [x] 1.1 Add `Routes map[int]string` to `Node` and `When string` to `Edge` in `backend/internal/orchestrator/workflow.go`, with YAML tags `routes` and `when`. Verify `TestLoadWorkflow` covers a command node with an integer-keyed `routes` map and an edge with `when`, and that a workflow without either still parses.
- [x] 1.2 Add the seven routing defects to `ValidateWorkflow` in `backend/internal/orchestrator/validate.go`: `routes` on a non-`command` kind, `routes` with a `postcondition`, an outgoing edge of a `routes` node with no `when`, a `when` naming an undeclared label, one label on two outgoing edges, a declared label no edge uses, and a `when` whose source declares no routes. Verify with table-driven tests beside the existing `TestValidateWorkflow` in `backend/internal/orchestrator/workflow_test.go`: one case per defect, plus one workflow carrying a routing defect and an unrelated defect whose assertion shows both problems in the same `ValidationError`.
- [x] 1.3 Carry `routes` and `when` through the editor draft in `backend/internal/orchestrator/workflow_draft.go` (`WorkflowNodeDraft`, `WorkflowEdgeDraft`, both conversion directions) and confirm the canonical YAML writer emits them. Verify a round-trip test: workflow → draft → workflow keeps both fields, and a saved draft's generated YAML re-parses into the same values.

## 2. Compiled plan and read projections

- [x] 2.1 Add `Routes map[int]string` to `PlanNode` and a `PlanEdge{From, To, When}` slice to `WorkflowPlan` in `backend/internal/orchestrator/plan.go`, and populate both in `compile.go`. Verify `compile_test.go` asserts that a routed workflow's plan carries its edges, their conditions, and each node's routes, and that `TestCompileWorkflowCanonicalTemplate` keeps passing on the unchanged template.
- [x] 2.2 Add `When string` to `GraphEdge` in `backend/internal/orchestrator/graph.go` and populate it in `graphEdges`. Verify a graph test reads a conditioned edge from a retained snapshot and asserts its source, target, and condition, with no compile, skill resolution, or event emitted.
- [x] 2.3 Add `Outcome string` to `NodeRun` in `backend/internal/orchestrator/run.go`, omitted from JSON when empty. Verify `run_test.go` asserts it round-trips and that a run record without it still unmarshals.

## 3. Readiness scheduler

- [x] 3.1 Replace the linear walk in `Engine.runFrom` (`engine.go`) with the readiness resolution from `design.md`: resolve a node once every predecessor has resolved, run the ready node earliest in `plan.Order`, and keep `plan.Order` out of the execution decision otherwise. Verify the existing `backend/internal/orchestrator/engine_test.go` and `engine_compile_test.go` suites pass without edits — that is the compatibility gate — and add one test asserting an unrouted diamond runs in the same order it does today.
- [x] 3.2 Let a `command` node complete under a route label: pass the node's routes into `runCommand`, return the selected label instead of an error for a declared code, carry it through `attemptNode` into `NodeRun.Outcome`, and emit it on `node.completed`. Verify engine tests for a declared code (completes with the label), an undeclared code (fails with exit code and output), a zero exit declared as a route (completes), and a timeout (fails, no label).
- [x] 3.3 Take edges by rule: a node whose record carries a label takes only the outgoing edge with that `when`; every other node takes all outgoing edges. Verify a run test where the gate completes under one label and the other branch's node never starts.
- [x] 3.4 Skip an untaken branch and keep the join: report a node with no taken incoming edge `skipped`, take none of its outgoing edges, and still run a join node once either side arrives. Verify a diamond test asserting the skipped branch, its propagated skip, and the join running exactly once.
- [x] 3.5 On failure, mark every unresolved node `skipped` rather than the tail of an order slice, and end the run failed. Verify the existing failure tests still pass and add a routed case where the failing node is followed by an untaken branch.

## 4. Resume

- [x] 4.1 Make resume a fresh scheduling pass with completed nodes pinned: derive each completed node's taken edges from its recorded `Outcome` (or from having no routes), recompute every other node, and keep `Start` launching the same pass. Verify resume tests for an off-path node that stays `skipped` and a failure-skipped node that is re-run once a taken edge reaches it.
- [x] 4.2 Fail a resume whose completed `routes` node carries no recorded label, naming that node, instead of guessing a branch. Verify the test asserts the run fails before any node executes and the error names the node.

## 5. Editor authoring

- [x] 5.1 Add `routes` to the node draft and `when` to the edge draft in `frontend/src/types/workflowDraft.ts`, plus setters in `frontend/src/editor/model.ts`: a route add/remove, an edge condition set, dropping a node's routes when its kind stops being `command`, and clearing an edge's condition when it is rewired to a source that does not declare that label. Verify `frontend/src/editor/model.test.ts` covers each setter and both drop rules.
- [x] 5.2 Add a routes editor to the node inspector, shown only for a `command` node, letting the operator add, change, and remove exit-code/label pairs. Verify `NodeInspector.test.tsx` asserts the editor appears for a command node, is absent for an agent node, and that an edit reaches the draft.
- [x] 5.3 Add a condition picker to the edge form offering only the source node's declared labels, and draw the chosen label on the edge without moving any node. Verify `WorkflowEditor.test.tsx` asserts the offered labels, that an edge with a non-routing source offers no condition, that the canvas shows the label, and that selecting and unselecting the label does not change any node position.
- [x] 5.4 Save and reopen a routed draft through the typed API client and surface a routing `400` as problems. Verify `frontend/src/api/workflowEditor.test.ts` asserts the round-trip and the refusal path.

## 6. Run dashboard rendering

- [x] 6.1 Mark each graph edge as taken or not taken from the run record's per-node `Outcome` and the graph's edge conditions, keeping topology and positions untouched. Verify a new `frontend/src/components/RunDashboard/GraphPanel/GraphPanel.test.tsx` asserts that a gate's untaken edge reads as not taken while the other reads as taken, that a skipped branch reads as not run, and that layout positions are identical before and after the marking.

## 7. Canonical template and documentation

- [x] 7.1 Add the review convention, the `review_gate`, and the `fix` node to `workflows/standard-feature.yaml`, with `review -> review_gate`, `review_gate -> fix (when: issues)`, `review_gate -> verify (when: clean)`, and `fix -> verify`. Verify `ValidateWorkflow` and `CompileWorkflow` accept it.
- [x] 7.2 Update `backend/internal/orchestrator/workflow_test.go` and `backend/internal/orchestrator/compile_test.go` for the template's new node count and new gate, and assert the gate's `routes` and both edges' `when` survive compilation.
- [x] 7.3 Add an integration test in `backend/internal/api/integration_test.go` that runs the canonical template twice: once with a mock that writes no findings, asserting the gate routes `clean` and `fix` is reported skipped, and once with a mock that writes a findings file on `review`, asserting `fix` runs. Verify both runs end `completed`.
- [x] 7.4 Document `routes`, `when`, the resolve/arrive rule, and the worktree-local findings path in `docs/workflows.md`, including that the findings file lives in the run worktree rather than the project's `.specflow/`. Verify every YAML example in the added section validates and every command shown runs as written.
- [x] 7.5 Add an Orchestrator bullet to `AGENTS.md` stating that an edge may be conditional on a route label and that a node runs when a taken edge reaches it, keeping the existing "edges carry ordering, not data" decision intact.

## 8. Integration checks

- [x] 8.1 Run `make test` and confirm the Go suite, the frontend typecheck, and the frontend tests pass.
- [x] 8.2 Run `openspec validate add-conditional-routing --strict` and `openspec validate --changes --strict` and confirm both are clean.

## Workflow follow-up

- Archive the change after the gates above pass, in the same commit as the implementation and the synced specs.
