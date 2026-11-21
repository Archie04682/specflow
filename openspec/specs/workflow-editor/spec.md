# workflow-editor Specification

## Purpose
Lets an operator author a workflow as a graph: open an existing definition, change its nodes, edges, and settings, validate it, and save it without hand-editing YAML.

## Requirements

### Requirement: A workflow can be opened for editing

The system SHALL expose `GET /api/workflows/{reference}?project=` returning the reference, the resolved source path, the source text, a content hash, and a parsed draft. It SHALL reject a reference that is not a bare filename, return `404` for a reference the project does not have, and report a source it cannot parse as a `400`.

#### Scenario: An existing workflow opens as a draft
- **WHEN** a client requests a workflow the project has
- **THEN** the response carries its source path, source text, content hash, and parsed draft

#### Scenario: An unknown workflow is not invented
- **WHEN** the reference is not in either workflow directory
- **THEN** the response is `404` with a JSON error

#### Scenario: A bad reference never touches a path
- **WHEN** the reference contains a separator, a parent segment, or is otherwise not a bare filename
- **THEN** the response is `400` and no file is read

#### Scenario: An unparseable file is reported
- **WHEN** the file exists but its YAML cannot be parsed
- **THEN** the response is `400` with the parse error and the editor does not open it

### Requirement: A workflow can be created or saved safely

The system SHALL expose `PUT /api/workflows/{reference}?project=` accepting a draft and the hash it was opened with. It SHALL validate the draft structurally, generate canonical YAML, write it atomically to the project copy, and return the written path and hash. A save whose base hash does not match the file it would overwrite SHALL return `409` with the current hash; a structurally invalid draft SHALL return `400` with its problems.

#### Scenario: A new workflow is created
- **WHEN** a client saves a valid draft for a reference with no project copy
- **THEN** `.specflow/workflows/<reference>.yaml` exists and the response carries its path and hash

#### Scenario: An edited workflow replaces its project copy
- **WHEN** a client saves a valid draft with the hash of the current project copy
- **THEN** the project copy is replaced atomically and no other file changes

#### Scenario: A template becomes a project copy
- **WHEN** the client opened a workflow from `workflows/` and saves with that source's hash
- **THEN** the project copy is created and the template is untouched

#### Scenario: A stale save is refused
- **WHEN** the file's hash no longer matches the submitted base hash
- **THEN** the response is `409` with the current hash and nothing is written

#### Scenario: Invalid structure is refused
- **WHEN** the draft has a duplicate node, a dangling edge, a cycle, or a missing agent harness
- **THEN** the response is `400` with the structural problems and nothing is written

#### Scenario: Saving does not touch a run
- **WHEN** a workflow is saved while runs created from it exist
- **THEN** those runs keep their pinned definition

### Requirement: The editor edits a workflow as a graph

The flow editor SHALL render the draft's nodes and edges on a canvas and let the operator add and remove nodes and edges, connect nodes vertically or horizontally, select any node or edge, edit the selected node, rewire an edge, and move nodes. Node positions SHALL be session-only and SHALL NOT be persisted.

#### Scenario: Adding a node updates the canvas
- **WHEN** the operator adds a node
- **THEN** the canvas shows it and the draft contains it with a unique identifier
- **AND** the node is placed at the visible canvas center or the nearest free slot, never overlapping an existing node

#### Scenario: Connecting nodes creates an edge
- **WHEN** the operator connects a source node to a target node, vertically or horizontally
- **THEN** the draft gains that edge unless it already exists or would loop a node to itself
- **AND** the edge is drawn between the handles the operator used; an edge loaded without a handle choice is routed from the sides that face each other

#### Scenario: Selecting another node switches the inspector
- **WHEN** a node is selected and the operator selects a different node
- **THEN** the inspector shows the newly selected node's fields

#### Scenario: Selecting an edge opens its form
- **WHEN** the operator selects an edge on the canvas or an edge row in the inspector
- **THEN** the edge is highlighted and its From/To form is shown

#### Scenario: Changing an edge rewires it
- **WHEN** the operator changes an edge's From or To to another node
- **THEN** the draft replaces that edge, unless the change would self-loop, reference an unknown node, or duplicate an existing edge

