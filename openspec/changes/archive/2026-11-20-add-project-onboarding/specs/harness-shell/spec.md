# Spec Delta

## ADDED Requirements

### Requirement: A project that is not ready is introduced with its gaps

The workbench SHALL surface a project that is not ready as a card stating each gap — repository, commit, flow, skills, tool, harness, and uncommitted provisioning — with an action where one exists and a re-scan after each action. It SHALL NOT block navigation, switching projects, or opening the editor, and it SHALL disappear once the project has no gaps. In offline mode it SHALL offer only the actions that need no network and state why the others are unavailable.

#### Scenario: Adding an unready project

- **WHEN** the operator adds a clean repository with no flow and no skills
- **THEN** the card lists the missing flow and skills, each with an action, and the project is still navigable

#### Scenario: A folder that is not a repository offers initialisation

- **WHEN** the active project is not a git repository
- **THEN** the card states that and offers an action that initialises one

#### Scenario: An initialised repository asks for the first commit

- **WHEN** the project is a repository with no commit
- **THEN** the card states that and shows the command for the first commit without offering to make it

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

### Requirement: A project without a repository offers to create one

The project tree SHALL NOT render a project's Chats and Flows leaves while the project is not a git repository. It SHALL render one row stating that, with an action that initialises a repository, and SHALL render the leaves once the project is one. The row SHALL NOT block activating the project or collapsing its node.

#### Scenario: A folder that is not a repository

- **WHEN** the operator expands a project that is not a git repository
- **THEN** no Chats or Flows leaves are rendered and a Create git repository action is offered

#### Scenario: Creating the repository reveals the leaves

- **WHEN** the operator activates the action
- **THEN** a repository is initialised and the leaves replace the action

#### Scenario: A repository keeps its leaves

- **WHEN** the project is a git repository
- **THEN** the leaves render as before


## MODIFIED Requirements

### Requirement: The composer offers flow creation when no flow is usable

The chat home SHALL NOT render a flow suggestion shelf. When the active project
has no usable flow, the composer SHALL disable the flow selector, explain that
no flows are available, and offer a **New flow** action aligned with the
selector row that opens a blank editor scoped to the active project. When a
usable flow exists the composer SHALL not duplicate the create action. A project
that is not a git repository or has no commit SHALL NOT render the composer,
because no flow can start there.

#### Scenario: A project without flows

- **WHEN** the active project's catalog has no valid flow
- **THEN** the selector is disabled, "No flows available" is shown, and New
  flow opens the blank editor in that project

#### Scenario: A project with flows

- **WHEN** the active project has at least one valid flow
- **THEN** the composer shows no New flow action and can start a run

#### Scenario: A project that cannot start a run

- **WHEN** the active project is not a repository, or is a repository with no commit
- **THEN** the composer is not rendered and the setup surface stands in its place
