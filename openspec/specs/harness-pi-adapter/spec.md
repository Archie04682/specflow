# harness-pi-adapter Specification

## Purpose

Defines how SpecFlow drives Pi as its first harness: starting the process in its bidirectional RPC mode, binding skills and models deterministically, translating Pi's records into the shared contract, routing Pi's questions to the user through an embedded bridge extension, and shutting the process down without leaking it.

## Requirements

### Requirement: Pi runs in RPC mode as a controlled child process

The adapter SHALL start Pi once per node execution as a long-lived child process in RPC mode, in the node's working directory, with the node's model, thinking level, tool selection, and project-trust setting applied at startup, and SHALL confirm the process is responsive before submitting work.

#### Scenario: Process starts and answers a readiness request
- **WHEN** a node execution begins on the Pi harness
- **THEN** Pi is started in RPC mode with the node's working directory, and a state request is answered before the prompt is submitted

#### Scenario: Startup fails
- **WHEN** Pi cannot be started or does not answer the readiness request within the startup timeout
- **THEN** the execution fails with a start-failure classification and the captured diagnostics

### Requirement: Deterministic skill binding

When a node binds a skill, the adapter SHALL load that skill from an absolute path and SHALL invoke it explicitly by name, so the execution does not depend on skills discovered from the user's global environment. When the skill's path does not exist, the adapter SHALL fail before submitting any prompt. After Pi starts, the adapter SHALL confirm the skill loaded and SHALL fail before prompting when it did not.

#### Scenario: Bound skill is invoked explicitly
- **WHEN** a node binds the `openspec-explore` skill and its directory exists
- **THEN** Pi is started with that skill loaded and the prompt names the skill explicitly

#### Scenario: Missing skill path fails before prompting
- **WHEN** a node binds a skill whose path does not exist
- **THEN** the execution fails as a configuration failure and no prompt is sent

#### Scenario: Unloadable skill fails before prompting
- **WHEN** a node binds a skill directory that Pi accepts as a path but does not load, such as a `SKILL.md` without a description
- **THEN** the execution fails as a configuration failure naming the skill, before any prompt is sent

### Requirement: Pi activity is translated into contract events

The adapter SHALL translate Pi's session records into the shared contract: incremental assistant text and reasoning, tool start/update/end, file changes derived from write and edit tool calls, cumulative usage, turn boundaries, compaction and retry notices, and the settled marker that terminates the execution.

#### Scenario: Streamed text is delivered incrementally
- **WHEN** Pi streams text deltas for an assistant message
- **THEN** each delta is delivered as it arrives and the completed message supplies the final text

#### Scenario: Tool activity is visible
- **WHEN** Pi starts, updates, and finishes a tool execution
- **THEN** three corresponding events are delivered in that order, correlated by the tool call identifier

#### Scenario: Turn completion is not execution completion
- **WHEN** Pi reports the end of one agent run but automatic retry, recovery, or queued work may still follow
- **THEN** the execution continues and terminates only on the settled marker

### Requirement: Questions reach the user through an embedded bridge

The adapter SHALL make a question tool available to models running on Pi by loading an extension that is embedded in the executable and materialized at runtime, so that a user can answer a question without Pi's own terminal interface being present. The adapter SHALL report the question tool call's own question text as the ask's description when Pi's dialog request cannot carry it, so that the operator receives the question's context for every ask kind.

#### Scenario: Model asks and the user answers

- **WHEN** the model calls the question tool and the user submits an answer
- **THEN** the answer is returned to the model as the tool result and Pi continues the same execution

#### Scenario: Bridge extension cannot load

- **WHEN** the embedded extension cannot be loaded by the installed Pi version
- **THEN** probing reports that asking the user is unsupported and the startup failure names the incompatibility

#### Scenario: The question text reaches the operator

- **WHEN** the model calls the question tool with a question and Pi's dialog request carries only a title
- **THEN** the reported ask carries the dialog's title, kind, and options together with the question text as its description

#### Scenario: A dialog that carries its own message is unchanged

- **WHEN** Pi's dialog request already carries a message, as a confirmation does
- **THEN** the reported ask carries that message

#### Scenario: An unmatched dialog is still answerable

- **WHEN** a dialog arrives with no corresponding question tool call
- **THEN** the reported ask carries the dialog's own fields and can still be answered

### Requirement: Injected clarifications use Pi's steering semantics

A clarification SHALL be delivered with Pi's steering primitive while a turn is running and as a follow-up message when the agent is idle, and the resulting disposition SHALL be reported.

#### Scenario: Steering while a turn is running
- **WHEN** a clarification is injected while Pi is executing tool calls
- **THEN** it is queued as a steering message and its disposition is reported as queued

#### Scenario: Follow-up when idle
- **WHEN** a clarification is injected while the agent has no running turn
- **THEN** it is delivered as a follow-up message and its disposition is reported

### Requirement: Orderly shutdown without orphaned processes

On completion or cancellation the adapter SHALL close Pi's standard input to request shutdown, SHALL wait for the grace period, and SHALL escalate to termination and then forced kill if the process has not exited. The adapter SHALL reap the process in every path.

#### Scenario: Clean shutdown after settled
- **WHEN** Pi reports that it has settled
- **THEN** its standard input is closed, it exits within the grace period, and the adapter returns the completed outcome

#### Scenario: Cancellation escalates
- **WHEN** an execution is cancelled and Pi does not exit after its input is closed and the grace period elapses
- **THEN** the adapter terminates the process, forces it if necessary, and still reports the execution as cancelled

### Requirement: Diagnostics are isolated from the protocol

The adapter SHALL treat standard output as protocol records only and standard error as diagnostics. Diagnostics SHALL be captured with a bounded buffer, SHALL never be parsed as protocol data, and SHALL be attached to failures.

#### Scenario: Pi logs to stderr
- **WHEN** Pi writes human-readable diagnostics to standard error while healthy
- **THEN** those bytes do not appear as events and the execution is unaffected

#### Scenario: Failure carries diagnostics
- **WHEN** an execution fails
- **THEN** the failure includes the most recent captured diagnostics

### Requirement: Missing or incompatible Pi fails fast

The adapter SHALL verify that a Pi executable is available and reports a supported version before starting an execution, and SHALL fail with an actionable, non-retryable message rather than hanging when it does not.

#### Scenario: Pi is not installed
- **WHEN** no Pi executable can be found on the path
- **THEN** the failure is classified as a missing adapter and names the executable that was searched for

#### Scenario: Pi predates the supported protocol
- **WHEN** the installed Pi reports a version older than the minimum the adapter supports
- **THEN** the failure is classified as a configuration failure and states the minimum supported version

### Requirement: Protocol framing is line-fed and separator-safe

The adapter SHALL split Pi's standard output into records on the line-feed byte only, SHALL accept an optional preceding carriage return, and SHALL NOT treat Unicode line or paragraph separators inside JSON strings as record boundaries. A record that cannot be parsed before the execution settles SHALL be reported as a protocol violation.

#### Scenario: Separator characters inside a payload
- **WHEN** a record's JSON string contains a Unicode line or paragraph separator
- **THEN** the record is delivered intact and is not split

#### Scenario: Malformed record before settling
- **WHEN** a complete line cannot be parsed as JSON before Pi settles
- **THEN** the execution fails with a protocol-violation classification

### Requirement: The bridge handshake is not telemetry

The adapter SHALL consume the embedded extension's bridge-ready notification as a startup handshake and SHALL NOT publish it as a node event.

#### Scenario: Bridge marker is swallowed during an execution
- **WHEN** the bridge extension loads at the start of a node execution
- **THEN** no node event carries the bridge-ready message
