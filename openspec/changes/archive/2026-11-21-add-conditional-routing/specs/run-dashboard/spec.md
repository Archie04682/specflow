# Spec Delta

## ADDED Requirements

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

## MODIFIED Requirements

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
