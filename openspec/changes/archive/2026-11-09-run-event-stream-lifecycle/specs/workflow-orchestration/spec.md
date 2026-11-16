# Spec Delta

## ADDED Requirements

### Requirement: Run event streams end when the run terminates

When a run reaches `completed`, `failed`, or `cancelled`, the system SHALL publish the terminal event, deliver every event already retained for the run, and end every open event stream. A subscriber that connects after termination SHALL receive the retained events and then EOF. Resuming or retrying the run SHALL open a new live epoch on the same sequence.

#### Scenario: Live stream ends after the terminal event
- **WHEN** a run reaches a terminal status while a client is subscribed
- **THEN** the client receives the terminal event and the stream ends without waiting for another event

#### Scenario: Late subscriber receives history and EOF
- **WHEN** a client subscribes after the run has terminated
- **THEN** it receives the retained events in sequence order and the stream ends

#### Scenario: Resume opens a new live epoch
- **WHEN** a failed or cancelled run is resumed or retried
- **THEN** new subscribers receive the retained history and then live events from the new execution

### Requirement: Slow event subscribers are shed without blocking execution

A subscriber that cannot keep up with the live stream SHALL be disconnected rather than block event publication or node execution. A subscriber that reconnects with the last sequence number it received SHALL receive every later event in order.

#### Scenario: Slow subscriber is disconnected
- **WHEN** a subscriber's buffer is full because it is not reading
- **THEN** that subscriber's stream ends and the engine continues publishing without waiting for it

#### Scenario: Other subscribers are unaffected
- **WHEN** one subscriber is disconnected for being slow
- **THEN** every other subscriber continues to receive events in order

#### Scenario: Shed subscriber catches up
- **WHEN** a disconnected subscriber reconnects with its last received sequence number
- **THEN** it receives every later retained event and no earlier one

### Requirement: A terminal event stream stops reconnecting at the tail

For a terminal run, the system SHALL answer a client at the tail with `204 No Content` and no body, a client behind the tail with `200` plus the later events then EOF, and a client without a cursor with the full retained stream then EOF. An unconditional `410` would make a page reload lose history and SHALL NOT be used.

#### Scenario: Cursor at the terminal tail
- **WHEN** a terminal run's last event has already been received and the client reconnects with that sequence number
- **THEN** the server responds `204 No Content` and the client stops reconnecting

#### Scenario: Cursor behind the terminal tail
- **WHEN** a terminal run has later events than the client's last sequence number
- **THEN** the server responds `200` with those later events and then ends the stream

#### Scenario: Fresh client reads the full history
- **WHEN** a client connects to a terminal run without a cursor
- **THEN** the server responds `200` with every retained event and then ends the stream

### Requirement: An active event stream sends heartbeats while idle

While a run is active and no event is available, the system SHALL send a heartbeat on the event stream at a bounded interval so an idle connection is not closed by an intermediary.

#### Scenario: Idle stream remains open
- **WHEN** a run is active and no event is published for the configured heartbeat interval
- **THEN** the client receives a heartbeat and the stream remains open

### Requirement: Transition events expose lifecycle payloads

Run events SHALL keep dot-separated type names and a stable envelope of sequence, timestamp, type, run identifier, optional node identifier, and data object. Transition events SHALL carry the fields a client needs to render progress: workflow identifier and objective on `run.started`; attempt on `node.started`; error and retryability on `node.failed` and `node.postcondition_failed`; postcondition outcome on `node.completed`; and summary on `run.completed`.

#### Scenario: Run start identifies the workflow and objective
- **WHEN** a run starts
- **THEN** its `run.started` event carries the workflow identifier and the objective

#### Scenario: Node start carries the attempt
- **WHEN** a node begins an attempt
- **THEN** its `node.started` event carries the attempt number

#### Scenario: Node failure carries retryability
- **WHEN** a node fails, including a postcondition failure
- **THEN** its failure event carries the error text and whether the failure is retryable

#### Scenario: Node completion carries the postcondition outcome
- **WHEN** a node completes
- **THEN** its `node.completed` event carries whether the postcondition held and any resolved change

#### Scenario: Run completion carries a summary
- **WHEN** a run completes
- **THEN** its `run.completed` event carries a summary of the execution

### Requirement: The event journal is repaired after an unclean shutdown

On open, the system SHALL discard a partial trailing record, SHALL report a record that exceeds its read limit instead of silently truncating replay, and SHALL report an append failure. A repaired journal SHALL continue sequence numbering without reusing a retained number.

#### Scenario: Partial trailing record is discarded
- **WHEN** the journal ends with a truncated JSON record after a crash
- **THEN** that record is not replayed and new events append after the last complete record

#### Scenario: Oversized record fails loudly
- **WHEN** a journal record exceeds the configured read limit
- **THEN** opening the journal reports the failure instead of silently returning a shorter history

#### Scenario: Append failure is reported
- **WHEN** writing a new event to the journal fails
- **THEN** the failure is reported rather than ignored

### Requirement: Terminal runs release live stream resources

When a run reaches a terminal status, the system SHALL release the resources it held
to stream that run, including its live subscriber set and its open event journal. A
later replay SHALL still return the retained events, and a resumed or retried run
SHALL continue the same sequence.

#### Scenario: Terminal run releases its stream resources
- **WHEN** a run reaches a terminal status and leaves the live set
- **THEN** no live subscriber remains and the run's journal is no longer held open

#### Scenario: Replay after release still returns history
- **WHEN** a client requests events for a terminal run that is no longer in the live set
- **THEN** the server reads the journal and returns the retained events in sequence order

#### Scenario: Resume reopens the journal
- **WHEN** a failed or cancelled run is resumed or retried after release
- **THEN** its journal is reopened and new events continue from the retained sequence
