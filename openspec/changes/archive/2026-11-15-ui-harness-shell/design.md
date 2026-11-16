# Design

## Context

See `proposal.md` for motivation and `specs/harness-shell/spec.md` for the
contract. What shapes the approach:

- The installed DeepSeek Harness application was inspected as the visual and
  structural reference. Its UI packages define the token set, the frame
  geometry, and the component language this change adopts: base
  `rgb(21 21 23)`, elevated layers `rgb(35 35 36)` / `rgb(44 44 46)` /
  `rgb(53 53 56)`, sidebar `rgb(27 27 28)`, labels
  `rgb(245 246 247)` / `rgb(207 211 214)` / `rgb(173 178 184)` /
  `rgb(129 133 140)`, one blue accent `rgb(86 134 254)`, hairline borders at
  6–20% white, radii 4/8/12/16/20/28, 14 px UI type, a 38 px new-session
  control at radius 12, and a 28 px-radius composer card. Only the values are
  borrowed; no DSH code, font, icon, or asset is copied.
- The existing run view and flow editor are data-complete. Rebuilding them
  inside this increment would delay the shell and risk the event projection
  and editor save contract. They are hosted, not rewritten.
- `ui-ux-pro-max` was consulted for a product-wide direction; its generic
  output did not match the harness reference, so this design uses the verified
  DSH token values plus the skill's general checks (focus visibility,
  contrast, reduced motion, 4/8 px rhythm, touch targets).
- The frontend is a single embedded bundle; no runtime font or icon network
  request is acceptable.

## Goals / Non-Goals

**Goals:**

- A persistent shell with a project tree that stays mounted while the
  workspace changes object.
- A new-chat composer and Settings dialog that look like one product with the
  rest of the shell.
- Routes and deep links that restore both the selected object and the tree's
  active project.
- A documented token set so the follow-up view restyles converge on one
  language.

**Non-Goals:**

- Chat message/turn grouping, composer-dock interaction redesign, or a details
  sidebar for the run view.
- Restyling the flow editor canvas, node cards, and inspector interiors.
- Deleting project files, multiple projects open at once, auth, light theme,
  or changing any backend contract.

## Decisions

### D1 — The project tree replaces the Chat/Flows tab strip

Chats and flows are different object types with different verbs, but they
share one owner: the project. A tab strip made the user pick a type before
picking an object and implied a landing page per type. The tree instead mirrors
the data model: project → **Chats** and **Flows** leaves → objects. A project
node expands in place and its leaves load only while it is open.

