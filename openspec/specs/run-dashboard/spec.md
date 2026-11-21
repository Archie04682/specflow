# run-dashboard Specification

## Purpose
Gives the operator a live view of a workflow run: start it, watch the graph and event stream advance, intervene when the run asks, control it when it fails, and review it afterwards.

## Requirements

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

The graph SHALL be built from the run's graph projection, preserving the authored nodes and edges, and SHALL NOT infer structure from the run's node records or their order. Each node SHALL show its title and kind, and each conditioned edge SHALL show the label it declares.

#### Scenario: A branch is drawn as a branch

- **WHEN** the run's graph contains a branch and a join
- **THEN** the rendered graph shows those edges instead of a single chain

#### Scenario: A conditioned edge is labelled

- **WHEN** the run's graph carries an edge with a condition
- **THEN** the edge is drawn with that label

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

### Requirement: Thinking is collapsed by default

The activity window SHALL present each thinking block as a collapsed disclosure that names the block and reports its size, SHALL reveal the full text only when the operator expands it, SHALL preserve the operator's expansion choice while the stream re-renders, and SHALL indicate that a collapsed block is still streaming without showing its text.

#### Scenario: A new thinking block is collapsed

- **WHEN** a thinking block arrives in the stream
- **THEN** it is shown collapsed with its size and its body is not rendered

#### Scenario: Expanding reveals the block

- **WHEN** the operator expands a collapsed thinking block
- **THEN** its full text is rendered as markdown

#### Scenario: A streaming block stays collapsed and live

- **WHEN** a collapsed thinking block is still receiving deltas
- **THEN** its row indicates that it is streaming and its body text does not grow the page

#### Scenario: Expansion survives new events

- **WHEN** new events arrive while a thinking block is expanded
- **THEN** that block stays expanded

### Requirement: Activity text renders markdown

The activity window SHALL render assistant messages, thinking bodies, question descriptions and answers, injection text, and notices as markdown, including emphasis, lists, links, fenced code, and tables. It SHALL display raw HTML as text, SHALL NOT fetch remote images, SHALL open links outside the application, and SHALL render errors and command output as preformatted text.

#### Scenario: Markdown structure is rendered

- **WHEN** a message contains a heading, an emphasized phrase, a list, and a fenced code block
- **THEN** each is rendered with its structure instead of its source characters

#### Scenario: Raw HTML is not executed

- **WHEN** activity text contains an HTML tag or script
- **THEN** it is displayed as text and nothing executes

#### Scenario: Images are not fetched

- **WHEN** activity text contains an image reference
- **THEN** the image is not requested and its description is shown instead

#### Scenario: Links open outside the application

- **WHEN** the operator activates a link in activity text
- **THEN** it opens in a new browsing context without access to the application window

#### Scenario: A streaming block renders plain until it settles

- **WHEN** a text or thinking block is still receiving deltas
- **THEN** it renders as preformatted plain text and switches to markdown once it completes

#### Scenario: Errors and output stay verbatim

- **WHEN** a failure error or tool output is rendered
- **THEN** its whitespace and characters are preserved without markdown interpretation

### Requirement: The activity split is resizable

The run view SHALL offer an adjustable separator between the graph and the activity column, SHALL clamp the resulting width so both panes keep a usable minimum, SHALL be operable from the keyboard as well as by pointer, SHALL restore a default width on reset, and SHALL remember the chosen width for later runs in the same browser. The separator SHALL be hidden when the layout stacks.

#### Scenario: Dragging changes the activity width

- **WHEN** the operator drags the separator
- **THEN** the activity column widens or narrows and the graph takes the remaining space

#### Scenario: Clamping keeps both panes usable

- **WHEN** the separator is dragged past a bound
- **THEN** the activity column stops at its minimum or maximum width

#### Scenario: The keyboard adjusts the split

- **WHEN** the separator has focus and the operator presses an arrow key
- **THEN** the activity width changes by a step, and the home and end keys move it to its bounds

#### Scenario: Reset restores the default

- **WHEN** the operator resets the separator
- **THEN** the activity column returns to its default width

#### Scenario: The chosen width is remembered

- **WHEN** the run view is reloaded or a different run opens in the same browser
- **THEN** the activity column opens at the width the operator chose

#### Scenario: The stacked layout hides the separator

- **WHEN** the viewport is too narrow for the side-by-side layout
- **THEN** the graph and activity panes stack and no separator is offered

### Requirement: Tool calls render their input and output

A tool call SHALL render a tool-specific input: a shell prompt line for command tools, a path or range for file tools, a pattern for search tools, and the question for the ask tool. It SHALL render its output as preformatted text with a failure state when the tool failed, SHALL bound the initial height of long output and let the operator reveal the rest, and SHALL render an unrecognized tool as labelled argument rows rather than a JSON blob.

#### Scenario: A command call shows its shell line

- **WHEN** a command tool starts with a shell command
- **THEN** the card shows the command as a prompt line and its output beneath it

#### Scenario: A failed call is marked failed

