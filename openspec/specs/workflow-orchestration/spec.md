# workflow-orchestration Specification

## Purpose

Turns a workflow definition into an observable run: it decides what starts, in what order, with what inputs, and whether a finished node actually did its job — without knowing which harness executes it.

## Requirements

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

### Requirement: Workflow definitions are compiled before a run is created

Before a run is created, the system SHALL compile the validated workflow into an executable plan. Compilation SHALL resolve every node's harness, model and reasoning effort, interactive and injection permissions, working-directory mode, timeout, absolute skill directories, and order. An omitted model identifier and the literal `default` SHALL stay omitted. Compilation SHALL fail run creation when a value cannot be resolved, naming every defect, and SHALL NOT validate model identifiers.

#### Scenario: Effective values are resolved once
- **WHEN** a node omits its harness and model while the workflow defaults set both
- **THEN** the plan carries the workflow's harness and model, and execution uses those values without consulting the defaults again

#### Scenario: Working directory mode is resolved
- **WHEN** a node declares no working directory
- **THEN** the plan records that it runs in the run workspace

#### Scenario: Default sentinel is not carried
- **WHEN** a node's model is the literal `default`
- **THEN** the plan carries no model identifier and the harness chooses its own model

#### Scenario: Resolution defects are reported together
- **WHEN** two nodes name skills that exist in none of the searched locations
- **THEN** run creation fails and the error names both nodes, their skills, and the searched locations

#### Scenario: Compilation failure has no run side effects
- **WHEN** compilation fails
- **THEN** no worktree, branch, or run record is created

#### Scenario: Unknown model is not a compile error
- **WHEN** a node names a model identifier that its harness does not support
- **THEN** compilation succeeds and the failure is reported when the node executes

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

A `command` node SHALL run its command through a shell in the node's declared working directory with the node's environment and timeout, and SHALL treat a non-zero exit as node failure. It SHALL NOT involve a harness. It SHALL record the rendered command and publish `command.started` before execution and `command.finished` with the exit code, captured output, and duration after every outcome, including failure, timeout, and cancellation.

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

When a node names a skill, compilation SHALL resolve that name to one or more absolute directories and SHALL carry them in the plan. When no directory matches, the system SHALL fail run creation and SHALL name the skill and the locations searched.

#### Scenario: Skill resolves in the project
- **WHEN** a node names `openspec-explore` and that skill exists in the project's skill directory
- **THEN** the plan carries its absolute directory and the node's request names the skill

#### Scenario: Missing skill fails before start
- **WHEN** a node names a skill that exists in none of the searched locations
- **THEN** the run is not created and the failure names the skill and the searched locations

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

A run that failed at a node SHALL be resumable at that node without re-executing any node that already completed. Before re-executing, resuming SHALL compile the workflow definition recorded with the run — not the file at its original path — and SHALL fail without executing a node when it no longer compiles. It SHALL restore the run's context, including the recorded change and checkpoints, from its persisted metadata.

#### Scenario: Resume skips completed nodes
- **WHEN** a run that failed at its third node is resumed
- **THEN** the first two nodes are not re-executed and execution continues at the third

#### Scenario: Resume preserves the resolved change
- **WHEN** a run resumes after its change was resolved
- **THEN** subsequent nodes receive the same change without re-deriving it

#### Scenario: Editing the workflow does not change a resumed run
- **WHEN** a workflow file is edited after a run is created and the run is then resumed
- **THEN** the run executes the definition recorded at creation, not the edited file

#### Scenario: Resume fails when the recorded definition no longer compiles
- **WHEN** a skill used by the recorded definition has since been removed
- **THEN** the resumed run fails before any node executes and names the missing skill and the searched locations

### Requirement: Interactive waits use configured deadlines

