# Spec Delta

## ADDED Requirements

### Requirement: A project can be chosen with the operating system's folder dialog

The dashboard SHALL offer a browse action beside the project path that asks the local server to open the operating system's folder dialog. On selection it SHALL fill the project path; on cancellation it SHALL leave the current value unchanged; when the dialog is unavailable or fails it SHALL keep manual entry and explain the failure.

#### Scenario: Choosing a folder fills the project path
- **WHEN** the operator activates Browse and chooses a folder
- **THEN** the project field holds the returned absolute path

#### Scenario: Cancelling changes nothing
- **WHEN** the operator dismisses the folder dialog
- **THEN** the project field is unchanged and no error is shown

#### Scenario: An unavailable dialog falls back to typing
- **WHEN** the endpoint reports that no folder dialog is available
- **THEN** the project field remains editable and an explanation is shown

### Requirement: The folder dialog endpoint is local-only

The system SHALL expose `POST /api/project/pick`, accepting only requests whose remote address and, when present, Origin are loopback. It SHALL return `200` with the chosen path or a cancellation, `501` when no dialog implementation is available, and `500` when the dialog fails. It SHALL bound the wait with a deadline and SHALL NOT list directories or modify the project.

#### Scenario: A loopback request returns the selected path
- **WHEN** a loopback request completes with a folder selected
- **THEN** the response carries that absolute path

#### Scenario: Cancellation is a normal outcome
- **WHEN** the operator cancels the native dialog
- **THEN** the response reports cancellation without an error

#### Scenario: Remote and foreign-origin requests are refused
- **WHEN** the remote address is not loopback or the request carries a non-loopback Origin
- **THEN** the request is refused

#### Scenario: No dialog on this platform
- **WHEN** the server has no folder-dialog implementation
- **THEN** the response is `501` and the dashboard falls back to manual entry
