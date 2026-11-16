# Spec Delta

## MODIFIED Requirements

### Requirement: Interactive waits use configured deadlines

An interaction deadline SHALL resolve only from configuration: the node's `ask_timeout` or `approval_timeout`, else the workflow's defaults. A question SHALL use the harness's suggested timeout only when no configured value exists. When no deadline resolves, the wait SHALL continue until the operator answers or decides, or the run is cancelled, and SHALL NOT fail the run on a fallback deadline. The compiled plan SHALL carry the configured node and workflow values, so editing a workflow SHALL NOT change a run already created.

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

A node MAY require approval. Before each attempt starts, the run SHALL enter a waiting status naming the node, attempt, and action, and SHALL NOT start the attempt until approval arrives. Every attempt needs its own approval; a retry SHALL NOT reuse a decision. Rejection leaves the attempt unstarted and ends the run cancelled; a node that had already run keeps its failure. A configured deadline that passes fails the node and run as timed out; without a deadline the gate waits until the operator decides or the run is cancelled.

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

## ADDED Requirements

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
