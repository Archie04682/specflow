# Spec Delta

## ADDED Requirements

### Requirement: A command node can complete under a route label

A `command` node MAY declare `routes`, mapping exit codes to route labels. When its command exits with a declared code the node SHALL complete under that label and the run record SHALL carry it. An exit code the node does not declare SHALL fail the node, and a timeout or a cancellation SHALL never select a route.

#### Scenario: A declared exit code routes instead of failing

- **WHEN** a `command` node declaring `routes: {1: issues}` exits with 1
- **THEN** the node is reported completed, its record carries the label `issues`, and the run continues

#### Scenario: An undeclared exit code is still a failure

- **WHEN** a `command` node declaring `routes: {1: issues}` exits with 2
- **THEN** the node is reported failed with the exit code and captured output

#### Scenario: A zero exit can be a route

- **WHEN** a `command` node declares `routes: {0: clean}` and exits 0
- **THEN** the node completes under `clean` and the run continues

#### Scenario: A timed-out route command does not route

- **WHEN** a `command` node that declares routes runs past its timeout
- **THEN** the node is reported failed as timed out and no route label is recorded

#### Scenario: The route is reported to the event stream

- **WHEN** a `command` node completes under a route label
- **THEN** its `node.completed` event carries that label

### Requirement: An edge may be conditional on a route label

An edge MAY declare `when`, naming a route label its source node declares. A node that declares routes SHALL take exactly the outgoing edge whose `when` matches the label it completed under, and no other. A node that declares no routes SHALL take every outgoing edge, as it does today.

#### Scenario: A gate takes one branch

- **WHEN** a gate completes under `issues` and has outgoing edges labelled `issues` and `clean`
- **THEN** the edge labelled `issues` is taken and the edge labelled `clean` is not

#### Scenario: A node without routes takes every edge

- **WHEN** a node that declares no routes has two outgoing edges
- **THEN** both edges are taken

### Requirement: A node runs when a taken edge reaches it

The system SHALL resolve a node only after every predecessor has resolved. A node with at least one incoming edge taken SHALL run; a node with none SHALL be reported `skipped` and SHALL take none of its own edges. Among nodes that are ready, the system SHALL run the one earliest in the compiled order, one node at a time.

#### Scenario: The branch not taken is skipped

- **WHEN** the review gate completes under `clean`
- **THEN** the `fix` node is reported skipped and the run continues

#### Scenario: A join runs when either branch reaches it

- **WHEN** a node has incoming edges from `fix` and from the review gate, and the gate took `clean`
- **THEN** `fix` is skipped first and the join node then runs

#### Scenario: A skip propagates

- **WHEN** a skipped node has an outgoing edge
- **THEN** that edge is not taken and its target resolves by the same rule

#### Scenario: A workflow without routes keeps its order

- **WHEN** a workflow declares no routes at all
- **THEN** every node runs in the same order it ran in before routing existed

### Requirement: Routing declarations are validated before a run starts

The system SHALL reject a workflow that declares `routes` on a node that is not a `command` node or together with a `postcondition`, an outgoing edge of a `routes` node with no `when`, a `when` naming a label its source does not declare, two outgoing edges using one label, a declared label no outgoing edge uses, or a `when` whose source declares no routes. Every defect SHALL be reported with the others.

#### Scenario: A conditional edge must name a declared label

- **WHEN** an edge declares `when: issues` and its source node declares only the route `clean`
- **THEN** run creation is refused and the defect names the edge and the unknown label

#### Scenario: An unused label is refused

- **WHEN** a gate declares the route `major` and no outgoing edge uses it
- **THEN** run creation is refused and names the unused label

#### Scenario: A gate cannot also carry a postcondition

- **WHEN** a `command` node declares both `routes` and a `postcondition`
- **THEN** run creation is refused and names the node

#### Scenario: A route is refused on a kind that cannot select

- **WHEN** an `agent` node declares `routes`
- **THEN** run creation is refused and names the node and its kind

#### Scenario: Routing defects join the other defects

- **WHEN** a workflow has an unknown node kind and an unlabelled edge out of a gate
- **THEN** both defects are reported in the same response

### Requirement: Resuming a routing workflow re-derives the taken path

