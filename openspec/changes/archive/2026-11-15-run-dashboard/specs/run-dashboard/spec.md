# Spec Delta

## Purpose

Gives the operator a live view of a workflow run: start it, watch the graph and event stream advance, intervene when the run asks, control it when it fails, and review it afterwards.

## ADDED Requirements

### Requirement: A run starts from a project, workflow, and objective

The dashboard SHALL require a project path, a workflow reference, and a non-empty objective before starting a run, SHALL create the run through the existing create endpoint, and SHALL enter the run's active view on success. A rejected start SHALL leave the form populated and show the server's error message.

#### Scenario: A complete form starts a run
- **WHEN** the operator submits a project, a workflow, and an objective and the server creates the run
- **THEN** the dashboard opens that run's active view with its graph and event stream

#### Scenario: A rejected start keeps the form
- **WHEN** the server rejects the create request with an error message
- **THEN** the message is shown, the form keeps its values, and no run view is entered

#### Scenario: A created run that failed to start is still visible
- **WHEN** the create response carries a run together with a start error
- **THEN** the dashboard opens that run and shows the error

### Requirement: The dashboard holds the project root in the browser

The project root SHALL be an absolute path entered once and retained across reloads; every request SHALL carry it in the `project` parameter, and the dashboard SHALL NOT depend on a server-owned project. A run identifier in the URL SHALL reopen that run when a project is known.

#### Scenario: The project survives a reload
- **WHEN** the operator reloads the dashboard after entering a project
- **THEN** the project is restored without retyping

#### Scenario: A deep link reopens a run
- **WHEN** the URL names a run and a project is retained
- **THEN** the dashboard opens that run

#### Scenario: A deep link without a project asks for one
- **WHEN** the URL names a run and no project is retained
- **THEN** the dashboard asks for a project instead of failing silently

### Requirement: The graph shows the workflow's authored topology

The graph SHALL be built from the run's graph projection, preserving the authored nodes and edges, and SHALL NOT infer structure from the run's node records or their order. Each node SHALL show its title and kind.

#### Scenario: A branch is drawn as a branch
- **WHEN** the run's graph contains a branch and a join
- **THEN** the rendered graph shows those edges instead of a single chain

#### Scenario: An unavailable graph does not hide the stream
- **WHEN** the graph projection cannot be read
- **THEN** the run view still shows the event stream and states that the graph is unavailable

### Requirement: Node visuals track execution state

Each node SHALL show its state among pending, running, completed, failed, waiting for approval, waiting for an answer, cancelled, skipped, or not run. Waiting for an answer SHALL be derived from the run's pending interaction because the node itself remains running, and a node still pending in a terminal run SHALL read as not run.

#### Scenario: A running node is marked running
- **WHEN** a node is executing
- **THEN** the graph marks it running

#### Scenario: A question marks its node as waiting
- **WHEN** the run is waiting for an answer to a node's question
- **THEN** that node reads as waiting for an answer

#### Scenario: Never-run nodes read as not run
- **WHEN** a run reaches a terminal state with nodes that never started
- **THEN** those nodes read as not run rather than pending

#### Scenario: A failed node stays selectable
- **WHEN** a node fails
- **THEN** the graph marks it failed and keeps it selectable

### Requirement: A failed node exposes its failure and retry

Selecting a failed node SHALL show its recorded error, failure class, attempt count, and checkpoint. The dashboard SHALL offer retry in forward or clean mode, SHALL send the node and mode, and SHALL surface a refusal as a refusal rather than as a started retry.

#### Scenario: Failure details are shown
- **WHEN** the operator selects a failed node
- **THEN** its error, failure class, attempts, and checkpoint are shown

#### Scenario: A forward retry resumes the view
- **WHEN** the operator retries a failed node forward and the server accepts
- **THEN** the node returns to running and live events resume

#### Scenario: A refused retry is reported
- **WHEN** the server refuses the retry because the run or node is not failed
- **THEN** the refusal is shown and the run state is refreshed

### Requirement: Live updates arrive over SSE without polling

The dashboard SHALL receive live updates from the run's SSE endpoint, SHALL replay retained events when it first opens a run, and SHALL NOT poll run endpoints for updates. It SHALL treat a terminal event as the end of the stream, a `204` as an already-finished stream, and a dropped transport as a connection status rather than a lost run.

#### Scenario: Opening a run replays what happened
- **WHEN** the dashboard opens a run that has already emitted events
- **THEN** the retained events are rendered before new ones

#### Scenario: A terminal run stops streaming
- **WHEN** the run reaches a terminal status
- **THEN** the dashboard closes the stream and does not reconnect

#### Scenario: A reconnect does not duplicate events
- **WHEN** the transport drops and reconnects
- **THEN** events already rendered are not rendered twice

#### Scenario: A finished tail is reported as ended
- **WHEN** the server answers the stream request with `204`
- **THEN** the dashboard reports the stream as ended

### Requirement: Events render according to their kind

The dashboard SHALL render the lifecycle events, thinking and assistant text, tool calls, file changes, usage, notices, and status lines from the real event taxonomy. Token deltas SHALL be combined into text blocks so the number of rendered items grows with turns and tool calls, not with tokens.

#### Scenario: Thinking deltas become one block
- **WHEN** a sequence of thinking deltas is followed by its completion
- **THEN** one thinking block is shown and it matches the completed text

#### Scenario: A tool call is one card
- **WHEN** a tool starts, reports updates, and finishes
- **THEN** one card shows its final summary and any file it changed

#### Scenario: Long streams stay bounded
- **WHEN** a run emits thousands of token deltas
- **THEN** rendered items grow with turns and tool calls instead of tokens

