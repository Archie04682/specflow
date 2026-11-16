# harness-shell Specification

## Purpose
One workbench shell for every SpecFlow view: a persistent project tree, workspace views with a shared header, deep-linkable routes, and a single dark visual language.

## Requirements

### Requirement: A persistent control panel hosts the project tree

The shell SHALL render a persistent left control panel containing the SpecFlow
brand, **New chat**, a project tree, and **Settings**. The tree is the only
catalog navigation: there are no Chat/Flows tabs. Each project expands to
**Chats** and **Flows** leaves listing its runs and workflows. Leaves SHALL
expose loading, empty, and error states; one leaf's failure SHALL NOT hide the
workspace or another leaf.

#### Scenario: A project shows its two leaves
- **WHEN** a project is expanded and has runs and workflows
- **THEN** its Chats and Flows leaves list them

#### Scenario: A collapsed project costs nothing
- **WHEN** a project node is collapsed
- **THEN** its leaves and their reads are not rendered

#### Scenario: A leaf failure stays local
- **WHEN** one project's run or workflow read fails
- **THEN** only that leaf shows the error with a retry

#### Scenario: An empty project invites the first object
- **WHEN** a project has no chats or no flows
- **THEN** the corresponding leaf shows an empty state and its create action

### Requirement: The project tree is navigation

A project row SHALL toggle its expansion and make that project active. A
project's **New chat** leaf action SHALL activate it and open the chat home; its
**New flow** leaf action SHALL open a blank editor in that project. Chat rows
SHALL open their run; flow rows SHALL open their editor. The tree SHALL mark the
active project and the open run or flow without relying on color alone.

#### Scenario: Toggling a project node
- **WHEN** the operator activates a project row
- **THEN** that project becomes active and its leaves toggle

#### Scenario: Opening a chat from the tree
- **WHEN** the operator activates a chat row under a project
- **THEN** that run opens with its owning project

#### Scenario: Creating from a leaf
- **WHEN** the operator activates New chat or New flow on a project's leaf
- **THEN** the action is scoped to that project

### Requirement: A new chat starts from a hero composer

The chat home SHALL present a greeting and a composer card containing the
active project, a flow selector fed by that project's workflow catalog, and an
objective. It SHALL disable flows that cannot be used, show a warning when a
harness is not ready without blocking the form, require an objective, disable
submission while a start is in flight, and keep the form and show the server
message when creation is refused. A successful start SHALL open the new run.

#### Scenario: Starting a run
- **WHEN** the operator selects a valid flow, enters an objective, and starts
- **THEN** the run is created with that project, flow, and objective and opens
  in the workspace

#### Scenario: An unusable flow cannot be selected
- **WHEN** the catalog contains an invalid workflow
- **THEN** its option is disabled and its problems are shown

#### Scenario: A rejected start keeps the draft
- **WHEN** the server refuses run creation
- **THEN** the composer keeps the objective and flow and shows the reason

### Requirement: Settings owns the project registry

The panel foot SHALL show a **Settings** action instead of a project selector.
Settings SHALL list the registered projects with their paths and active state,
allow switching the active project, allow adding a project by typed path or the
operating system's folder dialog, and allow removing a project from the
workbench without deleting files. Removing the active project SHALL activate
the next registered project, or leave no active project when none remain.

#### Scenario: Adding a project
- **WHEN** the operator adds an absolute path in Settings
- **THEN** the path joins the registry, becomes active, and appears in the tree

#### Scenario: Switching projects
- **WHEN** the operator selects a registered project in Settings
- **THEN** it becomes the active project

#### Scenario: Removing the active project
- **WHEN** the operator removes the active project and another remains
- **THEN** the next project becomes active and the removed node leaves the tree

#### Scenario: A deep link without a project
- **WHEN** a run or flow deep link is opened with no registered project
- **THEN** the workspace asks for the project and keeps the deep link

### Requirement: The workbench holds multiple projects

The workbench SHALL persist an ordered project registry and one active
project. Every registered project SHALL appear as a tree node with its own
Chats and Flows leaves and create actions. Projects with duplicate basenames
SHALL be visibly distinguished by the fewest parent-path segments that tell
them apart. A project SHALL be addable from Settings and the empty tree state.

#### Scenario: Two projects remain visible at once
- **WHEN** the registry holds two projects
- **THEN** both appear as tree nodes with their own leaves, even when one
  project's read fails

#### Scenario: Duplicate basenames are distinguished
- **WHEN** two registered projects end in the same folder name
- **THEN** each node shows a parent-path suffix that tells them apart

### Requirement: The panel collapses and adapts to narrow viewports

On desktop the panel SHALL be collapsible, and the workspace SHALL expose a
control to restore it. The collapsed state SHALL persist across reloads. Under
900 px the panel SHALL become an overlay drawer over the workspace with a
scrim; Escape and the scrim SHALL close it. The drawer and collapse controls
SHALL be keyboard reachable and expose their expanded state.

#### Scenario: Collapsing and restoring
- **WHEN** the operator collapses the panel and reloads
- **THEN** the panel stays collapsed and a restore control is visible

#### Scenario: The narrow drawer closes with Escape
- **WHEN** the drawer is open under 900 px and the operator presses Escape
- **THEN** the drawer closes and focus returns to its trigger

### Requirement: Routes and deep links restore the workspace

