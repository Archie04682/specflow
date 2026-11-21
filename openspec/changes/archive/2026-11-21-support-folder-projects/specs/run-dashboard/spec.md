## MODIFIED Requirements

### Requirement: Run controls act only when the run permits

The dashboard SHALL offer cancellation while a run is running, resume for a
failed or cancelled run, and retry only for a failed node. For a completed
folder run whose changes are not applied it SHALL offer applying and discarding
them; a failed or cancelled folder run SHALL offer discarding. It SHALL confirm
the discard and report what applying wrote and deleted. It SHALL surface
refusals, and SHALL reopen the event stream after a retry or resume so
subsequent events are received.

#### Scenario: Cancellation is confirmed

- **WHEN** the operator cancels a running run
- **THEN** a confirmation is required before the request is sent

#### Scenario: A retry reopens the stream

- **WHEN** a retry or resume is accepted
- **THEN** the stream reconnects after the last received event

#### Scenario: A refused control is reported

- **WHEN** a control is refused because the run's state does not permit it
- **THEN** the refusal is shown and the run is refreshed

#### Scenario: A folder run offers apply and discard

- **WHEN** the operator opens a completed folder run that is not applied
- **THEN** applying and discarding are offered and the run says its changes are not in the project yet

#### Scenario: Applying reports its files

- **WHEN** an apply succeeds
- **THEN** the written and deleted paths are shown and the run reports that it was applied

#### Scenario: A failed folder run can be discarded
- **WHEN** the operator opens a failed folder run
- **THEN** discarding is offered, because the run still holds the project's copy

#### Scenario: Discarding is confirmed

- **WHEN** the operator discards a folder run's changes
- **THEN** a confirmation is required before the request is sent