An interaction deadline SHALL resolve only from configuration: the node's `ask_timeout` or `approval_timeout`, else the workflow's defaults; a question SHALL use the harness's suggested timeout only when nothing is configured. Without a deadline the wait SHALL continue until the operator answers or decides, or the run is cancelled, and SHALL NOT fail on a fallback deadline. Compiled values SHALL be pinned so editing the workflow does not change an existing run.

#### Scenario: Node overrides the workflow default

- **WHEN** a workflow sets an approval timeout and a node sets a different one
- **THEN** that node's approval wait uses the node's value

#### Scenario: Workflow default applies

- **WHEN** a node sets no interaction timeout and the workflow sets one
- **THEN** the node's question and approval waits use the workflow's value

#### Scenario: Nothing is configured

- **WHEN** neither the node nor the workflow sets an interaction timeout and the harness suggests none
- **THEN** the wait has no deadline and continues until the operator responds or the run ends

#### Scenario: Harness suggestion is honored

- **WHEN** no interaction timeout is configured and the harness asks with its own timeout
- **THEN** the question uses the harness's suggested deadline

#### Scenario: Edited workflow does not change a run

- **WHEN** a workflow's interaction timeout is edited after a run is created
- **THEN** the run keeps the deadline compiled at creation

### Requirement: Approval gates pause a run before a node

A node MAY require approval. Before each attempt starts, the run SHALL enter a waiting status naming the node, attempt, and action, and SHALL NOT start the attempt until approval arrives. Each attempt needs its own approval; a retry SHALL NOT reuse a decision. Rejection leaves it unstarted and ends the run cancelled; a previously failed node keeps its failure. A configured deadline that passes fails the node and run as timed out; without one the gate waits for the operator or cancellation.

#### Scenario: Approval lets the node run

- **WHEN** a node requires approval and the user approves it
- **THEN** the node executes and the run continues

#### Scenario: Rejection ends the run

- **WHEN** a node requires approval and the user rejects it
- **THEN** the node does not execute, a node that never ran is reported skipped, and the run ends cancelled

#### Scenario: Rejected retry keeps the failure

- **WHEN** a retried node's approval request is rejected
- **THEN** the node remains failed, the new attempt does not start, and the run ends cancelled

#### Scenario: Retry requires a new approval

- **WHEN** an approved attempt fails and the node is retried
- **THEN** the run waits for approval again and the earlier decision is not reused

#### Scenario: Approval timeout fails the run

- **WHEN** no decision arrives before the approval deadline
- **THEN** the node is reported failed as timed out and the run ends failed

#### Scenario: Approval request carries enough context

- **WHEN** approval is required for an attempt
- **THEN** the request identifies the node, its title, the attempt, the action to be taken, and the deadline when one is configured

#### Scenario: An unanswered approval keeps waiting

- **WHEN** no decision arrives and no approval deadline is configured
- **THEN** the run stays waiting for approval and does not fail

### Requirement: Harness questions are routed to the user and hold the run

When a running node's harness asks a question, the system SHALL report the question with the node and SHALL hold the node until an answer, a cancellation, or the deadline resolves it. Only one question SHALL be outstanding per node; a later question SHALL wait for the earlier one to resolve. The answer SHALL be delivered to the execution that asked, and a resolution arriving after the question resolved SHALL be discarded.

#### Scenario: Question is surfaced and answered
- **WHEN** a harness asks a question during a node
- **THEN** the run reports the question with the node identifier and continues after the answer is delivered

#### Scenario: Unanswered question reaches its deadline
- **WHEN** no answer arrives before the question's deadline
- **THEN** the question is cancelled, the harness is told so, and the node proceeds or fails by the harness's own response

#### Scenario: Second question waits
- **WHEN** a harness asks a second question while one is outstanding
- **THEN** the second is not surfaced as active until the first resolves, and both are answered in order

#### Scenario: Late answer is discarded
- **WHEN** an answer arrives after the question timed out or was cancelled
- **THEN** the answer is rejected and the execution is unaffected

