# Spec Delta

## ADDED Requirements

### Requirement: The workflow catalog lists a project's workflows

The system SHALL expose `GET /api/workflows?project=` returning every `.yaml` workflow file directly under the project's `.specflow/workflows/` and `workflows/` directories, ordered by reference, with a reference that exists in both directories listed once from `.specflow/workflows/`. Each entry SHALL carry the reference accepted by run creation, its path, name, description, version, structural validity, structural problems, nodes, and edges.

#### Scenario: Workflows from both directories are listed
- **WHEN** a project has one workflow under `.specflow/workflows/` and another under `workflows/`
- **THEN** both are returned, ordered by reference

#### Scenario: The project copy wins a duplicate reference
- **WHEN** the same reference exists under both directories
- **THEN** one entry is returned and it names the `.specflow/workflows/` path

#### Scenario: An unusable project is rejected
- **WHEN** the `project` query parameter is absent or the path is not a git repository
- **THEN** the response is `400` with a JSON error body

### Requirement: Workflow catalog reads are structural and side-effect free

The catalog SHALL parse and structurally validate each workflow without compiling it, resolving a skill, probing a harness, or changing the project. A file that cannot be parsed or fails structural validation SHALL still be listed, marked invalid, and carry its reported problems, and SHALL NOT fail the other entries.

#### Scenario: A missing skill does not invalidate a listing
- **WHEN** a workflow binds a skill that does not exist in the project
- **THEN** the workflow is listed as structurally valid

#### Scenario: A broken file is listed with its problem
- **WHEN** a workflow file is not valid YAML
- **THEN** that entry is marked invalid with its error and every other workflow is still returned

#### Scenario: Reading the catalog changes nothing
- **WHEN** the catalog is read
- **THEN** no run, worktree, or project file is created, changed, or removed

### Requirement: A run's graph is read from the definition it was pinned to

The system SHALL expose `GET /api/runs/{id}/graph?project=` returning the run's authored nodes and edges from the workflow definition retained for that run, falling back to the recorded workflow path for a run created before snapshots. Each node SHALL carry its identifier, title, kind, and effective interactive, injection, and approval flags; each edge SHALL name its source and target.

#### Scenario: The pinned definition wins over an edited file
- **WHEN** the project's workflow file is edited after the run is created
- **THEN** the graph reflects the definition retained with the run

#### Scenario: A legacy run falls back to its recorded path
- **WHEN** a run was created before retained snapshots existed
- **THEN** its recorded workflow path supplies the graph

#### Scenario: An unknown run is not found
- **WHEN** the run identifier is unknown to the project
- **THEN** the response is `404` with a JSON error body

### Requirement: Graph reads never compile or resolve

The run graph endpoint SHALL parse the retained definition only. It SHALL NOT compile the workflow, resolve a skill directory, probe a harness, execute a node, emit a run event, or change the run record, so a skill or harness that has disappeared since creation SHALL NOT prevent the graph from being returned.

#### Scenario: A vanished skill does not block the graph
- **WHEN** a skill bound by the run no longer exists
- **THEN** the graph is still returned

#### Scenario: Reading a live run's graph changes nothing
- **WHEN** the graph is read while the run is running
- **THEN** the run's status and last event sequence are unchanged

#### Scenario: An unreadable retained definition is reported
- **WHEN** the retained definition cannot be read or parsed
- **THEN** the response is `500` with a JSON error body
