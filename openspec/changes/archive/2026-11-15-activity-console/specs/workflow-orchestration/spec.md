# Spec Delta

## MODIFIED Requirements

### Requirement: Command nodes run in the run workspace

A `command` node SHALL execute its command through a shell in the node's declared working directory, pass the node's environment, enforce the node's timeout, and treat a non-zero exit as node failure. It SHALL NOT involve a harness. It SHALL record the rendered command in the run's node record and SHALL publish `command.started` before execution and `command.finished` with the exit code, the captured output, and the duration after every outcome, including a non-zero exit, a timeout, and a cancellation.

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