#### Scenario: Free-form and suggested answers both work
- **WHEN** a question offers suggested answers and the user submits free-form text
- **THEN** the text is delivered without being rejected for not matching a suggestion

### Requirement: Clarifications are routed into the running node

The system SHALL accept a clarification only while a node is running, only when the node allows injection, and SHALL queue it for the execution's injection channel without blocking the request. It SHALL reject the request with a reason when no node is running, the node or harness disallows injection, or the queue is full. The adapter's acknowledgement SHALL record the delivery disposition.

#### Scenario: Clarification reaches the running node
- **WHEN** the user submits a clarification while a node that allows injection is running on a harness that supports it
- **THEN** the message is delivered to that execution and its disposition is recorded

#### Scenario: No running node
- **WHEN** the user submits a clarification for a run that is not running
- **THEN** the request is rejected with a reason and no execution is affected

#### Scenario: Node disallows injection
- **WHEN** the user submits a clarification for a running node whose configuration disables injection
- **THEN** the request is rejected with a reason naming the node

#### Scenario: Messages accumulate while the harness is busy
- **WHEN** several clarifications arrive while the harness is not reading its injection channel
- **THEN** every message is queued and delivered in order without blocking any request

#### Scenario: Full queue is rejected
- **WHEN** the injection queue is full
- **THEN** the request is rejected with a reason and no message is silently dropped

#### Scenario: Delivery disposition is recorded
- **WHEN** the adapter acknowledges an injected message as delivered, queued, or failed
- **THEN** the run records that disposition and publishes `injection.acknowledged`

### Requirement: A pending interaction is visible in the run record

While a run waits for an answer or an approval, its record SHALL include a pending interaction naming the kind, identifier, node, attempt, question or action, and deadline. The record SHALL be cleared when the interaction resolves. A client that reads the run SHALL see the pending interaction without replaying the event stream.

#### Scenario: Pending question is visible
- **WHEN** a client reads a run that is waiting for an answer
- **THEN** the record identifies the question, its node, options, and deadline

#### Scenario: Pending approval is visible
- **WHEN** a client reads a run that is waiting for approval
- **THEN** the record identifies the approval, its node, attempt, action, and deadline

#### Scenario: Resolution clears the pending interaction
- **WHEN** a question is answered or an approval is decided
- **THEN** the run record no longer reports a pending interaction

### Requirement: Interactive transitions are published as events

Every interactive transition SHALL be published as a sequenced run event identifying the interaction and the node. The events SHALL be named `question.asked`, `question.answered`, `question.timed_out`, `question.cancelled`, `injection.received`, `injection.acknowledged`, `injection.rejected`, `approval.required`, `approval.granted`, `approval.rejected`, and `approval.timed_out`.

#### Scenario: Question lifecycle is observable
- **WHEN** a question is asked and then answered
- **THEN** subscribers receive `question.asked` followed by `question.answered` with the same identifier

#### Scenario: Question timeout is observable
- **WHEN** a question reaches its deadline without an answer
- **THEN** subscribers receive `question.timed_out`

#### Scenario: Injection lifecycle is observable
- **WHEN** a clarification is accepted, acknowledged, or rejected
- **THEN** subscribers receive `injection.received`, `injection.acknowledged`, or `injection.rejected` respectively

#### Scenario: Approval lifecycle is observable
- **WHEN** approval is requested and then granted, rejected, or timed out
- **THEN** subscribers receive `approval.required` followed by the matching terminal approval event

### Requirement: Interactive HTTP endpoints validate requests and report outcomes

The `POST /api/runs/{id}/answers`, `/injections`, and `/approve` endpoints SHALL require their identifying fields, return 200 on acceptance, 400 for a malformed or incomplete request, 404 for an unknown run, 409 when the run or node cannot accept the operation, and 410 when the interaction already resolved. Inject SHALL report `queued` rather than claiming delivery.

