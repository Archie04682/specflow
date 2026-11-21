# Spec Delta

## ADDED Requirements

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
