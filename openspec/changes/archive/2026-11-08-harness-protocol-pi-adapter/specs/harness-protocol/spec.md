# Spec Delta

## Purpose

Defines the harness-agnostic contract every coding-agent adapter must satisfy so SpecFlow can execute a workflow node on any harness — streaming its work, asking the user questions, delivering mid-run clarifications, and cancelling cleanly — without the orchestrator knowing which runtime is behind it.

## ADDED Requirements

### Requirement: Harness discovery

The system SHALL maintain a registry of harness adapters keyed by a stable identifier and SHALL expose it at `GET /api/harnesses`, returning each harness's identifier, display name, version, readiness, and declared capabilities as JSON.

#### Scenario: Installed harnesses are listed
- **WHEN** the client sends `GET /api/harnesses`
- **THEN** the response is `200` with `Content-Type: application/json` and a body listing the `pi` harness with its identifier, display name, version, readiness, and capability flags

#### Scenario: Unavailable runtime is reported, not hidden
- **WHEN** a registered harness's runtime executable is not installed
- **THEN** the response still lists it, marked not ready, with a message naming the missing executable

### Requirement: Truthful capability declaration

Each adapter SHALL declare a fixed set of capability flags covering streaming, asking the user, mid-run injection, graceful abort, skills, model selection, tool selection, plugins, usage reporting, and session resume. A flag SHALL be `true` only when the feature actually works in the current environment, and a capability found unavailable during probing SHALL be reported as `false`.

#### Scenario: Failed component downgrades a capability
- **WHEN** a harness supports asking the user in principle but its bridge component fails to load while probing
- **THEN** the reported capability for asking the user is `false`

#### Scenario: Declaration is available without executing work
- **WHEN** the harness is listed or preflight-checked
- **THEN** its capabilities are returned without starting an execution

### Requirement: Preflight rejection of unmet node requirements

The system SHALL compare each node's declared requirements — interactive questions, mid-run injection, a bound skill, an explicit model, a tool policy — against the selected adapter's capabilities before that node starts, and SHALL refuse to start a node whose requirements the adapter cannot meet.

#### Scenario: Interactive node on a harness that cannot ask
- **WHEN** a node declares `interactive: true` and the selected harness reports that it cannot ask the user
- **THEN** the node is not started and the refusal names the node and the unmet capability

#### Scenario: Satisfied requirements are accepted
- **WHEN** every declared requirement of a node is supported by the selected harness
- **THEN** preflight accepts the node without an error

### Requirement: Exactly one terminal outcome per execution

Each node execution SHALL produce exactly one terminal outcome — completed, failed, aborted, timed out, or cancelled — carrying the final assistant text, turn count, tool call count, usage when available, and file changes when available. Completion SHALL be signalled by that outcome and not by a stream event; an event stream that ends without a terminal outcome SHALL be reported as a failure.

#### Scenario: Successful execution
- **WHEN** the harness finishes its work and reports that it has settled
- **THEN** the outcome is `completed` and carries the final assistant text

#### Scenario: Stream ends without a terminal outcome
- **WHEN** the harness process exits after emitting events but before settling
- **THEN** the outcome is a failure classified as an unexpected harness exit and includes the captured diagnostics

### Requirement: Telemetry is best-effort

Incremental text, reasoning, tool activity, file changes, usage, notices, and status labels SHALL be treated as optional telemetry. Their absence, truncation, or an unrecognized record type SHALL NOT fail an execution, and an unrecognized record type SHALL be reported at most once as a notice.

#### Scenario: Unknown record does not fail the node
- **WHEN** the harness emits a record whose type the adapter does not recognize
- **THEN** the adapter emits one notice for that record type, continues consuming the stream, and the node still completes

#### Scenario: Harness emits no telemetry
- **WHEN** a harness completes without emitting any telemetry records
- **THEN** the node completes successfully with an empty event history

### Requirement: Interactive questions block until resolved

When a harness raises a question, that execution SHALL pause until the question is answered, cancelled, or reaches its deadline, and the resolution SHALL be delivered back to the same execution. Questions SHALL support free text, choosing from a list, confirmation, and multi-line editing. A resolution arriving after the question was cancelled SHALL be discarded without affecting the execution.

#### Scenario: User answers before the deadline
- **WHEN** the harness raises a question and the user submits an answer before the deadline
- **THEN** the answer is delivered to the harness and the execution continues

#### Scenario: Deadline expires with no answer
- **WHEN** no answer arrives before the question's deadline
- **THEN** the question is cancelled, the harness receives a cancellation, and the execution continues or fails according to the harness's own response

#### Scenario: Late answer is ignored
- **WHEN** an answer arrives for a question that was already cancelled
- **THEN** the answer is discarded and the execution is unaffected

### Requirement: Mid-run clarification delivery

When a node allows injection and its harness supports it, the system SHALL deliver a user message into the running execution without restarting it and SHALL report the delivery disposition. The system SHALL reject injection before delivery when the node or the harness does not allow it.

#### Scenario: Clarification reaches a running execution
- **WHEN** the user submits a clarification while a node is running and both the node and the harness allow injection
- **THEN** the harness acknowledges the message and the event history records its delivery disposition

#### Scenario: Injection refused when not allowed
- **WHEN** the user submits a clarification for a node whose configuration disables injection
- **THEN** the request is rejected with an error naming the node and no work is interrupted

### Requirement: Bounded cancellation

Cancelling an execution SHALL request a graceful stop first and SHALL terminate the harness process within a bounded grace period. A cancelled execution SHALL be reported as cancelled rather than failed, and SHALL leave no harness process running.

#### Scenario: Graceful stop
- **WHEN** an execution is cancelled while the harness is running
- **THEN** the harness receives a stop request, the execution ends within the grace period, and the outcome is `cancelled`

#### Scenario: Harness ignores the stop request
- **WHEN** the harness does not stop within the grace period
- **THEN** it is terminated forcibly and the outcome is still `cancelled`

### Requirement: Classified failures

Every unsuccessful execution SHALL carry a classification from a fixed set — missing adapter, unsupported requirement, configuration, authentication, start failure, protocol violation, unexpected exit, or timeout — and SHALL state whether retrying could succeed.

#### Scenario: Missing runtime is not retryable
- **WHEN** the harness executable cannot be found
- **THEN** the failure is classified as a missing adapter and marked not retryable

#### Scenario: Unexpected exit is retryable
- **WHEN** the harness process dies unexpectedly mid-execution
- **THEN** the failure is classified as an unexpected exit and marked retryable
