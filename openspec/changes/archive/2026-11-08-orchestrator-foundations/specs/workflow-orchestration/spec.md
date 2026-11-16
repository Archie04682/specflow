# Spec Delta

## Purpose

Turns a workflow definition into an observable run: it decides what starts, in what order, with what inputs, and whether a finished node actually did its job — without knowing which harness executes it.

## ADDED Requirements

### Requirement: Workflow definitions are validated before a run starts

Before a run begins, the system SHALL load the workflow definition and reject it when a node identifier is duplicated, an edge names a node that does not exist, the graph contains a cycle, a node kind is unknown, an `agent` node names no harness, or a `command` node declares no command. The rejection SHALL name every defect found, not only the first.

#### Scenario: Valid workflow is accepted
- **WHEN** a run is created for a workflow whose nodes, kinds, harnesses, commands, and edges are all consistent and acyclic
- **THEN** the run is created and no validation error is reported

#### Scenario: Cycle is rejected
- **WHEN** a workflow's edges form a cycle
- **THEN** the run is rejected with an error naming the nodes in the cycle

#### Scenario: Every defect is reported together
- **WHEN** a workflow has a duplicate node identifier and an edge to a missing node
- **THEN** the rejection reports both defects in the same response

### Requirement: A run has exactly one terminal outcome

Every run SHALL end in exactly one terminal status — `completed`, `failed`, or `cancelled` — and every node SHALL end in exactly one status. When a node fails, the run SHALL fail at that node, and nodes that had not started SHALL be reported as `skipped` rather than silently absent.

#### Scenario: All nodes complete
- **WHEN** every node in the graph completes and its postcondition holds
- **THEN** the run ends `completed` and every node is reported `completed`

#### Scenario: A node fails
- **WHEN** a node fails
- **THEN** the run ends `failed`, that node is reported `failed`, and nodes that never started are reported `skipped`

### Requirement: Agent nodes run through a harness adapter

An `agent` node SHALL execute by selecting its declared harness from the registry and calling that adapter's `Run` with a prepared request and a host, and SHALL take the node's outcome from the adapter's single terminal result. The system SHALL NOT treat a stream event as the end of a node.

#### Scenario: Node outcome comes from the adapter result
- **WHEN** an `agent` node's adapter returns a completed result
- **THEN** the node is reported `completed` after the adapter returns, not when its event stream ends

#### Scenario: Unknown harness
- **WHEN** an `agent` node names a harness that is not registered
- **THEN** the node fails before any process starts, naming the unknown harness

### Requirement: Command nodes run in the run workspace

A `command` node SHALL execute its command through a shell in the node's declared working directory, pass the node's environment, enforce the node's timeout, and treat a non-zero exit as node failure. It SHALL NOT involve a harness.

#### Scenario: Command succeeds
- **WHEN** a `command` node's command exits zero
- **THEN** the node is reported `completed` with the captured output

#### Scenario: Command fails
- **WHEN** a `command` node's command exits non-zero
- **THEN** the node is reported `failed` with the exit code and captured output

#### Scenario: Command exceeds its timeout
- **WHEN** a `command` node's command runs longer than the node's timeout
- **THEN** the command is terminated and the node is reported `failed` as timed out

### Requirement: Prompt templates use a closed variable set

The system SHALL render a node's prompt by substituting only `{{objective}}`, `{{change}}`, `{{run_id}}`, and `{{workdir}}` from the run context. A placeholder outside that set SHALL fail the node before it starts, naming the unknown placeholder. No other expression syntax SHALL be evaluated.

#### Scenario: Known placeholders are substituted
- **WHEN** a node's prompt contains `{{change}}` and the run has resolved a change
- **THEN** the reference is replaced with that change before the prompt reaches the harness

#### Scenario: Unknown placeholder fails before start
- **WHEN** a node's prompt contains `{{planner.steps}}`
- **THEN** the node is not started and the failure names the unsupported placeholder

### Requirement: Model and reasoning effort resolve from node to workflow to harness default

A node's model and reasoning effort SHALL be taken from the node when set, otherwise from the workflow defaults, otherwise omitted so the harness applies its own default. An empty value and the literal `default` SHALL both mean "omit", and SHALL never be forwarded to a harness as a model identifier.

