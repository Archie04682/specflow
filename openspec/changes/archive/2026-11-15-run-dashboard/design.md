# Design

## Context

See `proposal.md` for motivation and `specs/` for the behavior contract. What shapes the approach:

- The backend is complete for execution: run lifecycle, worktrees, postconditions, retry/resume, pending interactions, and a sequenced SSE stream. The HTTP surface is in [router.go](backend/internal/api/router.go) and [runs.go](backend/internal/api/runs.go); the stream is in [sse.go](backend/internal/api/sse.go).
- The event taxonomy is dotted and nested: lifecycle events are their own SSE event names, while harness telemetry arrives as `node.event` discriminated by `data.kind` ([contract.go](backend/internal/harness/contract.go), [engine.go](backend/internal/orchestrator/engine.go)). Named SSE events do not reach `EventSource.onmessage`, so a client must subscribe per type.
- A run's record is an execution record, not a graph: `Run.Nodes` is built in **declaration order** (`newNodeRuns` in [engine.go](backend/internal/orchestrator/engine.go#L1630-L1641)), and the compiled `WorkflowPlan` keeps only a topological `Order`, not edges ([plan.go](backend/internal/orchestrator/plan.go)). The only artifact with edges is the workflow source retained at `.specflow/runs/<id>/workflow.yaml`, whose **path** is in `Run.WorkflowSnapshot`.
- `planFor` is an execution seam: it compiles and resolves skills, so it fails when a skill disappeared since creation ([engine.go](backend/internal/orchestrator/engine.go#L1478-L1520)). Inspection must not use it. Any graph read has to parse the retained source structurally.
- The frontend is a Vite shell that builds into the Go embed directory and currently renders one health check ([App.tsx](frontend/src/App.tsx), [vite.config.ts](frontend/vite.config.ts)). React 19, Vite 8, TypeScript 7, `--frozen-lockfile`, and `make test` = `go test` + `tsc --noEmit`.
- Every run endpoint identifies a project by an absolute path in `?project=`; there is no server-owned project and no endpoint that serves the project's workflow files or a run's topology.

## Goals / Non-Goals

**Goals:**

- One run view that serves both a live run and a completed run, so history is not a second implementation.
- A graph that is true to the pinned definition, with positions computed deterministically on the client.
- A stream renderer that stays readable and bounded under token-rate events, and that is testable without a browser.
- State that cannot silently drift from the engine: the run record is authoritative, the stream is narrative.
- Interaction correctness for all four question kinds, including the confirmation boolean that is silently false when answered as text.
- A frontend test suite wired into `make test` from the first commit.

**Non-Goals:**

- No diff rendering: `file_changed` carries only `path` and `tool`; there is no diff payload or diff endpoint. File touches are rendered as file rows until a diff endpoint exists.
- No virtualization library, router, UI kit, markdown renderer, or syntax highlighter.
- No optimistic state for run transitions: the engine's transitions are subtle (retry resets, skip-on-failure, rejection-as-cancellation) and a refetch is cheap.
- No server-owned project, no project registry, no run discard, no workflow editing.
- No Playwright suite in this change; the run view is exercised by component tests plus a manual matrix against real Pi.

## Decisions

### D1 — The project root stays a client-held path

Alternatives: a `--project` server default, a registered project list, or a browser directory picker.

The API already scopes every operation to an absolute project path, and the single-binary story is "run it anywhere, point it at repositories". A server default would touch every handler and the API contract; a registry adds a new resource with lifecycle questions. For this change the path is entered once on the start screen, persisted in `localStorage`, and sent as `?project=` exactly as `curl` does. The header shows the basename. The `?run=` deep link only works when a project is retained; without one the dashboard asks for it instead of issuing a request that cannot succeed.

Consequence: the client must handle a stale or wrong path, which the API reports as `400` with a message. That is a first-class start-screen error, not a silent failure.

### D2 — Two read-only projections, not fields on `Run`

`GET /api/workflows?project=` and `GET /api/runs/{id}/graph?project=` are separate resources rather than new fields on the run record. The run record is persisted and replayed; adding presentation topology would make every run larger, force a schema decision, and couple execution history to UI concerns. The run list also must stay cheap. The graph is fetched once per opened run, then held; it does not change while the run executes.

The endpoints live in `backend/internal/api` as thin handlers over package-level orchestrator functions, mirroring how `ResolveWorkflowPath` is a free function. The catalog needs no engine; the graph needs only the project and the run identifier. Neither handler consults the live run.

### D3 — The graph comes from the pinned source, parsed structurally

The graph read resolves its source the same way `planFor` does — `WorkflowSnapshot`, else `WorkflowPath` — then parses and validates the structure. It deliberately does **not** call `planFor` or `CompileWorkflow`: a completed run whose skill was removed must still render, and a graph read must never fail because a capability disappeared. Skill paths are not needed to draw a graph.

The response carries effective per-node flags that need no compilation (`interactive`, `allowInject` resolved through workflow defaults, and the node's `requireApproval`), because the dock needs them to decide whether to offer injection before an error teaches the user.

Expected shape (exact JSON is fixed by the handler and its test):

```json
{
  "runId": "1f0c...",
  "workflowId": "standard-feature",
  "name": "Standard feature",
  "nodes": [
    {"id": "explore", "title": "Explore", "kind": "agent",
     "interactive": true, "allowInject": true, "requireApproval": false}
  ],
  "edges": [{"from": "explore", "to": "propose"}]
}
```

`nodes` keep authored declaration order; edges are authored edges. The client does not need a topological order because it computes levels from edges.

### D4 — The workflow catalog is complete, structural, and deterministic

The catalog enumerates `.yaml` files directly under `.specflow/workflows/` and `workflows/`, with `.specflow/workflows/` winning a shared reference (matching resolution precedence). `reference` is the basename without `.yaml`, because that is what `POST /api/runs` accepts. Entries are ordered by reference. Each entry is parsed and passed through `ValidateWorkflow` only; no skill resolution, no harness probe, no compile. A file that fails to parse or validate is listed with `valid: false` and its problems rather than hidden, so the start screen can explain why a workflow is unusable instead of omitting it. A project with no workflow directory returns an empty list, not an error.

### D5 — Stream and run state are two data flows

```
REST GET /api/runs/{id}  -------------->  RunState (authoritative)
        ^                                   nodes, status, pending, usage
        | coalesced refetch on               |
        | lifecycle events                   +--> GraphPanel, Header, Footer,
        |                                    |    InteractionDock
SSE lifecycle events -----+
                          |
SSE node.event -----------+--> EventProjection (narrative) --> StreamPanel
   (deltas, tools, files)
```

The client never derives `NodeStatus` from events. It refetches the run on lifecycle events (`run.*`, `node.*`, `question.*`, `approval.*`, `injection.*`, `change.resolved`) with a short debounce (about 120 ms), and on every accepted action. Deltas, tool updates, file changes, usage, notices, and status lines do not trigger a refetch. That is roughly 20-40 small GETs per run, paid only when something happened.

Why: the engine's state transitions are not a pure function of events a client sees — a retry resets the node and every later node, a failure marks the rest skipped, a rejection keeps the attempt unstarted and cancels the run, cancellation marks the running node cancelled. Reimplementing that in TypeScript is how the graph and the record start disagreeing. The run record already encodes the answer, and the engine's own requirement is that a reader sees the pending interaction without replaying the stream.

### D6 — The event projection is a pure reducer, idempotent by sequence

A single pure function folds events into display items:

| Wire | Projection |
| :--- | :--- |
| `run.started`, `run.completed/failed/cancelled`, `change.resolved` | lifecycle divider or banner |
| `node.started/completed/failed/skipped` | section marker and node summary |
| `node.event` with `kind: thinking_delta` / `thinking_done` | one thinking block per turn; `_done` replaces the text |
| `node.event` with `kind: text_delta` / `text_done` | one assistant block per turn; `_done` replaces the text |
| `node.event` with `kind: tool_started/updated/finished` | one tool card keyed by `callId`; updates patch it, finish closes it |
| `node.event` with `kind: file_changed` | a file row attached to the most recent finished card for the same tool on that node, else a standalone row |
| `node.event` with `kind: usage` | not a row; totals are taken from the refetched run |
| `node.event` with `kind: notice/status` | warning row / dim status row |
| `question.*`, `approval.*`, `injection.*` | narrative cards; the dock itself reads `run.pending` |

Rules that make it correct under replay:

- `seq` is the idempotency key. An event with `seq <= lastSeq` is ignored, so a reconnect that replays overlaps cannot duplicate blocks.
- A `*_done` event is authoritative: it replaces the accumulated delta text, which also repairs a dropped delta.
- A non-delta event for the same node closes any open text block, so a tool card starts a new paragraph.
- `data` is `Record<string, unknown>`; every field is guarded. The adapter contract explicitly permits absent payloads.
- `file_changed` has no call identifier, so attachment is best-effort by node, tool name, and adjacency. When it cannot be attached, it is still a file row.
- Unknown `node.event` kinds render as an "unrecognized event" row, never as another kind.

The projection keeps every item but the DOM renders only the newest window — the last 80 items initially, widened by 120 per "show earlier" — so opening a long journal does not build hundreds of rows up front. Item count is bounded by turns and tool calls, not tokens, so a window is enough; no virtualization library is needed. Long text and tool arguments are truncated per item, and each row is memoized so a delta burst re-renders only the block that changed.

A retained replay arrives as one SSE message per event, so the hook queues events and folds them into the projection once per animation frame (at most 400 per frame). Without that batching every replayed event is a separate React render; with it the page stays responsive while the journal catches up and the newest items appear without a long freeze.

### D7 — SSE lifecycle uses the browser's reconnect, with explicit terminal handling

`EventSource` already reconnects with backoff and resends `Last-Event-ID`; hand-rolling exponential backoff duplicates the browser's job and creates double-connection bugs. The dashboard:

- opens one source per run view: `?project=...` with no `after` on first open, so the retained history replays from the beginning;
- reopens explicitly after a retry or resume with `?after=<lastSeq>`, because the engine seals the stream on a terminal status and then starts a new epoch; the old source is closed and the new one picks up exactly where it stopped;
- maps `readyState`: `CONNECTING` after an error is "reconnecting", `CLOSED` is "ended or failed"; a `204` response is the server's stop signal and is reported as ended;
- closes the source on the first terminal event (`run.completed`, `run.failed`, `run.cancelled`) so no reconnect follows;
- exposes the connection state in the footer, with a manual reconnect affordance when the transport is closed unexpectedly.

Named events require an explicit subscription list, declared once as a constant: `run.started`, `node.started`, `node.event`, `node.completed`, `node.failed`, `node.postcondition_failed`, `node.skipped`, `run.completed`, `run.failed`, `run.cancelled`, `question.asked`, `question.answered`, `question.timed_out`, `question.cancelled`, `injection.received`, `injection.acknowledged`, `injection.rejected`, `approval.required`, `approval.granted`, `approval.rejected`, `approval.timed_out`, `change.resolved`. The list mirrors the README event table; a type added on the backend without updating it would be invisible, which is the accepted cost of using `EventSource` over a hand-parsed fetch stream.

React StrictMode mounts effects twice in development; cleanup closes the first source, and the idempotent projection makes the second replay harmless.

### D8 — The interaction dock reads `run.pending` and is gated on the question kind

`run.pending` is the source of truth for what is blocking; it survives a reload and is cleared by the engine when the interaction resolves. Event cards in the stream are narrative only.

The dock must know the question's kind before enabling controls. `PendingInteraction` carries options but not `kind`, `default`, `placeholder`, or `prefill`; the `question.asked` event does. The dock therefore takes the ask shape from the projection's map of `question.asked` payloads by identifier, and shows a skeleton until it is known. This is a correctness gate, not a nicety: the adapter sends `confirmed: answer.Confirmed != nil && *answer.Confirmed` for a confirmation question ([adapter.go](backend/internal/harness/pi/adapter.go#L535-L544)), so a confirmation rendered as a text input and answered with a value is delivered as **false**.

Answers per kind: `input` and `editor` send `value`; `select` sends the chosen option as `value`; `confirm` sends `confirmed`; every kind offers `cancelled: true`. A `410` means the interaction already resolved: close the dock and refetch. A `409` means this run is not waiting for it: show the message and refetch.

Injection is offered only while a node is running, submit is disabled in flight, and the success copy says "queued" because the API queues rather than delivers. Approvals send `nodeId`, `attempt`, and `decision`; rejection is labeled as ending the run cancelled, matching the engine.

### D9 — One `RunView` serves live and historical runs

The run view is the same component tree for a running run, a waiting run, and a completed one; the controls it offers are derived from `run.status` and node statuses. A completed run opens the same SSE endpoint, receives the full replay, and closes on the terminal event. This removes a second graph/stream implementation and makes "basic run detail view" a mode of the same view rather than a parallel screen.

### D10 — Graph rendering: React Flow, custom nodes, deterministic client layout

`@xyflow/react` is the choice because the next planned surface is a graph editor and because pan/zoom, custom node rendering, and edges are solved. It is pinned to 12.12.x; its peer range accepts React 19.

Positions are computed from edges with longest-path layering: roots at level 0, each node one level below its deepest predecessor, siblings ordered by declaration order. A chain renders as a vertical line; a branch renders side by side. A cycle (which validation prevents) falls back to declaration order. This is ~60 deterministic lines, avoids a layout dependency, and — critically — keeps positions stable when only statuses change, so the graph does not jump during execution.

`nodeTypes` is defined at module scope. `fitView` runs on init and when the topology identity changes, never on status changes. Nodes are not draggable or connectable. A failed node selects a details drawer with error, class, attempts, and checkpoint plus the retry controls. Edges use a smooth-step style; the edge entering a running node may animate, which is cosmetic and disabled under reduced motion.

Node visuals derive as:

```
pending                  -> pending
running                  -> running (pulse unless reduced motion)
waiting_approval         -> waiting approval (also run.pending approval)
node.status running +
  run.pending is question -> waiting answer   (node status has no waiting_answer)
terminal run + pending   -> not run
failed / skipped / cancelled / completed  -> as recorded
```

Status is always stated in text or an icon inside the node, not only by color.

### D11 — Zustand for one store with run and stream slices

Alternatives: `useReducer` + context, or a hand-rolled `useSyncExternalStore`.

The SSE callback lives outside React and writes at token rate; selectors let the graph, dock, and footer subscribe to narrow slices without rerendering the stream. A single store with two slices avoids syncing two stores, and `getState()` is usable from the event handler. `zustand` 5.1 is small and React 19-compatible. The dependency is deliberate: AGENTS.md says not to add a state library casually, and the justification is the out-of-React producer plus selector granularity. If the team prefers zero dependencies, the same slices can move to a reducer + split contexts without changing the component contracts.

Store shape: `run`, `graph`, `graphError`, `loading`, `error`, `pendingAction`, `stream.status`, `stream.items`, `stream.lastSeq`, `stream.askById`, `ui.selectedNode`. REST calls live in hooks (`useRunActions`, `useWorkflows`), not in the store; they call store setters.

### D12 — Shell layout, dark tokens, and no Tailwind

The shell is a CSS grid: header (fixed), main (`minmax(320px, 38%) 1fr` above 1024px, stacked below), footer. The interaction dock is a sticky panel at the bottom of the right column, not a modal, so the stream stays visible while a question is pending; on tablet it stacks under the stream. React Flow gets a sized grid cell with `min-height: 0`.

Styles use one `globals.css` with custom properties for the dark palette, plus a small set of semantic classes. Tailwind 4 would work with Vite 8, but it adds a build plugin and PostCSS configuration for a fixed, single-theme developer tool; hand-written CSS keeps the JSX readable and the dependency surface small. Light theme is out of scope; `color-scheme: dark` is set.

Accessibility: `:focus-visible` rings, buttons for actionable nodes, `aria-label` carrying node status, a single polite live region for interaction prompts and terminal status (never for the token stream), and `prefers-reduced-motion` disabling the pulse.

### D13 — URL state without a router

`?run=<id>` opens a run; `?view=history` opens the history list. Navigation uses `history.pushState` and a `popstate` listener. The SPA fallback already serves the entry document for a deep link ([spa.go](backend/internal/api/spa.go)). A router dependency earns nothing for two views.

### D14 — One typed client, one error model

`frontend/src/api/client.ts` wraps `fetch`, adds the `project` query parameter, parses JSON, and throws a typed `ApiError {status, message}`. Status mapping: `400`/`404`/`409`/`410`/`500` keep the server's `error` text; a network failure becomes a distinct "cannot reach the server" error; a non-JSON body becomes a generic message. No component calls `fetch` directly. Types mirror the Go JSON exactly, with the traps written down: `NodeRun.startedAt` is always present and may be the zero time (`0001-01-01...`); `usage` is always present as an object; `pending`, `finishedAt`, `error`, and `injections` are omitted when absent.

### D15 — Testing seams and the gate

Vitest 5, jsdom 31, and Testing Library 16 are compatible with React 19 and Vite 8.

- Pure tests: event projection (deltas, done-authoritative, tool pairing, file attachment, idempotency, unknown kinds), layout (chain, diamond, multiple roots), node state derivation, error mapping.
- Component tests: start form validation and server-error display; each question kind including the confirmation payload; approval decision payload; inject queued/refused; auto-scroll behavior with a fake scroll container; dock closing on `410`.
- The SSE hook takes an injectable `EventSource` factory so tests can drive events synchronously. jsdom has no `EventSource` or `ResizeObserver`; the setup file provides stubs, and the React Flow component is not unit-tested (its layout function and node card are).
- `make test` runs `vitest run` after `go test` and the typecheck. The test files are included in `tsc --noEmit`, so they must typecheck too.
- Manual matrix against real Pi: start, question of each kind, injection, approval approve/reject, postcondition failure + retry, cancel + resume, stream reconnect. The binary registers only the `pi` adapter, so there is no deterministic fake harness for manual UI work; that is recorded as a follow-up.

## Risks / Trade-offs

| Risk | Mitigation |
| :--- | :--- |
| A token-rate stream overwhelms rendering | Coalesce deltas into blocks; windowed DOM; item text truncated; no per-token state. |
| A reconnect replays a large journal | Fold queued replay events once per frame; render the newest window; idempotent by `seq`. |
| Named SSE events are missed because the type list is stale | One `EVENT_TYPES` constant with a comment tying it to the README table; unknown `node.event` kinds still render generically. |
| Answering a confirmation as text silently yields `false` | Gate the dock on the `question.asked` kind; reload recovers it from replay before enabling controls. |
| Refetch storms around terminal transitions | Debounce ~120 ms and coalesce per run; terminal refetch is single. |
| Engine state differs from the client's guess | The client never guesses: run state comes only from the run record. |
| React Flow + React 19 + StrictMode quirks | Pin 12.12.x, define `nodeTypes` at module scope, keep positions derived rather than stored, and test the build early. |
| A large dependency install breaks the frozen lockfile | Update `pnpm-lock.yaml` in the same change; `make test`/CI use `--frozen-lockfile`. |
| A stale project path fails every request | Explicit start-screen error with an edit action; the stored path is verified by the first request, not assumed. |
| The graph endpoint accidentally gains compile behavior | Handler reads the retained source only; a test removes a bound skill and asserts the graph still returns. |
| No deterministic UI harness in the binary | Manual matrix with real Pi; component tests cover behavior; a dev mock adapter is a follow-up, not a silent gap. |
| Unauthenticated local UI can cancel/approve runs | No new mutation surface: the same endpoints were already reachable; documented as a local-tool assumption. |
| No diff view despite a "diff" expectation | Explicit non-goal: events carry file paths only; render file rows and revisit when a diff endpoint exists. |

## Migration Plan

1. Land the two read endpoints and their tests first; they are additive and independently testable.
2. Build the frontend against them; the placeholder app is replaced in the same change, and the Vite build still writes into `backend/web/dist`, so the single-binary build is unchanged.
3. Update `Makefile`, `README.md`, and `AGENTS.md`; the lockfile change ships with the dependency addition.
4. Rollback is a revert of the frontend tree and the two handlers; there is no persisted-data migration, because `run.json` is untouched and legacy runs fall back to `WorkflowPath`.

## Open Questions

- Should a later change extend `PendingInteraction` with the question's shape so a reload no longer needs the replayed event? The current design is correct without it; the field would remove a startup dependency, not change behavior.
- Should the server ever own a default project (`--project`, `GET /api/project`)? That is a product decision for a later change; this design treats the path as client state.
- Should the binary expose a development mock harness so the dashboard can be exercised deterministically without real Pi and credentials? Useful test infrastructure, but a separate change.