#### Scenario: Unknown kinds are not invented
- **WHEN** an event kind the dashboard does not know arrives
- **THEN** it is shown as an unrecognized event rather than as another kind

### Requirement: The event stream follows new events unless the user scrolls up

The stream SHALL auto-scroll to the newest event while the view is at the bottom, SHALL stop auto-scrolling when the operator scrolls up, and SHALL let the operator return to the live position.

#### Scenario: New events keep the newest in view
- **WHEN** an event arrives while the view is at the bottom
- **THEN** the stream scrolls to show it

#### Scenario: Scrolling up is respected
- **WHEN** the operator scrolls up and a new event arrives
- **THEN** the scroll position does not change

#### Scenario: Returning to the bottom resumes following
- **WHEN** the operator returns to the bottom of the stream
- **THEN** auto-scroll resumes

### Requirement: A pending question is answered according to its kind

The interaction dock SHALL render a pending question as free text, single select, confirmation, or editor according to the kind reported by its question event, SHALL send a value for text and select answers and a confirmed flag for confirmation answers, SHALL offer cancellation, and SHALL enable controls only once the kind is known.

#### Scenario: A confirmation sends its boolean
- **WHEN** a confirmation question is pending
- **THEN** the dock offers yes and no and sends the confirmed flag

#### Scenario: A select sends its option
- **WHEN** a select question is pending
- **THEN** its options are offered and the chosen option is sent as the value

#### Scenario: A reload recovers the question kind
- **WHEN** the dashboard reloads while a question is pending
- **THEN** the kind is recovered from the replayed stream before controls are enabled

#### Scenario: A resolved question closes the dock
- **WHEN** an answer is refused because the question already resolved
- **THEN** the dock closes and the run record is refreshed

### Requirement: A clarification is queued into the running node

While a node is running, the dock SHALL allow a clarification to be sent, SHALL report it as queued rather than delivered, and SHALL show the acknowledgement or rejection that follows as events.

#### Scenario: An accepted clarification is queued
- **WHEN** the operator sends a clarification during a run and the server accepts it
- **THEN** the dashboard reports it as queued

#### Scenario: A refused clarification is explained
- **WHEN** no node is running, the node forbids injection, or the harness cannot accept it
- **THEN** the refusal reason is shown

### Requirement: An approval gate is decided by node and attempt

The dock SHALL show a pending approval's node, action, attempt, and deadline, SHALL send a decision for that node and attempt, and SHALL state that rejecting ends the run cancelled.

#### Scenario: The gate shows what will run
- **WHEN** an approval is pending
- **THEN** the dock shows the action the approval would start

#### Scenario: Approval starts the node
- **WHEN** the operator approves the pending gate
- **THEN** the gate closes and the node proceeds

#### Scenario: Rejection ends the run cancelled
- **WHEN** the operator rejects the pending gate
- **THEN** the run ends cancelled and the node is not started

### Requirement: Run controls act only when the run permits

The dashboard SHALL offer cancellation while a run is running, resume for a failed or cancelled run, and retry only for a failed node. It SHALL confirm cancellation, SHALL surface refusals, and SHALL reopen the event stream after a retry or resume so subsequent events are received.

#### Scenario: Cancellation is confirmed
- **WHEN** the operator cancels a running run
- **THEN** a confirmation is required before the request is sent

#### Scenario: A retry reopens the stream
- **WHEN** a retry or resume is accepted
- **THEN** the stream reconnects after the last received event

#### Scenario: A refused control is reported
- **WHEN** a control is refused because the run's state does not permit it
- **THEN** the refusal is shown and the run is refreshed

### Requirement: Past runs are listed and reviewable

The dashboard SHALL list the project's runs newest first with status, workflow, objective, start time, and node progress, SHALL open a selected run with its graph and full event history, and SHALL show its final status, per-node results, checkpoints, and error when present.

#### Scenario: History is newest first
- **WHEN** the operator opens run history
- **THEN** the project's runs are listed newest first

#### Scenario: A completed run can be reviewed
- **WHEN** the operator selects a completed run
- **THEN** its graph, events, final status, and per-node results are shown

#### Scenario: A failed run offers recovery
- **WHEN** the operator selects a failed run
- **THEN** its error and the applicable retry or resume control are shown

### Requirement: Failures and in-flight work are visible

The dashboard SHALL surface network failures, non-success HTTP responses, and malformed responses as visible messages, SHALL show a loading state for asynchronous work, and SHALL prevent duplicate submissions while an action is in flight.

#### Scenario: A network failure is retryable
- **WHEN** a request fails before reaching the server
- **THEN** a visible message offers a retry

#### Scenario: An API error keeps its message
- **WHEN** the API returns an error body
- **THEN** that message is shown to the operator

#### Scenario: An action cannot be submitted twice
- **WHEN** a start or interaction request is in flight
- **THEN** its control is disabled until the request settles

### Requirement: Presentation is dark, responsive, and accessible

The dashboard SHALL default to a dark theme, SHALL keep the graph, stream, and controls usable at desktop and tablet widths, SHALL convey state without relying on color alone, and SHALL honor the reduced-motion preference.

#### Scenario: First load uses the dark theme
- **WHEN** the dashboard loads for the first time
- **THEN** it renders in the dark theme

#### Scenario: Tablet widths keep every control reachable
- **WHEN** the viewport is tablet width
- **THEN** the graph, stream, dock, and controls remain reachable

#### Scenario: Reduced motion is honored
- **WHEN** the operator requests reduced motion
- **THEN** running-node animation is suppressed