#### Scenario: Valid request is accepted
- **WHEN** a client answers an outstanding question with its identifier and a value
- **THEN** the response is 200 and the answer is delivered to the waiting execution

#### Scenario: Approval decision is accepted
- **WHEN** a client approves or rejects a pending gate with its node and decision
- **THEN** the response is 200 and the decision resolves the wait

#### Scenario: Incomplete request is rejected
- **WHEN** a client sends an answer without a question identifier or an injection without text
- **THEN** the response is 400 and no interaction is affected

#### Scenario: Unknown run is rejected
- **WHEN** a client targets a run identifier the project does not have
- **THEN** the response is 404

#### Scenario: Unacceptable state is rejected
- **WHEN** a client injects into a node that disallows injection or into a run with no running node
- **THEN** the response is 409 with a reason

#### Scenario: Resolved interaction is rejected
- **WHEN** a client answers a question or approves a gate that already resolved
- **THEN** the response is 410

#### Scenario: Injection is acknowledged as queued
- **WHEN** a client submits a valid clarification
- **THEN** the response is 200 with status `queued`

### Requirement: Run events are sequenced and replayable

Every run SHALL assign a monotonically increasing sequence number to each event it emits and SHALL retain events so a client that reconnects can resume from the last event it received. A reconnect that supplies a sequence number SHALL receive every event after it and no duplicates, and an event stream SHALL report events in sequence order.

#### Scenario: Late subscriber catches up
- **WHEN** a client subscribes to a running run and supplies the last sequence number it saw
- **THEN** it receives every later event in order and no earlier one

#### Scenario: Sequence numbers are strictly increasing
- **WHEN** events are emitted for a run
- **THEN** each event's sequence number is greater than the previous one

### Requirement: Cancelling a run interrupts a pending interaction

Cancelling a run while it waits for an answer or an approval SHALL interrupt that wait immediately, clear the pending interaction, and end the run cancelled. No interaction handler SHALL remain blocked after the cancellation returns.

#### Scenario: Cancel during a question
- **WHEN** a run is cancelled while a question is outstanding
- **THEN** the question is resolved as cancelled, the node stops, and the run ends cancelled

#### Scenario: Cancel during approval
- **WHEN** a run is cancelled while approval is pending
- **THEN** the approval wait ends, the pending interaction is cleared, and the run ends cancelled

### Requirement: Cancelling a run stops the running node

Cancelling a run SHALL cancel the execution of the node that is currently running, SHALL wait for it to stop within a bounded period, and SHALL end the run as cancelled. Nodes that had not started SHALL be reported skipped.

#### Scenario: Cancel while a node runs
- **WHEN** the user cancels a run whose node is executing
- **THEN** the node's execution is cancelled, the node is reported cancelled, and the run ends cancelled

#### Scenario: Cancel leaves no execution running
- **WHEN** a run is cancelled and the operation returns
- **THEN** no node execution from that run is still running

### Requirement: Run event streams end when the run terminates

When a run reaches `completed`, `failed`, or `cancelled`, the system SHALL publish the terminal event, deliver every event already retained for the run, and end every open event stream. A subscriber that connects after termination SHALL receive the retained events and then EOF. Resuming or retrying the run SHALL open a new live epoch on the same sequence.

#### Scenario: Live stream ends after the terminal event
- **WHEN** a run reaches a terminal status while a client is subscribed
- **THEN** the client receives the terminal event and the stream ends without waiting for another event

#### Scenario: Late subscriber receives history and EOF
- **WHEN** a client subscribes after the run has terminated
- **THEN** it receives the retained events in sequence order and the stream ends

#### Scenario: Resume opens a new live epoch
- **WHEN** a failed or cancelled run is resumed or retried
- **THEN** new subscribers receive the retained history and then live events from the new execution

### Requirement: Slow event subscribers are shed without blocking execution

A subscriber that cannot keep up with the live stream SHALL be disconnected rather than block event publication or node execution. A subscriber that reconnects with the last sequence number it received SHALL receive every later event in order.