#### Scenario: Moving a node changes only the canvas
- **WHEN** the operator drags a node
- **THEN** its canvas position changes without changing the draft topology
- **AND** saving does not persist the position

#### Scenario: The keyboard removes the selection
- **WHEN** a node or edge is selected and Delete or Backspace is pressed outside a form control
- **THEN** the edge is removed immediately, or the node is removed after confirmation, and the selection clears

#### Scenario: Removing a node removes its edges
- **WHEN** the operator removes a node
- **THEN** the node and every edge that named it are gone from the draft

### Requirement: The editor reports save outcomes truthfully

The editor SHALL disable saving while a request is in flight, surface structural problems from a `400`, offer a reload when a `409` reports a changed file, and mark the draft saved only after the server returns the written path and hash.

#### Scenario: Validation problems are shown
- **WHEN** the server rejects a draft as structurally invalid
- **THEN** the editor shows the problems and keeps the draft editable

#### Scenario: A conflict offers a reload
- **WHEN** the server reports that the workflow changed on disk
- **THEN** the editor explains the conflict and offers to reload the current file

#### Scenario: A successful save updates the baseline
- **WHEN** the server accepts a save
- **THEN** the editor records the returned hash and no longer marks the draft dirty

### Requirement: A new workflow starts from a blank draft

The editor SHALL create a new workflow from a blank draft with an empty node list, derive its file reference from the workflow name (lowercase ASCII letters and digits separated by single hyphens), require a non-empty derived reference before saving, and make the reference fixed once the file exists.

#### Scenario: A new workflow derives its file name
- **WHEN** the operator opens the new-workflow editor and names it "Some new flow"
- **THEN** the editor shows `some-new-flow.yaml` and saving writes `.specflow/workflows/some-new-flow.yaml`

#### Scenario: A name without usable characters blocks saving
- **WHEN** the name has no ASCII letter or digit
- **THEN** saving is unavailable until the name changes

#### Scenario: The reference is fixed after creation
- **WHEN** a new workflow has been saved once
- **THEN** changing its name can no longer change its file reference in the editor

### Requirement: The editor authors a command node's routes

The node inspector SHALL let the operator add, change, and remove exit-code-to-label routes on a `command` node. It SHALL offer routes on no other node kind, and a node that stops being a `command` node SHALL lose its routes.

#### Scenario: A gate's routes are edited

- **WHEN** the operator adds the route `1: issues` to a `command` node
- **THEN** the draft's node carries that route and the outgoing edge forms offer `issues`

#### Scenario: A non-command node offers no routes

- **WHEN** the operator selects an `agent` node
- **THEN** the inspector shows no routes editor

#### Scenario: Changing the kind drops the routes

- **WHEN** the operator changes a gate to an `agent` node
- **THEN** the draft's node no longer carries routes

### Requirement: The editor authors an edge's condition

The edge form SHALL offer only the route labels its source node declares, and the canvas SHALL show an edge's condition on that edge. An edge whose source declares no routes SHALL carry no condition, and rewiring an edge to a source that does not declare its label SHALL clear the condition instead of keeping it. Saving SHALL write both fields as canonical YAML and reading a saved workflow SHALL restore them.

#### Scenario: The picker lists the source's labels

- **WHEN** the operator selects an edge whose source declares `clean` and `issues`
- **THEN** the condition picker offers those two labels

#### Scenario: An edge without a routing source carries no condition

- **WHEN** the operator selects an edge whose source declares no routes
- **THEN** the form offers no condition and the draft's edge has none

#### Scenario: Rewiring clears an incompatible condition

- **WHEN** an edge conditioned on `issues` is rewired to a node that does not declare `issues`
- **THEN** the edge's condition is cleared

#### Scenario: The canvas shows the condition

- **WHEN** a draft contains a conditioned edge
- **THEN** the canvas draws its label on that edge without moving any node

#### Scenario: A saved condition reads back

- **WHEN** a workflow with routes and conditioned edges is saved and opened again
- **THEN** the draft carries the same routes and conditions

#### Scenario: An invalid routing draft is refused

- **WHEN** a draft's edge names a label its source does not declare
- **THEN** the save is refused with `400` and the editor shows the routing problems