The tree is the only catalog navigation. The row's state is derived from the
route (the open object's project), never stored separately, so a deep link and
the tree cannot disagree. A project row toggles expansion and activates the
project; each leaf owns its own create action, which is also scoped to that
project. Removing the flows index removes a page that only existed to host a
tab.

### D2 — Routes keep their existing spellings

| Workspace | URL |
| --- | --- |
| Chat home | `/` |
| Chat session | `?run=<id>&project=<path>` |
| Flow editor | `?view=editor&edit=<reference>&project=<path>` (no `edit` = new) |

`?view=history` and the removed `?view=flows` both parse as the chat home so
old links keep working. There is no separate history page and no flows index:
the Chat and Flows leaves under each project are those surfaces.

### D3 — The shell is a two-track CSS grid with a collapsed state

```text
shell
├── control panel (280px | 248px | drawer)
└── workspace
    ├── view header (56px, owned by the active view)
    └── view body (scrolls or fills)
```

The shell stores nothing but the panel's collapsed flag in `localStorage`.
Collapsing hides the panel track and publishes a floating reveal control in the
workspace; every view header reserves inline space for it through a data
attribute on the root. Under 900 px the panel leaves the grid and becomes an
overlay drawer with a scrim; Escape and the scrim close it and focus returns to
the trigger. Panel width never animates while the window resizes.

### D4 — One token set, two stylesheets during the transition

The palette, radii, spacing, type, and scrollbar tokens are replaced in
`styles/globals.css`. Shell, catalog, hero, view-header, project-dialog, and
responsive rules live in a new `styles/shell.css` imported after the base sheet.
The run view and editor keep their existing component classes; they inherit the
new tokens and only their outer layout and header are adapted. This keeps this
change reviewable and lets the following view restyles delete their old blocks
instead of rewriting everything twice.

### D5 — Each expanded project owns its leaf reads

A project's leaves fetch through the typed client only while the node is
expanded, so collapsed projects cost no requests. Reads refetch on project
change and window focus; there is no polling timer. A live run's row is
refreshed by merging the open run record when the route names that run, so the
tree follows the REST-refetched record without a second stream. Chats and Flows
each own loading, empty, and error states, and one project's failure never
blocks the workspace or another project.

### D6 — The chat home keeps the start-run contract

The composer is a restyled `StartScreen`, not a new start path: it still uses
the workflow catalog, harness readiness probe, required objective, in-flight
disable, and run creation call. Only the fields' presentation changes. The
accessible names `Project`, `Flow`, `Objective`, and the `Start run` action stay
recognizable so behavior tests keep their subject.

The project control moves to the panel footer and a dialog. The dialog reuses
`ProjectPickerField`, so the native folder dialog and manual path entry keep
their existing behavior.

### D7 — Motion is functional only

Panel collapse, drawer, hover, and focus transitions run at 150–200 ms with
`cubic-bezier(0.4, 0, 0.2, 1)` and are removed under
`prefers-reduced-motion: reduce`. The shell adds no entrance animations, no
parallax, and no decorative gradients; depth comes from surface steps and
hairlines.

### D8 — Projects are a persisted registry, not one active path

The shell keeps an ordered list of project paths plus one active path. The
legacy single-project key upgrades into the list on first read, so an existing
browser keeps its project. The list is owned by `App`; every activation also
adds the path, which makes "select an unknown project" and "add a project" the
same gesture. **Settings** is the registry's home: it lists projects, switches
the active one, adds a typed/native path, and forgets a project without
touching files. Removing the active project falls back to the next one. Project
labels disambiguate duplicate basenames with the fewest parent segments,
because a path alone is a poor sidebar label.

The tree shows every registered project instead of filtering to the active one:
each node has its own leaves, its own loading and error state, and its own
create actions. A failure in one repository cannot hide another project's
sessions, and the active project never determines what the panel is allowed to
show.

### D9 — Run and flow routes carry their project

A run id or flow reference is only unique inside one project, so
`?run=<id>&project=<path>` and `?view=editor&edit=<ref>&project=<path>` carry
the owning project. The route's project is adopted into the active registry on
first open; afterwards the operator may switch the active project without the
open object dragging the panel back. The chat home uses the
active project. An old link without `project` still resolves through the active
project, and a deep link with no known project shows the project gate.

### D10 — The composer owns the create-flow affordance

The suggestion shelf did not earn its space and duplicated the Flows catalog.
Flow creation belongs where the flow choice is blocked: when a project has no
usable flow, the composer shows **New flow** on the selector row (its content
centered by the shared Button span rule), disables the selector with "No flows
available", and explains the next step. When a usable flow exists the composer
stays quiet; the tree's Flows leaf and the editor are the creation surfaces.

### D11 — Settings replaces the project footer

A project selector in the panel foot was a single-purpose control occupying
prime persistent space. Settings is the natural home for the whole local
configuration and currently owns the project registry: switch, add, remove. The
foot keeps one predictable entry point instead of a project popover, and the
workspace header is freed of the **Add project** action that had no object
context.

## Risks

- **Shell height inside a windowed app.** The run view and editor previously
  scrolled the page; in the workspace they must fill and scroll internally.
  Mitigation: every level from `html` to the view body gets `min-height: 0`,
  and the editor keeps a fixed header plus a scrollable tab panel.
- **Tree staleness.** Mitigation: refetch on focus and project change, merge
  the open run record, and expose a retry on each leaf error.
- **Route regressions on old links.** Mitigation: keep the old `view=history`
  and `view=flows` spellings mapped to chat home and cover the deep-link matrix
  in App tests.
- **Visual copy risk.** The reference is used for proportions and tokens only;
  the wordmark, mark, copy, and composer contents stay SpecFlow's.