- **WHEN** a tool finishes with an error
- **THEN** the card reports failure and styles the output as a failure

#### Scenario: Long output is expandable

- **WHEN** a tool's output exceeds the card's initial height
- **THEN** the card shows the bounded output and offers to reveal the rest

#### Scenario: An unrecognized tool lists its arguments

- **WHEN** a tool the dashboard does not know finishes
- **THEN** its arguments are shown as labelled name and value rows

#### Scenario: The ask tool shows the question

- **WHEN** the ask tool runs
- **THEN** its card shows the question's title, description, and options

### Requirement: Commands are visible in the run view

The activity window SHALL show a command node's rendered command while it runs, and its captured output, exit code, and duration when it finishes. The node details drawer SHALL show that command, exit code, and output from the run record after a reload.

#### Scenario: A running command shows its command line

- **WHEN** a command node starts
- **THEN** the activity window shows the rendered command with a running state

#### Scenario: Completion reveals the output and exit code

- **WHEN** a command node finishes
- **THEN** its card shows the captured output, exit code, and duration

#### Scenario: A failed command shows its failure

- **WHEN** a command node fails or times out
- **THEN** its card reports the failure with the captured output and exit code

#### Scenario: A reload restores the command in the drawer

- **WHEN** the operator opens a finished run and selects a command node
- **THEN** the drawer shows its rendered command, exit code, and captured output

### Requirement: A question presents its description

The interaction dock and the activity question card SHALL show a question's description between its title and its controls when the question or the ask tool call that produced it carries one, SHALL render it as markdown, and SHALL NOT present a select question as a title and options alone.

#### Scenario: A select question shows its description above the options

- **WHEN** a select question arrives with a description
- **THEN** the dock shows the title, then the description, then the options

#### Scenario: A description renders markdown

- **WHEN** a question's description contains a list or emphasis
- **THEN** it is rendered with that structure

#### Scenario: A question without a description still renders

- **WHEN** a question carries no description
- **THEN** the dock renders its title and controls without an empty description block

#### Scenario: A replayed question recovers its description

- **WHEN** a pending question's event carries no description but the stream holds the ask tool call that produced it
- **THEN** the dock shows the description from that tool call

### Requirement: The interaction dock leaves the Activity window half the column

The interaction dock SHALL take at most half of the activity column's height, counting the gap between
the panes against the dock, so the Activity window keeps at least the other half. A dock whose content
fits SHALL take only the height it needs, and content that does not fit SHALL scroll within the dock
rather than push the Activity window smaller.

#### Scenario: A tall question stops at the bound

- **WHEN** a pending question is taller than half the activity column
- **THEN** the dock stops at that bound and the question card scrolls to reveal the rest of its content

#### Scenario: The Activity window keeps the other half

- **WHEN** the dock reaches its bound
- **THEN** the Activity window keeps at least the other half of the column instead of collapsing

#### Scenario: A small interaction is not padded

- **WHEN** a pending interaction fits within the bound
- **THEN** the dock takes only the height its card needs and the Activity window takes the rest

#### Scenario: The bound holds when the panes stack

- **WHEN** the viewport is too narrow for the side-by-side layout and the activity column stacks below the graph
- **THEN** the dock is still bounded by half of that activity column

#### Scenario: A short column keeps every control reachable

- **WHEN** the activity column is too short for the card's heading and actions together
- **THEN** the card scrolls as one region and no control or option is clipped out of reach

### Requirement: A question option shows its whole label

A select question SHALL present every option's complete text: the label SHALL wrap onto as many lines as
it needs, SHALL break a long unbroken token instead of overflowing the card, and SHALL NOT be clipped,
truncated, or elided. Choosing a wrapped option SHALL send that option's full text as the answer.

#### Scenario: A long option wraps

- **WHEN** a select question offers an option longer than the dock's width
- **THEN** the option's label wraps onto further lines and its whole text is visible

#### Scenario: An unbroken token breaks

- **WHEN** an option's label contains a long unbroken token such as a path or a URL
- **THEN** the token breaks inside the label instead of running past the card's edge

#### Scenario: A wrapped option sends its whole value

- **WHEN** the operator chooses an option whose label wrapped
- **THEN** that option's full text is sent as the answer

### Requirement: The graph marks the edge that was taken

The graph SHALL mark each edge that was taken, using the route labels recorded on completed nodes and the conditions in the run's graph projection, and SHALL distinguish a taken edge from an edge that was not taken. Marking SHALL NOT move a node.

#### Scenario: The untaken branch is marked

- **WHEN** a gate completed under `clean` and one of its outgoing edges is labelled `issues`
- **THEN** the `issues` edge reads as not taken while the `clean` edge reads as taken
- **AND** no node moves

#### Scenario: An unconditional edge is taken

- **WHEN** an edge declares no condition and its source completed
- **THEN** the edge reads as taken

#### Scenario: A branched run never draws the untaken path as run

- **WHEN** a run completed with a skipped branch
- **THEN** the skipped branch's edges and nodes read as not run rather than as pending
