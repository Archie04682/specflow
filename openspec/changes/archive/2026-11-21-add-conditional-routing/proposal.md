# Proposal

## Why

A workflow's edges carry ordering only, so every node in the graph always runs.
A workflow cannot say "if review found defects, fix them; otherwise go straight
to verify". The one place the canonical template needs a decision — after review
— is currently expressed by a prompt asking a model to report defects and by a
human deciding what to do next. That is unauditable, unresumable, and invisible
in the graph the operator drew.

The graph model already forks: validation accepts a diamond today, and the
engine simply runs both sides of it. What is missing is a way to say that an
edge was not taken.

## What Changes

- A `command` node MAY declare `routes`, a mapping from exit code to a route
  label. When its command exits with a declared code, the node completes under
  that label instead of failing. An undeclared code, a timeout, or a
  cancellation still fails the node, so a crash can never masquerade as a
  branch.
- An edge MAY declare `when`, naming a route label of its source node. A `routes`
  node takes exactly the edge matching the label it completed under; every other
  node keeps taking all of its edges.
- The engine stops walking a flattened topological order and instead resolves a
  node when its predecessors have resolved: a node with at least one incoming
  edge taken runs, and a node with none is reported `skipped`. The topological
  order is retained as the deterministic tie-break between ready nodes, so a
  workflow with no `routes` executes in exactly the order it does today.
- A node's selected route label is persisted on the run record. A resumed run
  re-derives which edges were taken from the completed nodes' labels, so no
  separate frontier has to be stored, and a node skipped because the run failed
  is still re-run while a node skipped as off-path stays skipped.
- The compiled plan carries the workflow's edges; it currently carries only
  nodes and order.
- Validation rejects the routing contradictions: `routes` on a non-`command`
  node, `routes` together with a `postcondition`, an outgoing edge of a `routes`
  node without a `when`, a `when` naming a label its source does not declare,
  and a declared label no edge uses.
- The workflow draft, the run graph projection, and the flow editor carry
  `routes` and `when`, so the branch is authored and rendered instead of being
  written by hand.
- The canonical template gains the flow this change exists for: review writes
  its findings to a file, a gate routes on whether that file exists, and a fix
  node runs only when the gate says there are findings.
- `docs/workflows.md` gains the new fields, the join rule, and the worktree-local
  location of the findings file.

Explicitly out of scope:

- **Back-edges and revisited nodes.** Cycles stay rejected, a node still runs at
  most once per run, and `fix` therefore flows forward to verify rather than
  back to review.
- **Concurrent branches.** The engine still runs one node at a time.
- **Agent-declared outcomes.** The harness adapter contract does not change; an
  agent node cannot route, only a command node can.

## Capabilities

### New Capabilities

None. Conditional routing is scheduling behavior, and `workflow-orchestration`
already owns "what starts, in what order". A separate capability would describe
the same engine from a second spec file.

### Modified Capabilities

- `workflow-orchestration`: validation and compilation accept routing fields;
  run sequencing becomes a readiness resolution that skips untaken branches; a
  command node can complete under a route label; node and run records carry the
  selected label and the resulting skips; the run graph projection carries edge
  conditions; resume re-derives the taken path.
- `workflow-editor`: the draft, the structural validation, and the inspector
  carry `routes` on command nodes and `when` on edges, and the canvas shows an
  edge's condition.
- `run-dashboard`: the graph renders an edge's condition and distinguishes a
  branch that was taken from one that was skipped, without deriving topology
  from the node records.

## Impact

- **Engine**: `backend/internal/orchestrator` — `workflow.go` (new fields),
  `validate.go` (new rules), `plan.go` and `compile.go` (edges in the plan),
  `engine.go` (readiness resolution, route selection, skip propagation),
  `run.go` (`NodeRun` route label), `graph.go` (edge condition),
  `workflow_draft.go` and `workflow_source.go` (draft round-trip).
- **Frontend**: `frontend/src/types`, `frontend/src/components/WorkflowEditor`,
  `frontend/src/components/RunDashboard/GraphPanel`, and the graph layout and
  editor model tests. Layout stays topology-only; a condition label and a taken
  flag must not move a node.
- **Workflow content**: `workflows/standard-feature.yaml` gains the review gate
  and the fix node. Runs already created keep their pinned definition; new runs
  of the template take the new path.
- **Docs**: `docs/workflows.md`.
- **Compatibility**: the new YAML fields are optional and the new JSON fields
  are additive, so existing workflow files, pinned run snapshots, and run
  records keep working. A workflow with no `routes` takes every edge and runs
  every node in its existing order.
