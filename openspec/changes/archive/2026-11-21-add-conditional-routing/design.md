# Design

## Context

See `proposal.md` for motivation and `specs/` for the requirements. This section
records only the current mechanics the approach has to fit.

**Sequencing.** `Engine.runFrom` (`engine.go`) walks `plan.Order`, a flattened
topological order computed once by `Workflow.Order()` (`workflow.go`). Every node
in that slice executes; an edge only contributes to the sort. `Workflow.Order`
returns an error for a cycle, which is how `ValidateWorkflow` rejects one.

**The plan carries no edges.** `WorkflowPlan` holds `Nodes` and `Order`. The
engine never sees `Workflow.Edges` after compilation, so "which edges leave this
node" is not currently answerable at execution time.

**Node success.** A command node fails on a non-zero exit (`runCommand`). An
agent node's success is its adapter result plus its postcondition
(`attemptNode` → `EvaluatePostcondition`). A failure ended the run after every
node that had not started was reported skipped.

**Resume.** `Start` used to launch the walk at the first node, and `runFrom`
skipped nodes already `NodeCompleted`. "Resume at the failed node" was a
consequence of that walk, not of a stored cursor. A node skipped by a failure is
therefore re-run on resume, because it is not `completed`.

**The run record holds one `NodeRun` per node identifier**, indexed by
`Node.ID`, with no notion of a visit.

**Constraints.** One node at a time. Edges carry ordering, not data. Prompt
variables are the closed set `objective`, `change`, `run_id`, `workdir`. The
harness adapter contract must not change. The graph layout is topology-only and
must not move a node when execution state changes.

## Goals / Non-Goals

**Goals:**

- Let a workflow author express "after review, if there were findings, run fix;
  otherwise go straight to verify" as an authored, visible part of the graph.
- Keep the verdict a repository fact produced by a deterministic gate, so it is
  auditable, replayable, and never a model's self-report.
- Preserve the existing execution order for every workflow that declares no
  routing, so the change lands without touching what already works.
- Keep the persisted state minimal: one new field, no frontier.

**Non-Goals:**

- Back-edges, revisited nodes, or run-time loops. `Workflow.Order` keeps
  rejecting cycles.
- Concurrent branches. The scheduler picks one node at a time.
- Any routing signal from a harness. `harness.Adapter` and `harness.Result` are
  untouched.
- A general expression language on edges. A condition is a label equality, not
  a predicate.

## Decisions

### 1. The verdict is a command node's exit code

A `routes` map on a `command` node turns selected exit codes into route labels;
every other exit code keeps failing the node.

**Alternatives considered.**

- *Route on whether a postcondition held.* Reuses `EvaluatePostcondition` and
  needs no new node, but it forces a node to end `failed` while the run
  continues, which contradicts "a node's outcome SHALL NOT be `completed` merely
  because the harness exited successfully" and renders a red node inside a green
  run. It also has no natural expression for "review found nothing" — the
  verdict would be an inverted `file_absent`.
- *Add `Result.Outcome` to the adapter contract.* Highest fidelity to "the agent
  decides", but it changes the contract every adapter implements, needs a new
  Pi bridge tool, and makes the branch depend on model compliance rather than a
  check. It also breaks `AGENTS.md`'s rule that adding an adapter must not change
  the orchestrator.
- *Let a prompt report findings and have a human decide.* The status quo; not
  resumable or auditable.

A gate costs one extra node per decision. In exchange the branch is deterministic,
the node status never lies, and a crash (exit 2 from a `test` builtin, a shell
error) can never be mistaken for a branch.

### 2. Two fields: `routes` on the node, `when` on the edge

```yaml
- id: review_gate
  kind: command
  command: test ! -s .specflow/review.md
  routes: {0: clean, 1: issues}
edges:
  - {from: review_gate, to: fix,    when: issues}
  - {from: review_gate, to: verify, when: clean}
```

The node declares the vocabulary; the edge selects from it. Two independent
places to validate, and neither is executable content.

**Alternatives considered.**

- *A guard command on the edge itself* (`when: "grep -q ..."`). Fewer nodes, but
  it puts executable content on an edge and gives an edge its own process
  lifecycle, exit code, output, timeout, and failure mode — a second kind of
  node, wearing an edge's clothes.
- *A `switch` block listing `{label, target}` pairs on the node.* One place to
  look, but it duplicates topology in node configuration, defeating the graph
  and the editor's edge model.

The name `routes` was chosen over `select` and `on_exit` because it reads as
"the ways this node can end", and it pairs with the edge's `when`.

### 3. Readiness resolution replaces the linear walk; `Order` becomes a tie-break

```
resolved[N]  -- node has ended (completed / skipped / failed)
arrived[N]   -- at least one incoming edge was taken   (roots: true)
pending[N]   -- unresolved predecessor count           (roots: 0)

loop until every node is resolved:
    ready = unresolved nodes with pending == 0
    N = the one earliest in plan.Order
    if !arrived[N]:  mark skipped;      take no outgoing edge
    else:            execute N;         take outgoing edges by rule (below)
    for each taken edge N->M: arrived[M] = true
    for each edge N->M:       pending[M]--
```

Taking edges: a node whose record carries a route label takes only the outgoing
edge whose `when` equals it; every other node takes all of its outgoing edges.

The resolution always terminates: `pending` only decreases, and a DAG has a
node with no unresolved predecessors until every node is resolved.

