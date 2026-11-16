# Spec Delta

## ADDED Requirements

### Requirement: Interactive waits use configured deadlines

The system SHALL resolve an approval deadline from the node's configured timeout, else the workflow default, else 30 minutes. A question SHALL use the node's or workflow's configured deadline when set, otherwise the harness's suggested timeout, otherwise 30 minutes. The compiled plan SHALL carry the configured node and workflow values, so editing a workflow SHALL NOT change a run already created.

#### Scenario: Node overrides the workflow default
- **WHEN** a workflow sets an approval timeout and a node sets a different one
- **THEN** that node's approval wait uses the node's value

#### Scenario: Workflow default applies
- **WHEN** a node sets no interaction timeout and the workflow sets one
- **THEN** the node's question and approval waits use the workflow's value

#### Scenario: Nothing is configured
- **WHEN** neither the node nor the workflow sets an interaction timeout and the harness suggests none
- **THEN** the wait uses 30 minutes

#### Scenario: Harness suggestion is honored
- **WHEN** no interaction timeout is configured and the harness asks with its own timeout
- **THEN** the question uses the harness's suggested deadline

#### Scenario: Edited workflow does not change a run
- **WHEN** a workflow's interaction timeout is edited after a run is created
- **THEN** the run keeps the deadline compiled at creation

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

### Requirement: Cancelling a run interrupts a pending interaction

Cancelling a run while it waits for an answer or an approval SHALL interrupt that wait immediately, clear the pending interaction, and end the run cancelled. No interaction handler SHALL remain blocked after the cancellation returns.

#### Scenario: Cancel during a question
- **WHEN** a run is cancelled while a question is outstanding
- **THEN** the question is resolved as cancelled, the node stops, and the run ends cancelled

#### Scenario: Cancel during approval
- **WHEN** a run is cancelled while approval is pending
- **THEN** the approval wait ends, the pending interaction is cleared, and the run ends cancelled

## MODIFIED Requirements

### Requirement: Approval gates pause a run before a node

A node MAY require approval. Before each attempt starts, the run SHALL enter a waiting status naming the node, attempt, and action, and SHALL NOT start the attempt until approval arrives. Every attempt needs its own approval; a retry SHALL NOT reuse a decision. Rejection leaves the attempt unstarted and ends the run cancelled; a node that had already run keeps its failure. Passing the deadline fails the node and run as timed out.

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
- **THEN** the request identifies the node, its title, the attempt, the action to be taken, and the deadline

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
