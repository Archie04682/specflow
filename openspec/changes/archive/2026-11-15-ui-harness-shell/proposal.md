# Proposal

## Why

SpecFlow's UI is a stack of centered page screens: the start screen is a form card, history is a separate page, and the run view and flow editor each paint their own top bar. The product has outgrown that shape. Chats (runs) and flows (workflows) are now two long-lived collections that an operator moves between constantly, and there is no persistent surface that shows either collection while a run or an editor is open.

DeepSeek Harness demonstrates the interaction shape this product wants: a persistent left control panel with the brand, a primary "new" action, and the collections; a workspace area that owns the current object; and a quiet near-black visual system with hairline borders, soft radii, and one blue accent. Rebuilding the shell around that shape makes chats and flows first-class objects instead of routes, and gives the run view and the flow editor a stable home.

This change is the shell and navigation increment. The chat-style run conversation (message bubbles, composer dock, details sidebar) and the flow editor's internal restyle are follow-up changes; this one moves the product onto the new frame without changing run execution, the event contract, or workflow save semantics.

## What Changes

- Replace the top-bar dashboard shell with a persistent left control panel and a workspace area:
  - Brand row, a full-width **New chat** action, a collapsible project tree, and **Settings** in the panel foot.
  - Each project node expands into **Chats** and **Flows** leaves, so the two object types live inside their project instead of in a tab strip.
  - A collapse control on desktop and an off-canvas drawer under 900px, with Escape and a scrim.
- Support **multiple projects** in one workbench: a persisted project registry, project-scoped run and flow URLs, and Settings for adding, switching, and forgetting projects.
- Replace the start-screen card with a **chat home**: a hero greeting and a composer card holding the active project, flow, and objective, with the existing start-run validation, harness readiness warning, invalid-flow handling, and error contract. When the active project has no usable flow, the composer offers **New flow** aligned with the selector instead of a suggestion shelf.
- Remove the standalone **flows home**: browsing and creating flows happen inside the project tree, the editor, and the composer, so the workspace has no flows index page.
- Remove duplicated navigation from the workspace header: no **Browse flows** and no **Add project** action. Those live in the tree and Settings.
- Host the existing run view and flow editor in the workspace with a shared view header: run identity and controls, editor save state and actions. The run graph, event stream, interactions, footer metrics, canvas, and inspector keep their behavior and data flows.
- Adopt a DeepSeek-Harness-derived token set and component language: flat near-black surfaces, hairline borders, 12 px radii for controls and 28 px for the composer, system UI type, one blue accent, near-white primary actions, visible focus, and reduced-motion support.
- Update the route model: `?run=&project=` (chat) and `?view=editor[&edit=][&project=]` (flow). `?view=history` and the removed `?view=flows` land on the chat home.
- Update README and AGENTS.md for the new shell, tree, Settings, and route map.

**Not included:** the chat conversation redesign (message grouping, composer dock, details sidebar), restyling the flow editor canvas and inspector interiors, deleting or renaming workflows, deleting project files, backend changes, or any change to run execution, persistence, event taxonomy, or workflow save concurrency.

## Capabilities

### New Capabilities

- `harness-shell`: the persistent control panel and its project tree, the Settings-managed project registry, collapsing and responsive behavior, the chat home, the tree leaf actions, and the routing and restoration contract that binds the panel to the workspace.

### Modified Capabilities

None. The run and editor behaviors are hosted, not changed; their existing capability changes stay authoritative.

## Impact

- `frontend/src`: a new `Shell` component group (shell frame, control panel, project tree, settings dialog, view header, chat home), catalog hooks, route model changes, small adaptations to `RunView` and `WorkflowEditor`, and restyled tokens and tests.
- `frontend/src/styles`: the shell token set, layout, panel, catalog, hero, and view-header styles, with the existing view styles updated to the new tokens.
- `README.md` and `AGENTS.md`: navigation map, tree and Settings behavior, and the note that chat and flow are leaves under each project.
- No backend, API, persistence, or event changes. `make build` still emits the single binary with embedded assets.
