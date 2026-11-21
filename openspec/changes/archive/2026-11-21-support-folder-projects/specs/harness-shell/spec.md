## MODIFIED Requirements

### Requirement: A project that is not ready is introduced with its gaps

The workbench SHALL show an unready project as numbered steps per gap —
repository, commit, flow, skills, harness, uncommitted changes — the current one
expanded with its action, any step selectable, re-read after each. It SHALL
offer git or folder until a mode is recorded, then state it; a folder project
SHALL NOT show the repository or commit gap. It SHALL NOT block navigation or
editing and disappear when no gap remains. Offline it SHALL offer only local
actions and say why the rest are not.

#### Scenario: Adding an unready project

- **WHEN** the operator adds a clean repository with no flow and no skills
- **THEN** the card lists the missing flow and skills, each with an action, and the project is still navigable

#### Scenario: A folder that is not a repository offers initialisation

- **WHEN** the active project is not a git repository
- **THEN** the card states that and offers an action that initialises one

#### Scenario: An initialised repository asks for the first commit

- **WHEN** the project is a repository with no commit
- **THEN** the card states that and shows the command for the first commit without offering to make it

#### Scenario: Setup shows its progress

- **WHEN** the project has several gaps
- **THEN** the card names the current step, counts the steps, and shows the completed ones as summaries

#### Scenario: Any step can be opened

- **WHEN** the operator selects a completed or later step
- **THEN** that step expands and the progress count follows it

#### Scenario: Work done elsewhere is re-read

- **WHEN** the operator re-checks after committing or installing a tool
- **THEN** readiness is read again and the steps update

#### Scenario: The mode choice collapses once recorded

- **WHEN** the operator records a mode
- **THEN** the card states the mode and no longer offers the choice

#### Scenario: A gap is stated once

- **WHEN** the project is not a git repository and the flow catalog cannot be read
- **THEN** the card states the gap and no separate catalog error or retry is shown

#### Scenario: An action updates the card

- **WHEN** the operator uses the card's action to seed the default flow
- **THEN** the card re-scans, drops the flow gap, and keeps any remaining gaps

#### Scenario: A local skills source is offered

- **WHEN** the server is configured with a local skills source
- **THEN** the card's skills action installs from it and reports what was installed and what is still missing

#### Scenario: Uncommitted provisioning is stated

- **WHEN** a provisioning action leaves uncommitted paths
- **THEN** the card shows them and says that committing them is what a run needs next

#### Scenario: Offline mode narrows the actions

- **WHEN** offline mode is active
- **THEN** the card offers only local actions and marks the network ones unavailable with offline mode as the reason

#### Scenario: A ready project shows no card

- **WHEN** the project has a flow, its skills, the tool, the harness, and a clean tree
- **THEN** the card is not rendered

#### Scenario: Choosing folder dismisses the git gaps

- **WHEN** the operator chooses folder for a directory that is not a repository
- **THEN** the card drops the repository and commit rows and keeps the remaining gaps

#### Scenario: A locked mode is stated without a choice

- **WHEN** the project already has a run
- **THEN** the card states the mode and does not offer to change it

### Requirement: A project without a repository offers to create one

The project tree SHALL NOT render a project's Chats and Flows leaves while the
project's workspace mode is git and it is not a git repository. It SHALL render
one row stating that, with an action that initialises a repository, and SHALL
render the leaves once the project is one. A folder project SHALL render its
leaves, since no repository is needed. The row SHALL NOT block activating the
project or collapsing its node.

#### Scenario: A folder that is not a repository

- **WHEN** the operator expands a git project that is not a git repository
- **THEN** no Chats or Flows leaves are rendered and a Create git repository action is offered

#### Scenario: Creating the repository reveals the leaves

- **WHEN** the operator activates the action
- **THEN** a repository is initialised and the leaves replace the action

#### Scenario: A repository keeps its leaves

- **WHEN** the project is a git repository
- **THEN** the leaves render as before

#### Scenario: A folder project keeps its leaves

- **WHEN** the project's workspace mode is folder and the directory is not a repository
- **THEN** the Chats and Flows leaves render and no initialise action is offered