#### Scenario: Slow subscriber is disconnected
- **WHEN** a subscriber's buffer is full because it is not reading
- **THEN** that subscriber's stream ends and the engine continues publishing without waiting for it

#### Scenario: Other subscribers are unaffected
- **WHEN** one subscriber is disconnected for being slow
- **THEN** every other subscriber continues to receive events in order

#### Scenario: Shed subscriber catches up
- **WHEN** a disconnected subscriber reconnects with its last received sequence number
- **THEN** it receives every later retained event and no earlier one

### Requirement: A terminal event stream stops reconnecting at the tail

For a terminal run, the system SHALL answer a client at the tail with `204 No Content` and no body, a client behind the tail with `200` plus the later events then EOF, and a client without a cursor with the full retained stream then EOF. An unconditional `410` would make a page reload lose history and SHALL NOT be used.

#### Scenario: Cursor at the terminal tail
- **WHEN** a terminal run's last event has already been received and the client reconnects with that sequence number
- **THEN** the server responds `204 No Content` and the client stops reconnecting

#### Scenario: Cursor behind the terminal tail
- **WHEN** a terminal run has later events than the client's last sequence number
- **THEN** the server responds `200` with those later events and then ends the stream

#### Scenario: Fresh client reads the full history
- **WHEN** a client connects to a terminal run without a cursor
- **THEN** the server responds `200` with every retained event and then ends the stream

### Requirement: An active event stream sends heartbeats while idle

While a run is active and no event is available, the system SHALL send a heartbeat on the event stream at a bounded interval so an idle connection is not closed by an intermediary.

#### Scenario: Idle stream remains open
- **WHEN** a run is active and no event is published for the configured heartbeat interval
- **THEN** the client receives a heartbeat and the stream remains open

### Requirement: Transition events expose lifecycle payloads

Run events SHALL keep dot-separated type names and a stable envelope of sequence, timestamp, type, run identifier, optional node identifier, and data object. Transition events SHALL carry the fields a client needs to render progress: workflow identifier and objective on `run.started`; attempt on `node.started`; error and retryability on `node.failed` and `node.postcondition_failed`; postcondition outcome on `node.completed`; and summary on `run.completed`.

#### Scenario: Run start identifies the workflow and objective
- **WHEN** a run starts
- **THEN** its `run.started` event carries the workflow identifier and the objective

#### Scenario: Node start carries the attempt
- **WHEN** a node begins an attempt
- **THEN** its `node.started` event carries the attempt number

#### Scenario: Node failure carries retryability
- **WHEN** a node fails, including a postcondition failure
- **THEN** its failure event carries the error text and whether the failure is retryable

#### Scenario: Node completion carries the postcondition outcome
- **WHEN** a node completes
- **THEN** its `node.completed` event carries whether the postcondition held and any resolved change

#### Scenario: Run completion carries a summary
- **WHEN** a run completes
- **THEN** its `run.completed` event carries a summary of the execution

### Requirement: The event journal is repaired after an unclean shutdown

On open, the system SHALL discard a partial trailing record, SHALL report a record that exceeds its read limit instead of silently truncating replay, and SHALL report an append failure. A repaired journal SHALL continue sequence numbering without reusing a retained number.

#### Scenario: Partial trailing record is discarded
- **WHEN** the journal ends with a truncated JSON record after a crash
- **THEN** that record is not replayed and new events append after the last complete record

#### Scenario: Oversized record fails loudly
- **WHEN** a journal record exceeds the configured read limit
- **THEN** opening the journal reports the failure instead of silently returning a shorter history

#### Scenario: Append failure is reported
- **WHEN** writing a new event to the journal fails
- **THEN** the failure is reported rather than ignored

### Requirement: Terminal runs release live stream resources

