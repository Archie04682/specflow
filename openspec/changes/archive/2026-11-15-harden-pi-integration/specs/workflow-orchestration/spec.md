# Spec Delta

## ADDED Requirements

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
