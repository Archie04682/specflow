# Tasks

## 1. Draft model and canonical YAML

- [x] 1.1 Add the exported draft types (`WorkflowDraft`, defaults, node, edge) and conversion to and from `orchestrator.Workflow`, including the tri-state interactive/inject pointers and the model scalar/mapping; verify conversion tests for an empty draft, a full draft, and an unset-vs-false boolean.
- [x] 1.2 Add canonical YAML generation with the managed-file header and snake_case keys, plus a round-trip test that marshals a draft, parses it with `ParseWorkflow`, validates it, and converts back to the same draft.

## 2. Read and save

- [x] 2.1 Add `ReadWorkflowSource`: strict reference validation, resolution only under `.specflow/workflows/` then `workflows/`, SHA-256 hash, parsed draft, `isTemplate`, and typed not-found/invalid-reference/parse errors; verify tests for both directories, an unknown reference, a traversal reference, and an unparseable file.
- [x] 2.2 Add `SaveWorkflowSource`: convert and `ValidateWorkflow`, target `.specflow/workflows/<ref>.yaml`, create/update, template-copy authorization by base hash, conflict on a stale or missing base, atomic write, and returned path/hash; verify tests for create, update, template copy, stale hash, empty-base-over-existing, structural problems, and that a failed save leaves the old file byte-identical.
- [x] 2.3 Add `GET`/`PUT /api/workflows/{reference}` with the documented statuses and bodies, register them, and document the endpoints in the README API table; verify `httptest` coverage for every status and that a run created before a save is unchanged.

## 3. Frontend contract and routing

- [x] 3.1 Add the draft types and `getWorkflowSource`/`saveWorkflow` API calls; verify client tests for the GET response, the PUT body (`draft`, `baseHash`), and error statuses.
- [x] 3.2 Add the editor draft reducer: blank draft, unique node ids, add/remove node with incident edges, add/remove edge with duplicate and self-loop guards, field updates, dirty comparison; verify unit tests for each operation.
- [x] 3.3 Extend the route to `{view: "editor", reference?}`, add Edit/New actions to the start screen, and fall back to the start screen when the editor has no project; verify route and start-screen tests.

## 4. Editor screen

- [ ] 4.1 Build the interactive editor canvas and node card: auto-layout from the draft, selectable nodes and edges, connect handles on both axes, draggable nodes, edge rewiring form, Delete/Backspace removal, and add-node/edge-list controls; verify model, geometry, and page tests and a manual pass for select/connect/move/rewire/delete.
- [x] 4.2 Build the tabbed Workflow/Node/Edges inspector (Workflow first) and workflow settings form covering every runtime field, hiding internal node ids, the workflow id, and the version (a read-only document revision is shown), deriving a new file name from Name; stack Kind, Title, Harness, and Skills in one column (Kind first) and keep the skills picker free of helper tooltips; verify tests that switch tabs, edit fields, assert the resulting draft and field order, confirm the derived filename and reference, and confirm no id or version field is exposed.
- [x] 4.3 Build the editor page: load or blank, derive a new workflow's file reference from its Name, dirty tracking and back warning, save in flight, `400` problems, `409` conflict with reload, success baseline update, and the template-copy notice; verify tests for load, new, save, validation, and conflict.
- [ ] 4.4 Add the editor layout and responsive rules to the stylesheet, including a fixed inspector header/tabs and a scrollable active tab panel; verify the canvas and inspector remain usable at desktop and tablet widths and every field is reachable by scrolling.

## 5. Integration

- [x] 5.1 Document the editor, the save target, and the comment-loss warning in the README; verify the described actions match the screen.
- [ ] 5.2 Run `make test` and `make build`, serve the binary, create a workflow through the editor, save it, and confirm it appears in the start screen catalog and can start a run; record the result.
- [x] 5.3 Run `openspec validate workflow-editor --strict` and `openspec validate --changes --strict`; verify both pass.

## Workflow follow-up

- Archive the change after a real create/edit/save round trip and a run started from the saved workflow are recorded.
- Keep the canonical-YAML and comment-loss behavior documented for anyone editing workflow files outside the UI.