The URL SHALL encode the chat home, a run, and a flow editor. Opening a URL
SHALL restore the workspace object and the tree's current project, and browser
back and forward SHALL move between them. `?view=history` and the removed
`?view=flows` SHALL both be treated as the chat home.

#### Scenario: A run deep link
- **WHEN** a project-scoped run URL is opened
- **THEN** the run is shown and its project node is current

#### Scenario: An editor deep link
- **WHEN** a project-scoped editor URL is opened
- **THEN** that flow's editor is shown and its project node is current

#### Scenario: An old link still lands somewhere valid
- **WHEN** `?view=history` or `?view=flows` is opened
- **THEN** the chat home is shown

### Requirement: The workspace hosts the existing views with a consistent header

Every workspace view SHALL render a header naming the current object with its
primary actions, and SHALL NOT duplicate tree or Settings navigation (no
Browse-flows or Add-project action). The run view SHALL keep its graph, stream,
interactions, controls, and metrics; the editor SHALL keep its canvas,
inspector, save, conflict, and dirty guards, and closing it SHALL return to the
chat home. Both SHALL fill the workspace and scroll internally.

#### Scenario: Run controls stay available in the shell header
- **WHEN** a running run is open
- **THEN** its status and permitted controls are in the workspace header

#### Scenario: Editor save stays guarded
- **WHEN** a flow has unsaved edits
- **THEN** the header reports the unsaved state and leaving prompts before
  discarding

### Requirement: Flows have no separate index page

Creating and browsing flows SHALL happen inside the project tree, the flow
editor, and the composer; there SHALL be no standalone flows index page or its
Add-project header action.

#### Scenario: Browsing flows
- **WHEN** the operator wants to open a flow
- **THEN** they expand the project's Flows leaf rather than visiting a flows
  page

### Requirement: Run and flow routes carry their project

Run and flow URLs SHALL carry the owning project. Opening such a URL SHALL add
and activate that project, then load the object. The chat home SHALL use the
active project. A deep link with no known project SHALL show the project gate
without dropping the link. Changing the active project SHALL NOT close or
redirect an already open run or flow.

#### Scenario: A project-scoped run deep link
- **WHEN** `?run=<id>&project=<path>` is opened
- **THEN** the project is added and activated and that run opens

#### Scenario: Switching the active project keeps the open object
- **WHEN** a run belonging to project A is open and the operator activates
  project B in Settings or the tree
- **THEN** the run stays open and the tree still marks A's copy as current

### Requirement: The composer offers flow creation when no flow is usable

The chat home SHALL NOT render a flow suggestion shelf. When the active project
has no usable flow, the composer SHALL disable the flow selector, explain that
no flows are available, and offer a **New flow** action aligned with the
selector row that opens a blank editor scoped to the active project. When a
usable flow exists the composer SHALL not duplicate the create action.

#### Scenario: A project without flows
- **WHEN** the active project's catalog has no valid flow
- **THEN** the selector is disabled, "No flows available" is shown, and New
  flow opens the blank editor in that project

#### Scenario: A project with flows
- **WHEN** the active project has at least one valid flow
- **THEN** the composer shows no New flow action and can start a run

### Requirement: The visual system is one dark harness language

The shell SHALL use one dark token set across the panel, workspace, tree,
composer, dialogs, and hosted views: flat near-black surfaces, hairline
borders, soft radii, a single blue accent, and near-white primary actions.
Interactive elements SHALL have visible focus, text and control contrast SHALL
meet WCAG AA for their size, and motion SHALL be removed under reduced-motion.
The shell SHALL NOT use emoji as icons or load icons or fonts from the network.

#### Scenario: Focus is visible from the keyboard
- **WHEN** the operator tabs through the panel and workspace
- **THEN** every interactive control shows a visible focus ring

#### Scenario: Reduced motion
- **WHEN** the system prefers reduced motion
- **THEN** panel, drawer, and hover transitions are not animated

### Requirement: The shell exposes the running server's build

The shell SHALL read the server identity on mount and again when the window regains focus. Settings SHALL show the running build's revision and start time, plus the checkout state when the server reports one. A failed identity read SHALL leave the workbench usable and SHALL NOT surface a blocking error.

#### Scenario: Settings names the running build
- **WHEN** the identity reports a revision and a process start time
- **THEN** Settings shows both for the running server

#### Scenario: An unavailable identity is not an error
- **WHEN** the identity request fails
- **THEN** the shell still renders and no identity error blocks the workbench

#### Scenario: Focus refreshes the identity
- **WHEN** the window regains focus and the server now reports a different start time
- **THEN** Settings shows the new build without a page reload

### Requirement: A stale server build is visible

When the identity reports that the running build is behind the checkout or older than its backend sources, the workbench SHALL show a warning banner naming the running build and telling the operator to restart the server, and SHALL let the operator dismiss it. The banner SHALL NOT appear for a current build, an unavailable identity, or a dismissed warning for the same server process.

#### Scenario: A stale build warns
- **WHEN** the identity reports a checkout ahead of the running build
- **THEN** a warning banner states that the running build is older than the checkout and that the server must restart

#### Scenario: A current build is silent
- **WHEN** the identity reports no staleness
- **THEN** no stale-build banner is shown

#### Scenario: Dismissal is per server process
- **WHEN** the operator dismisses the warning
- **THEN** it stays hidden while that server process is running, and a warning reported by a different server process is shown again

#### Scenario: An unavailable identity is silent
- **WHEN** the identity cannot be read
- **THEN** no stale-build banner is shown