#### Scenario: Node overrides the workflow default
- **WHEN** a node sets `model.id` and the workflow sets a different default
- **THEN** the node's model is the one used for that execution

#### Scenario: Default sentinel is not forwarded
- **WHEN** a node's model is absent, empty, or the literal `default`
- **THEN** the request carries no model identifier and the harness chooses

#### Scenario: Reasoning effort follows the same chain
- **WHEN** a node sets `model.thinking` and the workflow sets a different default
- **THEN** the node's reasoning effort is applied, and an absent value is omitted

### Requirement: Skill names resolve to absolute directories before execution

When a node names a skill, the system SHALL resolve that name to one or more absolute directories and SHALL pass them with the request. When no directory matches, the system SHALL fail the node before it starts and SHALL name the skill and the locations searched.

#### Scenario: Skill resolves in the project
- **WHEN** a node names `openspec-explore` and that skill exists in the project's skill directory
- **THEN** its absolute directory is passed with the request and the prompt names the skill

#### Scenario: Missing skill fails before start
- **WHEN** a node names a skill that exists in none of the searched locations
- **THEN** the node is not started and the failure names the skill and the searched locations

### Requirement: Each node declares the directory it runs in

A node SHALL run in the run's worktree by default and MAY declare the project root instead. A command that operates on the project itself — a merge, for example — SHALL declare the project root, because the worktree has the run's branch checked out and cannot also check out the branch being merged into.

#### Scenario: Node defaults to the run worktree
- **WHEN** a node declares no working directory
- **THEN** it executes in the run's worktree

#### Scenario: Merge node runs in the project root
- **WHEN** a node declares the project root as its working directory
- **THEN** it executes there and the run worktree is unaffected

### Requirement: Harness capability preflight runs before an agent node starts

Before starting an `agent` node, the system SHALL check the node's declared requirements against the selected adapter's capabilities and SHALL fail the node, without starting it, when a requirement is unmet.

#### Scenario: Unmet requirement fails the node before start
- **WHEN** a node declares `interactive: true` and its harness reports that it cannot ask the user
- **THEN** the node is not started and the run records the refusal naming the node and the missing capability

### Requirement: Node success is a postcondition checked against the repository

A node MAY declare a postcondition, and when it does the system SHALL evaluate it against the run workspace after the node reports success and SHALL fail the node when it does not hold. A failed postcondition SHALL be reported separately from a harness failure, and the node's outcome SHALL NOT be `completed` merely because the harness exited successfully.

#### Scenario: Successful harness with an unmet postcondition
- **WHEN** an `agent` node returns completed but its `file_exists` postcondition is false
- **THEN** the node is reported failed with a postcondition failure and the run does not advance

#### Scenario: Successful harness with a met postcondition
- **WHEN** a node returns completed and its postcondition holds
- **THEN** the node is reported completed

#### Scenario: Node without a postcondition
- **WHEN** a node declares no postcondition
- **THEN** its success is its execution outcome alone

### Requirement: Change-relative postconditions measure against a recorded baseline

A postcondition that asserts a new OpenSpec change appeared SHALL compare the project's change list against a baseline recorded in the run's metadata before the asserting node started, so the check still works after a restart or a resume. When more than one new change is present, the system SHALL fail the node as ambiguous rather than choose one.

#### Scenario: One new change is resolved
- **WHEN** the change set grew by exactly one directory since the baseline
- **THEN** that change becomes the run's resolved change and the node is reported completed

#### Scenario: No new change
- **WHEN** the change set is unchanged since the baseline
- **THEN** the node is reported failed with a postcondition failure

#### Scenario: Ambiguous change set
- **WHEN** more than one change directory appeared since the baseline
- **THEN** the node is reported failed as ambiguous and lists the candidates

### Requirement: Retrying a node offers a forward attempt or a clean attempt

A failed node SHALL be retryable either forward — re-executed on the current workspace with the previous failure included in the prompt — or cleanly, from the node's starting checkpoint. An adapter failure SHALL be retried automatically only when the adapter classified it as retryable and only up to the node's declared limit; a postcondition failure SHALL never be retried automatically and SHALL wait for the user's choice.