When a run reaches a terminal status, the system SHALL release the resources it held
to stream that run, including its live subscriber set and its open event journal. A
later replay SHALL still return the retained events, and a resumed or retried run
SHALL continue the same sequence.

#### Scenario: Terminal run releases its stream resources
- **WHEN** a run reaches a terminal status and leaves the live set
- **THEN** no live subscriber remains and the run's journal is no longer held open

#### Scenario: Replay after release still returns history
- **WHEN** a client requests events for a terminal run that is no longer in the live set
- **THEN** the server reads the journal and returns the retained events in sequence order

#### Scenario: Resume reopens the journal
- **WHEN** a failed or cancelled run is resumed or retried after release
- **THEN** its journal is reopened and new events continue from the retained sequence

### Requirement: The workflow catalog lists a project's workflows

The system SHALL expose `GET /api/workflows?project=` returning every `.yaml` workflow file directly under the project's `.specflow/workflows/` and `workflows/` directories, ordered by reference, with a reference that exists in both directories listed once from `.specflow/workflows/`. Each entry SHALL carry the reference accepted by run creation, its path, name, description, version, structural validity, structural problems, nodes, and edges.

#### Scenario: Workflows from both directories are listed
- **WHEN** a project has one workflow under `.specflow/workflows/` and another under `workflows/`
- **THEN** both are returned, ordered by reference

#### Scenario: The project copy wins a duplicate reference
- **WHEN** the same reference exists under both directories
- **THEN** one entry is returned and it names the `.specflow/workflows/` path

#### Scenario: An unusable project is rejected
- **WHEN** the `project` query parameter is absent or the path is not a git repository
- **THEN** the response is `400` with a JSON error body

### Requirement: Workflow catalog reads are structural and side-effect free

The catalog SHALL parse and structurally validate each workflow without compiling it, resolving a skill, probing a harness, or changing the project. A file that cannot be parsed or fails structural validation SHALL still be listed, marked invalid, and carry its reported problems, and SHALL NOT fail the other entries.

#### Scenario: A missing skill does not invalidate a listing
- **WHEN** a workflow binds a skill that does not exist in the project
- **THEN** the workflow is listed as structurally valid

#### Scenario: A broken file is listed with its problem
- **WHEN** a workflow file is not valid YAML
- **THEN** that entry is marked invalid with its error and every other workflow is still returned

#### Scenario: Reading the catalog changes nothing
- **WHEN** the catalog is read
- **THEN** no run, worktree, or project file is created, changed, or removed

### Requirement: A run's graph is read from the definition it was pinned to

The system SHALL expose `GET /api/runs/{id}/graph?project=` returning the run's authored nodes and edges from the workflow definition retained for that run, falling back to the recorded workflow path for a run created before snapshots. Each node SHALL carry its identifier, title, kind, and effective interactive, injection, and approval flags; each edge SHALL name its source and target.

#### Scenario: The pinned definition wins over an edited file
- **WHEN** the project's workflow file is edited after the run is created
- **THEN** the graph reflects the definition retained with the run

#### Scenario: A legacy run falls back to its recorded path
- **WHEN** a run was created before retained snapshots existed
- **THEN** its recorded workflow path supplies the graph

#### Scenario: An unknown run is not found
- **WHEN** the run identifier is unknown to the project
- **THEN** the response is `404` with a JSON error body

### Requirement: Graph reads never compile or resolve

The run graph endpoint SHALL parse the retained definition only. It SHALL NOT compile the workflow, resolve a skill directory, probe a harness, execute a node, emit a run event, or change the run record, so a skill or harness that has disappeared since creation SHALL NOT prevent the graph from being returned.

#### Scenario: A vanished skill does not block the graph
- **WHEN** a skill bound by the run no longer exists
- **THEN** the graph is still returned

#### Scenario: Reading a live run's graph changes nothing
- **WHEN** the graph is read while the run is running
- **THEN** the run's status and last event sequence are unchanged

