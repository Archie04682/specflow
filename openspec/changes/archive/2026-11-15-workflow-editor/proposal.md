# Proposal

## Why

Workflows are still authored as YAML files by hand. The dashboard can start and monitor them, but creating a node, wiring an edge, or changing a timeout means leaving the product and editing a schema with no feedback until the next run fails to compile. The next product surface is the flow editor: a visual screen that reads a workflow, lets the operator change its graph and node configuration, validates it, and saves it atomically.

## What Changes

- Add a structured read/save API for workflows:
  - `GET /api/workflows/{reference}?project=` returns the source, its content hash, and a parsed draft (nodes, edges, defaults, metadata).
  - `PUT /api/workflows/{reference}?project=` accepts the draft and the hash it was opened with, validates it structurally, generates canonical YAML, and writes it atomically.
- Saving always targets the project copy at `.specflow/workflows/<reference>.yaml`. Editing a workflow that only exists under `workflows/` creates that project copy (resolution precedence already makes it win) and leaves the template untouched.
- Optimistic concurrency: a save whose `baseHash` no longer matches the file returns `409` with the current hash; the editor offers a reload.
- Add the **flow editor** screen: a React Flow canvas with auto-layout, add/remove nodes, connect handles to create edges, an edge list, a full node inspector (kind, harness, skill, model, prompt/command, timeouts, interaction, retry, tools, postcondition), workflow metadata and defaults, validation problems, and a save/conflict flow.
- Add **Edit** and **New workflow** actions to the start screen and an editor route (`?view=editor&edit=<reference>`).
- Document that saved YAML is canonical: comments and formatting in an existing file are not preserved.

**Not included:** drag-persisted node positions, deleting a workflow, editing a structurally invalid file in the visual editor, a workflow template library, or changing a run that already exists (runs stay pinned).

## Capabilities

### New Capabilities

- `workflow-editor`: reading, saving, validating, and visually editing a project workflow as a graph, including its create/save concurrency contract.

### Modified Capabilities

None.

## Impact

- `backend/internal/orchestrator`: the draft model, conversion to and from the existing workflow types, canonical YAML generation, reference safety, hashed reads, validated atomic saves, and tests.
- `backend/internal/api`: `GET`/`PUT /api/workflows/{reference}` and their status/error contract; the existing catalog and run endpoints are unchanged.
- `frontend/src`: the editor page, graph canvas, node inspector, workflow defaults form, edge editing, the draft reducer, route and start-screen actions, and tests.
- `README.md`: editor usage, API rows, and the comment-loss warning.
- No run, event, or persistence changes. A saved workflow only affects runs created after it.
