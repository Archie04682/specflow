# Tasks

> Sections 2–4, 6.1, 6.3, 7.3–7.7 and part of 3.3 described the tabbed catalogs
> and the flows home. The project-tree/Settings iteration in section 10
> superseded those surfaces; the older tasks are kept as the record of the
> increment that landed before it.

## 1. Design foundation

- [x] 1.1 Replace the `:root` token set with the harness palette, radii, type, spacing, and scrollbar values: flat base/sidebar/elevated surfaces, hairline borders, label steps, one blue accent, near-white primary action, and a 4/8 px rhythm; remove the dot-grid and glow treatments; verify body, `.mono`, and the focus ring render on the new tokens.
- [x] 1.2 Restyle the shared primitives to the new language: buttons (38/30 px, 10/8 px radii, light primary, quiet secondary/ghost, danger), inputs and selects (elevated fill, hairline stroke, accent focus), chips, banners, dialogs, empty states, and scrollbars; verify no component keeps the old periwinkle-on-navy look.
- [x] 1.3 Add the shell layout stylesheet imported after the base sheet: two-track grid, 280/248 px panel tracks, collapsed track, skip link, workspace min-height chain, view header, and responsive drawer/scrim; verify `prefers-reduced-motion` disables the panel and drawer transitions.
- [x] 1.4 Add the missing inline icons needed by the shell (chat, flows, folder, panel-left, chevron, arrow-up, save, stop, play) with the existing 16 px stroke geometry and `aria-hidden` default.

## 2. Shell and control panel

- [x] 2.1 Replace the route model with `home | chat | flows | flow`, map the documented URLs, keep `?view=history` as chat home, and expose which tab a route belongs to; verify parse and serialize round-trips for every view.
- [x] 2.2 Build `AppShell`: skip link, panel track, workspace main, collapsed-state persistence, floating restore control, drawer open/close state, scrim, Escape handling, and focus return; verify the grid does not scroll the page.
- [x] 2.3 Build `ControlPanel`: brand row and collapse control, full-width New chat action, accessible Chat/Flows tab strip with counts, scrolling catalog region, and project footer; verify tab activation navigates and the active tab is derived from the route.
- [x] 2.4 Build `ChatCatalog`: runs grouped Active then Recent, status dot plus label, objective, flow name, relative time, progress for active runs, current-row marking, loading, empty, and error with retry; verify the open run's record is merged so its row follows REST refreshes.
- [x] 2.5 Build `FlowCatalog`: workflow rows with name/reference, description, node and edge counts, validity and problems, current-row marking, loading, empty, error with retry, and a New flow action; verify invalid rows explain themselves.
- [x] 2.6 Build `ProjectDialog` and the footer trigger: current name and path, typed path or native Browse, confirm/cancel, and focus handling; changing the project reloads both catalogs and resets the workspace when the current object cannot belong to it.
- [x] 2.7 Add the narrow-viewport drawer behavior under 900 px: trigger in every view header, scrim, Escape, body scroll lock, and focus return; verify the panel is not a permanent overlay on desktop resize.

## 3. Chat home

- [x] 3.1 Replace the start-screen card with the chat home: greeting, intro copy, and the composer card holding the project control, flow selector, objective, and Start run action; keep the accessible names and the existing start contract.
- [x] 3.2 Carry the current start behaviors into the new composition: catalog loading and invalid options with problems, harness not-ready warning, required objective hint, in-flight disable, `400` keeps the draft and shows the message, and success opens the run.
- [x] 3.3 Add the composer's flow actions: a New flow action beside the selector when the active project has no usable flow, with the selector disabled and explained. (The suggestion shelf and the header Browse flows link were later removed; see 10.4.)
- [x] 3.4 Render the no-project state for the chat home and deep-link prompt inside the workspace, with the inline project picker and the deep link preserved.

## 4. Catalog homes

- [x] 4.1 Build the flows home: catalog header with New flow, description, and a grid of flow cards (name, description, node/edge counts, validity, open action); verify empty, loading, and error states.
- [x] 4.2 Route the New flow action to a blank editor and a catalog row to that reference; verify browser back returns to the flows home.

## 5. Hosted views

- [x] 5.1 Adapt the run view: remove its own app header, render the shared view header with run identity, status, and permitted actions, keep the graph, stream, interactions, cancel-retry-resume, and footer metrics, and make the body fill the workspace; verify every existing run-view test passes against the adapted component.
- [x] 5.2 Adapt the flow editor: remove its app header, render the shared view header with name, file, dirty state, Close, and Save, keep the canvas and inspector height-correct, and preserve the unsaved-changes guard; verify its tests pass.
- [x] 5.3 Move the project prompt for run deep links into the shell workspace and keep the cancel path to the chat home.
- [x] 5.4 Delete the replaced surfaces and dead styles: the old history page, run-list rows, start-screen layout, and app-header/app-footer layout blocks that no view uses any more.

## 6. Verification