#### Scenario: An unreadable retained definition is reported
- **WHEN** the retained definition cannot be read or parsed
- **THEN** the response is `500` with a JSON error body

### Requirement: Waiting for an answer does not consume the execution timeout

An agent node's execution timeout SHALL measure execution rather than waiting. While a question is pending, the node's remaining execution time SHALL pause, and it SHALL resume when the question resolves. Cancelling a run while a question is pending SHALL end the run cancelled, not failed as timed out.

#### Scenario: A long answer wait does not time out the node

- **WHEN** a question stays unanswered for longer than the node's execution timeout and is then answered
- **THEN** the node continues and is not reported timed out

#### Scenario: The remaining time resumes after the answer

- **WHEN** a pending question resolves
- **THEN** the node's remaining execution time continues from where it paused

#### Scenario: Cancelling while waiting ends cancelled

- **WHEN** the operator cancels the run while a question is pending
- **THEN** the run ends cancelled rather than failed as timed out

### Requirement: Agent nodes enforce their timeout

An `agent` node SHALL execute under its compiled timeout. When that timeout expires, the system SHALL stop the harness execution, report the node failed as timed out, and treat the failure as retryable. Cancellation by the user or by run termination SHALL remain a cancellation and SHALL NOT be reported as a timeout.

#### Scenario: Agent node exceeds its timeout
- **WHEN** an `agent` node runs longer than its compiled timeout
- **THEN** the harness execution is stopped and the node is reported failed as timed out

#### Scenario: Timeout failure is retryable
- **WHEN** an `agent` node fails because its timeout expired
- **THEN** the failure is classified retryable and the node can be retried

#### Scenario: Cancellation is not a timeout
- **WHEN** a run is cancelled while an `agent` node is running before its deadline
- **THEN** the node and run are reported cancelled, not timed out

### Requirement: An interrupted in-flight run is repaired when observed

A persisted run found with status `running`, `waiting_approval`, or
`waiting_answer` and no live execution SHALL be repaired as interrupted: clear
the pending interaction, fail the interrupted node with a retryable reason
naming the restart, end the run failed, and publish the node and terminal
events. Later unfinished nodes SHALL be skipped. The run SHALL remain retryable
and resumable, and repair SHALL NOT change a live, terminal, or already
repaired run.

#### Scenario: Restart while waiting for approval
- **WHEN** the server reads a run whose record says `waiting_approval` and the
  process holds no execution for it
- **THEN** the run reads failed, its pending approval is gone, and the
  interrupted node is failed with a reason naming the restart

#### Scenario: Restart while waiting for an answer
- **WHEN** the server reads a run whose record says `waiting_answer` and the
  process holds no execution for it
- **THEN** the run reads failed, its pending question is gone, and the
  interrupted node is failed with a reason naming the restart

#### Scenario: Restart while a node is running
- **WHEN** the server reads a run whose record says `running` and the process
  holds no execution for it
- **THEN** the running node and the run are failed with the restart reason, and
  the run is resumable

#### Scenario: Later nodes are not left pending
- **WHEN** the interrupted node is not the last unfinished node
- **THEN** the later unfinished nodes are skipped and read as not run

#### Scenario: A live run is untouched
- **WHEN** the run is executing in the current process
- **THEN** reading it leaves its status and pending interaction unchanged

#### Scenario: Terminal runs are untouched
- **WHEN** the run is completed, failed, or cancelled
- **THEN** reading it makes no change and appends no event

#### Scenario: Repair happens once
- **WHEN** the repaired run is read again, or read concurrently
- **THEN** the record is unchanged and no second terminal event is appended

#### Scenario: Listing repairs the record
- **WHEN** a client lists the project's runs after a restart
- **THEN** the interrupted run is listed failed rather than waiting

#### Scenario: An unreadable journal does not block repair
- **WHEN** the run's event journal cannot be opened
- **THEN** the record is still repaired and the journal failure is reported
