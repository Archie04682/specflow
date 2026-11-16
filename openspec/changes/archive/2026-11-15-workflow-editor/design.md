# Design

## Context

See `proposal.md` for motivation and `specs/workflow-editor/spec.md` for the
contract. What shapes the approach:

- A workflow is a YAML document (`orchestrator.Workflow`) resolved from
  `<project>/.specflow/workflows/` then `<project>/workflows/`. The catalog
  already reads both without compiling.
- A run is pinned to the source that compiled into it. Saving a workflow must
  not touch an existing run or its retained snapshot.
- `ResolveWorkflowPath` accepts arbitrary paths and even checks the process's
  working directory. That is useful for `POST /api/runs`, but it is the wrong
  primitive for an editor: a reference must be a bare filename resolved only
  inside the project's two workflow directories.
- There is no write API for workflows today. `writeFileAtomic` exists inside the
  orchestrator's store and is not exported.
- The dashboard already has React Flow, the deterministic `layoutGraph`, a
  typed client, and a test harness. The editor reuses them rather than adding a
  second graph stack.

## Goals / Non-Goals

**Goals:**

- A safe, testable read/save contract with explicit validation and conflict
  outcomes.
- A visual editor that can change the graph and every node setting the runtime
  understands, without persisting layout.
- One generated YAML shape, produced by Go, so the editor never hand-writes a
  schema.

**Non-Goals:**

- Preserving comments, ordering, or formatting from the source file.
- Editing a structurally invalid file in the visual editor. A parse failure is
  reported; structural problems are editable because the draft still parses.
- Persisting node positions, deleting workflows, renaming a reference after
  creation, or a template gallery.
- Changing a run. Runs stay pinned; only future runs see a saved workflow.

## Decisions

### D1 — The API transports a structured draft; Go generates the YAML

The client never parses or serializes YAML. `GET` returns both the source (for
transparency and parse errors) and a `draft` JSON object. `PUT` accepts the
draft and the server converts it to `orchestrator.Workflow`, validates it, and
marshals canonical YAML. This keeps one schema in one language and avoids a
client YAML dependency and a second serializer to keep in sync.

The draft mirrors the YAML schema with camelCase JSON names:

```json
{
  "id": "standard-feature",
  "name": "Standard feature",
  "description": "...",
  "version": 2,
  "defaults": {
    "harness": "pi",
    "model": {"id": "default", "thinking": "high"},
    "interactive": false,
    "allowInject": true,
    "askTimeout": "30m",
    "approvalTimeout": "30m"
  },
  "nodes": [
    {
      "id": "explore",
      "title": "Explore",
      "kind": "agent",
      "harness": "pi",
      "skill": "openspec-explore",
      "prompt": "...",
      "command": "",
      "workdir": "run",
      "model": {"id": "default", "thinking": "high"},
      "interactive": true,
      "allowInject": true,
      "requireApproval": false,
      "timeout": "30m",
      "askTimeout": "10m",
      "approvalTimeout": "1h",
      "retry": 0,
      "tools": ["read"],
      "postcondition": {"type": "new_change", "description": "..."}
    }
  ],
  "edges": [{"from": "explore", "to": "propose"}]
}
```

Pointer fields (`interactive`, `allowInject`) are tri-state: absent means "inherit
the workflow default", so the draft must not turn an unset value into `false`.
The YAML file struct uses snake_case keys (`allow_inject`, `ask_timeout`,
`require_approval`, ...) and omits empty values. A model with no reasoning effort
is a scalar (`model: default`); with one it is a mapping. The file begins with two
comment lines stating that the editor manages it and does not preserve comments.

### D2 — Save always targets the project copy

The only writable path is `<project>/.specflow/workflows/<reference>.yaml`. A
workflow that exists only under `<project>/workflows/` is a template: opening it
returns the template's source and hash, and saving with that hash creates the
project copy without touching the template. Resolution precedence already makes
the project copy win for future runs.

This makes the editor safe for the canonical template in a checked-out
repository and avoids rewriting source-controlled files unexpectedly. The GET
response carries `isTemplate` so the editor can state which file will be written.

### D3 — Optimistic concurrency with a content hash

The GET hash is SHA-256 over the exact source bytes. A save compares it with:

- the current project copy when one exists (and refuses a missing base hash,
  so an empty create cannot clobber a file that appeared concurrently);
- the template when no project copy exists and the editor opened a template;
- nothing when the client is creating a new file from scratch (`baseHash: ""`).

A mismatch returns `409 {"error": ..., "hash": "<current>"}` and writes nothing.
The editor offers a reload rather than merging; a three-way merge is not worth
the complexity for a text document the operator can re-open.

### D4 — Reference safety, validation, and atomic writes

- Reference pattern: `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`. No separators, no
  leading dot, no path traversal.
- Resolution checks exactly two paths, in order:
  `<root>/.specflow/workflows/<ref>.yaml`, then `<root>/workflows/<ref>.yaml`.
  It never consults the process working directory.
- The draft converts to `orchestrator.Workflow` and runs `ValidateWorkflow`
  before anything is written. A `ValidationError` becomes
  `400 {"error", "problems"}`.
- The writer creates the directory, writes to a temporary file in the same
  directory, and renames it. A failed write leaves the previous file intact.
- The GET parse failure is `400` with the parse error; an unknown reference is
  `404`.

### D5 — The editor edits the graph, not the pixels

The canvas reuses React Flow and `layoutGraph`. Positions start from the
topology-derived layout; nodes are draggable and moved positions are
session-only — the workflow file records topology, never coordinates. A new
connection remembers the source/target handles the operator used and renders
between them. An edge without a session handle choice (loaded from disk, or
after a reload) is routed by the custom edge from the sides that face each
other, so side-by-side nodes get horizontal edges and stacked nodes vertical
ones. The graph is edited through:

- **Add node** — creates `node-N` (not colliding), title `New node`, kind
  `agent`, at the visible canvas center. If that point is occupied, it walks a
  small ring around it and uses the first slot whose node rectangle does not
  overlap an existing node. A loaded workflow keeps its deterministic layout;
  only later additions use the free-slot placement.
- **Connect** — dragging any source handle (bottom or right) to any target
  handle (top or left) adds an edge unless it is a duplicate or a self-loop.
  The new edge attaches to those exact handles for the session. Handle sides are
  visual intent, not workflow data: the saved file stores only endpoints, so a
  reloaded edge falls back to facing-side routing. Cycles are left to save-time
  validation.
- **Select** — clicking a node or an edge selects exactly one element; clicking
  the pane clears the selection. Selecting a node opens the Node tab; selecting
  an edge highlights it, clears the node, and opens the Edges tab.
- **Canvas gesture** — dragging the empty pane pans. Marquee selection and
  native text selection are disabled, so a Shift+drag cannot leave a large
  selection rectangle or select the page; nodes and edges are selected by click.
- **Edge form** — the selected edge's From/To selects rewire it through
  `replaceEdge`; a self-loop, unknown node, or duplicate is refused with an
  inline error. The form also reverses the edge or deletes it.
- **Delete/Backspace** — removes the selected edge immediately or the selected
  node after confirmation; ignored while focus is in a form control.
- **Canvas failure** — an error boundary around the canvas keeps the header,
  tabs, and inspector usable if a graph render throws, with a retry action.
- **Remove node** — deletes the node and every incident edge.
- **Edge list** — shows endpoint titles (never internal ids); rows select the
  edge, and each row keeps a remove action.
- **Tabbed inspector** — the side panel is split into **Workflow**, **Node**, and
  **Edges** tabs, in that order, so the long forms are not stacked. A new
  workflow opens on Workflow; an existing workflow with nodes opens on Node.
  Only the active tab renders; it owns its scroll container, while the
  file-name/meta header and the tabs stay fixed. Selecting a node switches to
  the Node tab; arrow keys move between tabs.
- **Node inspector** — one column: **Kind** and **Title** first in that order, then
  **Harness** and **Skills** when the node is an agent (node identifiers are
  generated internals and are not shown), followed by execution (model,
  prompt/command, workdir, timeout), interaction (interactive, allow_inject,
  require_approval, ask/approval timeouts, retry), tools, and postcondition.
  The skills picker carries no helper tooltip; only the warning for an
  uninstalled skill is shown. Identifiers are never renamed, so edges cannot
  dangle mid-edit.
- **Workflow settings** — name, description, harness, model, interactive, allow
  inject, ask/approval timeouts. A new workflow derives its file reference from
  the name (lowercase ASCII letters and digits separated by hyphens); the
  operator never edits a reference, workflow id, or version field. The workflow
  id and version are hidden; a new workflow derives its id from the file
  reference. The side header shows a read-only
  document revision derived from the saved content hash (the same value a save
  uses for conflict detection). Once a file exists its reference is fixed.

The page tracks the last saved draft; any difference marks it dirty and warns on
back. Save is disabled while in flight; success records the returned hash and
path; `400` shows the problems; `409` explains the change and offers reload.

### D6 — Route and entry points

`?view=editor&edit=<reference>` opens an existing workflow; `?view=editor` opens
a blank one. The start screen gains **Edit** beside the selected workflow and
**New workflow** in the workflow row. Without a project the editor route falls
back to the start screen so the project field can be set first. No router
dependency is added.

### D7 — Tests

- Orchestrator: draft↔workflow conversion, canonical YAML round-trips
  (draft → YAML → parse → draft), reference rejection, read from both
  directories, template copy, stale hash, create/update, parse and structural
  failures, and no write outside the target.
- API: GET `400`/`404`/`200`; PUT `400` validation (with problems), `409`
  conflict (with hash), `200` create/update, malformed body, missing project.
- Frontend: draft reducer (add/delete node, incident edges, add/remove/rewire
  edge, update fields, blank draft), edge geometry (horizontal, vertical, both
  directions, diagonal dominant axis), route parsing, start-screen entry, and
  the editor page with a mocked graph for load/save/conflict/validation,
  node-switch, edge selection/rewiring, and keyboard deletion.

## Risks / Trade-offs

| Risk | Mitigation |
| :--- | :--- |
| Comments and formatting disappear on save | Stated in the UI and README; the file is generated canonically. A raw-YAML tab is a later addition. |
| A concurrent editor overwrites a change | Content-hash preconditions return `409`; the editor reloads instead of merging. |
| A bad reference escapes the workflow directory | Strict bare-filename pattern and an explicit two-path resolver; no `ResolveWorkflowPath` for the editor. |
| A template is modified unexpectedly | Saves only write the project copy; the template stays untouched and the UI says so. |
| YAML generation drifts from the parser | The same Go structs are converted to and from the draft; round-trip tests parse the generated file with `ParseWorkflow`. |
| An invalid graph is only discovered on save | The server's `ValidateWorkflow` problems are shown; a client-side self-loop/duplicate guard covers the obvious edge mistakes. |
| The editor and run dashboard duplicate graph styling | The editor uses its own small node card but shares `layoutGraph`, tokens, and React Flow setup. |

## Migration Plan

Additive read/save endpoints and a new screen. No persisted format change and no
run migration. Rollback is route removal; existing workflow files remain valid.

## Open Questions

None. Position persistence, comments, and a raw YAML tab are explicit non-goals
for this change, not deferred decisions needed by the implementation.