`plan.Order` stops being the execution plan and becomes the deterministic
tie-break among ready nodes. That is what preserves today's behavior: a graph
with no `routes` takes every edge, so every node arrives, and the selected node
is always the next one in the existing topological order. Existing workflows and
existing tests therefore execute in exactly the same order.

**Alternatives considered.**

- *Breadth-first walk over taken edges.* Equivalent, but it produces dependency
  order rather than declaration order among independent nodes, which would
  reorder today's runs.
- *Unrolling loops at compile time.* Irrelevant here — no loops — and it would
  make node identifiers differ from the authored ones.

### 4. A node resolves when all predecessors resolve; it runs when any edge arrives

The join is AND on resolution and OR on arrival. A node reached by either side
of a fork has exactly one taken edge, so an AND-join on arrival would deadlock
on mutually exclusive branches; a first-arrival execution would race. Resolving
only after every predecessor is resolved keeps node order deterministic while
still allowing either branch to satisfy the join.

### 5. The plan carries the edges

`WorkflowPlan` gains `Edges []PlanEdge{From, To, When}`, and `PlanNode` gains
`Routes map[int]string`. Compilation is where the authored definition becomes
executable, and the engine must never re-read the workflow file: a resumed run
executes the retained snapshot, and `AGENTS.md` requires the engine to execute
the plan and nothing else.

### 6. `NodeRun.Outcome` is the only new persisted datum

A completed routing node records the label it completed under. Everything else
about the taken path is derived: a completed node's edges follow from its label
(or from having none), and every other node is recomputed.

This makes resume a *fresh scheduling pass with the completed nodes pinned*. No
stored frontier can drift from the graph, and the existing "a node skipped by a
failure is re-run, a completed node is not" behavior falls out of the same rule
instead of being special-cased.

A completed node that declares `routes` but carries no recorded label is a
corrupt record, and the resume fails naming that node rather than guessing.

### 7. `skipped` is reused; no new node status

Adding a `bypassed` status would ripple through the status set, the dashboard,
the projection reducer, and a spec that declares an exhaustive list. A skipped
node already means "did not run", and the run's status distinguishes "not
needed" from "not reached".

### 8. The findings file is a workflow convention, not an engine concept

The engine learns `routes` and `when`, nothing about reviews. The canonical
template and `docs/workflows.md` establish the convention: review writes
`.specflow/review.md`, the gate tests for it, and fix deletes it when done.

The path is relative to the node's working directory, so the file lives inside
the run's worktree at `.specflow/worktrees/<id>/.specflow/review.md` — ignored by
git at any depth because `.gitignore` names `.specflow/` without a leading
slash, but a *different* `.specflow/` from the project's run journals. That needs
saying in the docs, or someone will look in the wrong directory.

Using the repository rather than a payload is what keeps prompt variables closed
and edges data-free: fix reads the findings from the working tree because that is
where the work is.

### 9. The canonical template gets a real gate node

`workflows/standard-feature.yaml` gains `review` writing findings, a
`review_gate`, a `fix` node, and the two conditioned edges plus `fix -> verify`.
The alternative — leaving the template alone — would ship a capability with no
worked example and leave the review step exactly as undecidable as it is now.
Runs already created keep their pinned snapshot; only new runs take the new path.

## Risks / Trade-offs

- **[The verdict is only as good as the agent's compliance]** → The gate is
  deterministic, but "review found issues" is a claim the agent makes by writing
  a file. A review that finds defects and writes nothing routes `clean` and
  skips the fix. Mitigation: the template prompt states the file contract
  explicitly; the review node's prose stays in the event stream for the
  operator; and `verify` still runs on the `clean` path, so real breakage is
  caught even when the branch is wrong.
- **[A mis-inverted gate silently takes the wrong branch]** → No validator can
  know whether exit 0 should mean `clean`. Mitigation: the label is recorded on
  the node, so a wrong branch is visible in the run record and the graph, and
  the gate's command and output are recorded like any command node's.
- **[Skipped looks the same whether it was not needed or not reached]** → The
  graph marks an untaken branch as not run, and the run's status says which case
  applies. Accepted: a third status is not worth the cross-cutting cost.
- **[Downgrade]** → An older binary reading a routed template ignores `routes`
  and `when` (YAML is unmarshalled leniently) and runs both branches. Runs stay
  compatible in the other direction because run records gain one additive JSON
  field. Mitigation: routing is a new capability, so a downgrade loses it
  visibly rather than corrupting a run.
- **[Long condition labels]** → An edge label is author text. Mitigation: render
  it truncated on the edge with the full label in the edge form and the edge's
  tooltip; the layout itself ignores labels, so a long one cannot move a node.

## Migration Plan

1. Land the model, validation, plan, and engine changes with no template change.
   Every existing workflow declares no `routes`, so behavior is unchanged; the
   existing engine and compile tests are the regression gate.
2. Land the draft, graph, and editor changes; the new fields are optional in
   both directions, so a workflow written by the new editor opens in an old one
   minus the routing.
3. Update `workflows/standard-feature.yaml` and `docs/workflows.md` last, so the
   template never references a field the running server cannot honor.
4. Rollback is a revert: no on-disk format migration is required, no run record
   needs rewriting, and pinned snapshots remain readable.

## Open Questions

- Whether a future change should let a *gate* iterate rather than route forward
  (a bounded visit count) — deliberately excluded here; a back-edge touches the
  run record, resume, and layout far more than this design does.
- Whether the editor should offer a one-click "add a review branch" template
  action. Useful, but it is authoring sugar over fields that will exist, and it
  does not change this design.
