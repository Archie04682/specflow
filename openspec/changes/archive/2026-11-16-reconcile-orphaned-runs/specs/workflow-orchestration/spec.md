# Spec Delta

## ADDED Requirements

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