#### Scenario: Automatic retry for a retryable failure
- **WHEN** a node fails with a retryable adapter failure and has attempts remaining
- **THEN** the node is retried with the failure text included in the prompt

#### Scenario: Non-retryable failure is not retried
- **WHEN** a node fails with a non-retryable classification such as a missing runtime
- **THEN** no automatic retry occurs and the run reports the failure

#### Scenario: Postcondition failure waits for the user
- **WHEN** a node fails its postcondition
- **THEN** the run stops at that node and offers a forward retry, a clean retry, or an abort

#### Scenario: Clean retry restores the starting state
- **WHEN** the user chooses a clean retry
- **THEN** the workspace is restored to the node's starting checkpoint before the node runs again

### Requirement: A failed run resumes at the failed node

A run that failed at a node SHALL be resumable at that node without re-executing any node that already completed. Resuming SHALL restore the run's context, including the recorded change and checkpoints, from its persisted metadata.

#### Scenario: Resume skips completed nodes
- **WHEN** a run that failed at its third node is resumed
- **THEN** the first two nodes are not re-executed and execution continues at the third

#### Scenario: Resume preserves the resolved change
- **WHEN** a run resumes after its change was resolved
- **THEN** subsequent nodes receive the same change without re-deriving it

### Requirement: Approval gates pause a run before a node

A node MAY require approval. Before such a node starts, the run SHALL enter a waiting status that identifies the node and the action it would take, and SHALL NOT execute it until an approval arrives. A rejection SHALL leave the node unstarted and end the run as cancelled.

#### Scenario: Approval lets the node run
- **WHEN** a node requires approval and the user approves it
- **THEN** the node executes and the run continues

#### Scenario: Rejection ends the run
- **WHEN** a node requires approval and the user rejects it
- **THEN** the node does not execute, it is reported skipped, and the run ends cancelled

### Requirement: Harness questions are routed to the user and hold the run

When a running node's harness asks a question, the system SHALL report the question as a run event identifying the node and SHALL hold that node until an answer, a cancellation, or the deadline resolves it. The answer SHALL be delivered to the execution that asked, and an answer arriving after resolution SHALL be discarded.

#### Scenario: Question is surfaced and answered
- **WHEN** a harness asks a question during a node
- **THEN** the run reports the question with the node identifier and continues after the answer is delivered

#### Scenario: Unanswered question reaches its deadline
- **WHEN** no answer arrives before the question's deadline
- **THEN** the question is cancelled, the harness is told so, and the node proceeds or fails by the harness's own response

### Requirement: Clarifications are routed into the running node

The system SHALL accept a clarification for a run only while a node is running, only when that node allows injection, and SHALL deliver it through the execution's injection channel. It SHALL reject the request with a reason when no node is running or the node or harness does not allow injection.

#### Scenario: Clarification reaches the running node
- **WHEN** the user submits a clarification while a node that allows injection is running on a harness that supports it
- **THEN** the message is delivered to that execution and its disposition is recorded

#### Scenario: No running node
- **WHEN** the user submits a clarification for a run that is not running
- **THEN** the request is rejected with a reason and no execution is affected

### Requirement: Run events are sequenced and replayable

Every run SHALL assign a monotonically increasing sequence number to each event it emits and SHALL retain events so a client that reconnects can resume from the last event it received. A reconnect that supplies a sequence number SHALL receive every event after it and no duplicates, and an event stream SHALL report events in sequence order.

#### Scenario: Late subscriber catches up
- **WHEN** a client subscribes to a running run and supplies the last sequence number it saw
- **THEN** it receives every later event in order and no earlier one

#### Scenario: Sequence numbers are strictly increasing
- **WHEN** events are emitted for a run
- **THEN** each event's sequence number is greater than the previous one

### Requirement: Cancelling a run stops the running node

Cancelling a run SHALL cancel the execution of the node that is currently running, SHALL wait for it to stop within a bounded period, and SHALL end the run as cancelled. Nodes that had not started SHALL be reported skipped.

#### Scenario: Cancel while a node runs
- **WHEN** the user cancels a run whose node is executing
- **THEN** the node's execution is cancelled, the node is reported cancelled, and the run ends cancelled

#### Scenario: Cancel leaves no execution running
- **WHEN** a run is cancelled and the operation returns
- **THEN** no node execution from that run is still running
