# Spec Delta

## ADDED Requirements

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