- [x] 6.1 Update and extend the test suite: route parsing, App deep-link and tab matrix, control panel tabs and catalogs, chat home start behaviors, flows home, project dialog, collapse persistence, and drawer Escape; verify every suite passes.
- [x] 6.2 Run `make test`, `make build`, and `openspec validate ui-harness-shell --strict`; verify the binary serves the embedded UI.
- [x] 6.3 Make a Chrome pass at 1440x900, 1024x800, and 768x900: verify the panel, tabs, catalogs, chat home, run view, and editor at each width, collapse and restore, drawer open and Escape, focus rings, and that no page-level scrollbar appears; save evidence screenshots under `openspec/changes/ui-harness-shell/evidence/`.
- [x] 6.4 Verify contrast of labels and controls against their surfaces and reduced-motion behavior in the browser; record the measured values.
- [x] 6.5 Update `README.md` (navigation, catalogs, chat home, project control, routes, screenshots) and `AGENTS.md` (the two catalogs, the shell route map, and where shell styles live); verify the documented clicks and URLs match the running UI.

## 7. Multiple projects

- [x] 7.1 Add the project registry: `lib/projects` for normalization, adding, and duplicate-name labels; `lib/storage` for the ordered list plus the migrated active path; unit tests for normalization, suffix selection, migration, malformed data, and fallbacks.
- [x] 7.2 Add project-scoped routes: `?run=&project=` and `?view=editor&edit=&project=`, with parse/serialize round-trip tests and a route-project helper.
- [x] 7.3 Replace the single-project dialog with the projects dialog: known-project list with active marking and duplicate-name suffixes, add-by-typed-path/native Browse, Escape and focus handling; verify selection and addition in tests.
- [x] 7.4 Group the Chat catalog by project: per-project header with active state and New chat action, per-project run read with loading, empty, error/retry, active-first ordering, open run with project, and an Add project row; verify two projects and one failing project in tests.
- [x] 7.5 Group the Flows catalog by project with the same section grammar, per-project workflow read, New flow action, validity, and open flow with project; verify grouped flow tests.
- [x] 7.6 Group the flows home gallery by project section with per-project cards, empty state, New flow, and error/retry; verify the section and active-state tests.
- [x] 7.7 Wire App and Workbench: project list plus active project state, add-and-activate, one-time deep-link adoption, route project resolution, run/flow navigation carrying the project, New chat targeting the active project, and the projects dialog; verify migration and the deep-link matrix in App tests.

## 8. Composer flow affordance

- [x] 8.1 Remove the `Start from a flow` suggestion shelf and its edit/create links; keep only the flow selector, project chip, objective, and start action.
- [x] 8.2 Disable the selector and show `No flows available` when the active project has no usable flow, with a `New flow` action beside it that opens the blank editor in that project; explain the blocked start in the composer hint; verify empty and invalid-only catalogs in tests.

## 9. Iteration verification

- [x] 9.1 Run the full frontend and Go suites and the single-binary build after the multi-project refactor; verify all pass.
- [x] 9.2 Run `openspec validate ui-harness-shell --strict` and `openspec validate --changes --strict`; verify both pass.
- [x] 9.3 Chrome pass at 1440, 1024, and 768 with two projects: verify grouped sections, project switching, add-project dialog, project-scoped run/flow deep links, no-horizontal-overflow, and the composer's New flow state; save evidence under `evidence/`.
- [x] 9.4 Update the README (multi-project panel, project grouping, scoped URLs, composer behavior) and AGENTS.md (registry, grouping, project-scoped routes); verify documented clicks and URLs match the UI.

## 10. Project tree, Settings, and surface removal

- [x] 10.1 Replace the Chat/Flows tab strip with a collapsible project tree: project nodes expand to Chats and Flows leaves, each leaf owns its read only while expanded, leaf create actions are project-scoped, the active project and open object are marked, and duplicate names keep their suffix.
- [x] 10.2 Move the project selector out of the panel foot into a Settings dialog: gear action at the foot, project list with active marking, switch, add by typed/native path, remove from the workbench without touching files, and fallback activation when the active project is removed.
- [x] 10.3 Remove the standalone flows home and its route: flows are browsed and created in the tree, composer, and editor; `?view=flows` lands on the chat home; closing the editor returns home.
- [x] 10.4 Remove the duplicated workspace-header actions (Browse flows, Add project) and align the composer's New flow button content vertically with the flow selector via the shared Button span rule.
- [x] 10.5 Rework the tests: project tree expansion and leaf actions, Settings add/select/remove, removed flows route, editor close, and composer empty-flow alignment; delete the tabbed catalog and flows-home suites.
- [x] 10.6 Run the full suite/build/validation and a Chrome pass at 1440/1024/768 with expanded and collapsed project nodes, Settings add/remove, tree navigation, and the no-flow composer; save evidence under `evidence/`.
- [x] 10.7 Update the README and AGENTS.md for the tree, Settings, removed flows page, and header cleanup.
- [x] 10.8 Enlarge the Chats/Flows leaf labels and their create actions for legibility, and verify the rendered sizes (`evidence/tree-leaves-final-1440.png`: 13 px labels, 28 px actions).

## Follow-up changes

- Chat conversation: turn grouping, message and tool-row styling, composer dock with the interaction cards, and a collapsible run-details sidebar.
- Flow editor interior: canvas, node cards, handles, inspector, and edges in the new language.
- Archive this change after the manual matrix and the two follow-up surfaces are either linked or explicitly deferred.
