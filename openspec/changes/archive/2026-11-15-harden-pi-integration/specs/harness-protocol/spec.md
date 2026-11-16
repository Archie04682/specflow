# Spec Delta

## MODIFIED Requirements

### Requirement: Classified failures

Every unsuccessful execution SHALL carry a classification from a fixed set — missing adapter, unsupported requirement, configuration, authentication, provider error, start failure, protocol violation, unexpected exit, or timeout — and SHALL state whether retrying could succeed. Authentication failures and permanent configuration failures SHALL NOT be retryable; a transient provider error SHALL be.

#### Scenario: Missing runtime is not retryable
- **WHEN** the harness executable cannot be found
- **THEN** the failure is classified as a missing adapter and marked not retryable

#### Scenario: Unexpected exit is retryable
- **WHEN** the harness process dies unexpectedly mid-execution
- **THEN** the failure is classified as an unexpected exit and marked retryable

#### Scenario: Missing credentials are authentication
- **WHEN** a harness rejects a prompt because no credentials are available
- **THEN** the failure is classified as authentication and marked not retryable

#### Scenario: Unknown model is configuration
- **WHEN** a harness cannot start because the requested model does not exist
- **THEN** the failure is classified as configuration and marked not retryable

#### Scenario: Transient provider error is a provider failure
- **WHEN** a harness reports a rate-limited or overloaded provider after accepting the prompt
- **THEN** the failure is classified as a provider error and marked retryable

#### Scenario: Deadline expiry is a timeout
- **WHEN** an execution reaches its deadline while the harness is running
- **THEN** the failure is classified as timeout and marked retryable

## ADDED Requirements

### Requirement: In-band execution errors are not completions

When a harness reports that its own run ended with an unrecoverable error, the execution SHALL be reported as a failure carrying that error, even if the harness process settles normally.

#### Scenario: Harness settles after reporting a provider error
- **WHEN** the harness reports an unrecoverable provider error and then settles without further work
- **THEN** the outcome is failed with the reported error, not completed with empty text