A resumed run SHALL derive which edges were taken from the route labels recorded on its completed nodes, and SHALL report a node `skipped` only while no taken edge reaches it. A node skipped because the run failed SHALL be re-run when a taken edge reaches it, and a node skipped because its branch was not taken SHALL stay skipped.

#### Scenario: An off-path node stays skipped across a resume

- **WHEN** a run failed after its gate took `clean`, and the run is then resumed
- **THEN** the `fix` node stays skipped and is not executed

#### Scenario: A failure-skipped node is re-run

- **WHEN** a run failed at the gate and a node after the gate was reported skipped
- **THEN** resuming the run executes that node once the gate completes

#### Scenario: Resume needs no extra frontier

- **WHEN** a routing run is resumed
- **THEN** the taken path is reconstructed from the recorded labels alone

## MODIFIED Requirements

### Requirement: Command nodes run in the run workspace

A `command` node SHALL run its command through a shell in the node's declared working directory with the node's environment and timeout, and SHALL treat a non-zero exit as failure unless the node declares that code as a route. It SHALL NOT involve a harness. It SHALL record the rendered command and publish `command.started` before execution and `command.finished` with the exit code, captured output, and duration after every outcome, including failure, timeout, and cancellation.

#### Scenario: Command succeeds

- **WHEN** a `command` node's command exits zero
- **THEN** the node is reported `completed` with the captured output

#### Scenario: Command fails

- **WHEN** a `command` node's command exits non-zero
- **THEN** the node is reported `failed` with the exit code and captured output

#### Scenario: Command exceeds its timeout

- **WHEN** a `command` node's command runs longer than the node's timeout
- **THEN** the command is terminated and the node is reported `failed` as timed out

#### Scenario: Command start names what runs

- **WHEN** a `command` node begins
- **THEN** its `command.started` event carries the rendered command and the working directory it runs in

#### Scenario: Command finish carries the result

- **WHEN** the command exits
- **THEN** its `command.finished` event carries the exit code, the captured output, and the duration

#### Scenario: A timed-out command still publishes its finish

- **WHEN** the command exceeds its timeout
- **THEN** `command.finished` reports the termination before the node is reported failed

#### Scenario: The run record keeps the command

- **WHEN** the run record is read after the attempt
- **THEN** the node carries the rendered command that ran

### Requirement: A run has exactly one terminal outcome

Every run SHALL end in exactly one terminal status — `completed`, `failed`, or `cancelled` — and every node SHALL end in exactly one status. A run SHALL end `completed` when every node has ended `completed` or `skipped`. When a node fails, the run SHALL fail at that node, and nodes that had not started SHALL be reported as `skipped` rather than silently absent.

#### Scenario: All nodes complete

- **WHEN** every node in the graph completes and its postcondition holds
- **THEN** the run ends `completed` and every node is reported `completed`

#### Scenario: A run with a skipped branch completes

- **WHEN** every node has ended `completed` or `skipped`
- **THEN** the run ends `completed`

#### Scenario: A node fails

- **WHEN** a node fails
- **THEN** the run ends `failed`, that node is reported `failed`, and nodes that never started are reported `skipped`

### Requirement: A run's graph is read from the definition it was pinned to

The system SHALL expose `GET /api/runs/{id}/graph?project=` returning the run's authored nodes and edges from the workflow definition retained for that run, falling back to the recorded workflow path for a run created before snapshots. Each node SHALL carry its identifier, title, kind, and effective interactive, injection, and approval flags; each edge SHALL name its source and target, and the condition it declares when it has one.

#### Scenario: The pinned definition wins over an edited file

- **WHEN** the project's workflow file is edited after the run is created
- **THEN** the graph reflects the definition retained with the run

#### Scenario: A legacy run falls back to its recorded path

- **WHEN** a run was created before retained snapshots existed
- **THEN** its recorded workflow path supplies the graph

#### Scenario: An edge carries its condition

- **WHEN** the pinned definition declares a conditional edge
- **THEN** that edge carries its `when` label in the graph response

#### Scenario: An unknown run is not found

- **WHEN** the run identifier is unknown to the project
- **THEN** the response is `404` with a JSON error body
