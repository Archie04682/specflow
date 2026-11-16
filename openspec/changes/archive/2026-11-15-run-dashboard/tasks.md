# Tasks

> The workbench shell and the standalone history page were superseded by
> `openspec/changes/ui-harness-shell`: the shell now owns the layout and
> breakpoints (3.5), and the Chat catalog is the run history (11.1). Both tasks
> are marked done with that note.

## 1. Read surface: workflow catalog

- [x] 1.1 Add the catalog projection to `backend/internal/orchestrator`: summary types for workflows, nodes, and edges, plus `ReadWorkflowCatalog(projectRoot)` that enumerates `.yaml` files directly under `.specflow/workflows/` and `workflows/`, resolves duplicate references in favor of `.specflow/workflows/`, parses each file, runs `ValidateWorkflow` only, and orders entries by reference; verify table-driven tests cover both directories, duplicate precedence, ordering, and a project with no workflow directory.
- [x] 1.2 Cover catalog failure and side-effect behavior: an unparseable file and a structurally invalid file are returned with `valid: false` and their problems while other entries survive; a workflow binding a non-existent skill is still structurally valid; a catalog read creates no file, run, or worktree; verify those tests pass and the existing `go test ./backend/internal/orchestrator/...` suite stays green.
- [x] 1.3 Add the `GET /api/workflows` handler and route: `project` is required, an unopenable project returns `400` with a JSON error, a successful response has the documented entries; add the endpoint and response fields to the README API table; verify `httptest` cases assert each status and the JSON shape, and the README row matches the handler.

## 2. Read surface: run graph

- [x] 2.1 Add the run graph projection: resolve the run from the store, read `WorkflowSnapshot` and fall back to `WorkflowPath`, parse the retained source, and derive nodes with `id`, `title`, `kind`, effective `interactive` and `allowInject`, and `requireApproval`, plus authored edges; verify unit tests cover a pinned snapshot that differs from an edited project file, a legacy run without a snapshot, an unknown run, and a missing definition.
- [x] 2.2 Add the `GET /api/runs/{id}/graph` handler and route: `404` for an unknown run, `500` for an unreadable or unparseable recorded definition, `200` with the graph otherwise; document the endpoint in the README API table; verify `httptest` cases assert every status and that nodes keep declaration order and edges are authored edges.
- [x] 2.3 Prove the graph read never compiles or mutates: a run whose bound skill directory was removed still returns its graph, and reading the graph of a live run leaves the run status and last event sequence unchanged; verify both assertions in tests.

## 3. Frontend foundation

- [x] 3.1 Add runtime dependencies `@xyflow/react` and `zustand` and dev dependencies `vitest`, `jsdom`, `@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`, and `@types/node`; update the lockfile and keep the pnpm major pin; verify `pnpm install --frozen-lockfile` and `pnpm build` succeed and the built assets still land in `backend/web/dist`.
- [x] 3.2 Add the Vitest configuration and setup (jsdom environment, jest-dom, and stubs for `matchMedia`, `ResizeObserver`, and `EventSource`), a `test` script, and `vitest run` in `make test`; add a smoke test that renders `App`; verify `pnpm test`, `pnpm exec tsc --noEmit`, and `make test` all pass, and the README command table lists the frontend test run.
- [x] 3.3 Mirror the backend wire types in `frontend/src/types`: run statuses and node statuses, `Run`, `NodeRun`, `PendingInteraction`, `InjectionRecord`, harness `Usage`, the graph and catalog responses, and the event envelope; encode the present-but-empty traps (`startedAt` zero time, always-present `usage`, omitted optional fields); verify `tsc --noEmit` passes and a type-level test constructs a run fixture with absent and zero-valued fields.
- [x] 3.4 Implement the typed client in `frontend/src/api`: a `fetch` wrapper that appends the `project` parameter, parses JSON, and throws a typed `ApiError` for `400`, `404`, `409`, `410`, `500`, malformed bodies, and network failures, plus the run, workflow, graph, and action calls; verify unit tests map every status and a network failure to the intended error, and no component owns an ad-hoc `fetch`.
- [x] 3.5 Replace the placeholder shell with the dashboard shell: CSS custom properties for the dark palette, `color-scheme: dark`, a header/main/footer grid, and the desktop and tablet breakpoints; verify `pnpm build` succeeds and a manual pass at 1440px, 1024px, and 768px keeps all regions visible and scrollable. Superseded by `ui-harness-shell`, which delivers the two-track workbench and its 1440/1024/768 pass.

## 4. Event projection

- [x] 4.1 Implement the pure projection reducer: lifecycle items, node markers, coalesced thinking and assistant blocks, tool cards keyed by `callId`, file rows, notices, and status lines, with `data` guarded as unknown; verify unit tests cover a thinking sequence ending in `thinking_done`, a text sequence ending in `text_done`, and a tool start/update/finish sequence producing one card.
- [x] 4.2 Cover the edge cases: `_done` replaces accumulated delta text, a non-delta event closes the open block, a `file_changed` with no matching tool becomes a standalone row and with a matching tool attaches to its card, an unknown `node.event` kind becomes an unrecognized row, and an absent payload does not throw; verify each case in tests.
- [x] 4.3 Make replay idempotent: ignore any event whose `seq` is not greater than the last applied sequence; verify a test replays an overlapping window and produces the same items once, and a second test reconnects from a later sequence without duplicating the earlier items.
- [x] 4.4 Bound the rendered stream: keep all projected items but expose the newest window (last 80, widened by 120 per "show earlier" step), and truncate long text and tool arguments; verify a generated 10,000-delta fixture yields a bounded item count and a bounded rendered window.
- [x] 4.5 Fold SSE replay bursts into the projection once per animation frame (capped per frame) and memoize event rows; verify a burst of 25 events produces one stream update and opening a long retained journal leaves the view responsive.

## 5. SSE lifecycle

- [x] 5.1 Implement `useRunEvents` around an injectable `EventSource` factory: subscribe with `addEventListener` for every type in one `EVENT_TYPES` constant, open with `project` and no `after` on first mount, and expose connecting, live, reconnecting, ended, and failed states; verify fake-source tests open the expected URL, dispatch a named event into the store, and report each connection state.
- [x] 5.2 Handle termination: close the source on the first terminal event, treat `204` and a `CLOSED` ready state as ended rather than failed, and keep a manual reconnect action for an unexpected close; verify tests for each branch and that no second source is opened after a terminal event.
- [x] 5.3 Reopen after a retry or resume with `?after=<lastSeq>` and refetch the run before opening, and prove StrictMode's double mount leaves no duplicate items; verify tests cover the reopen URL, the refetch order, and a simulated double mount with overlapping replay.

## 6. Run state and actions

- [x] 6.1 Add the Zustand store with run and stream slices, selector hooks, and UI state for the selected node; keep REST calls out of the store; update AGENTS.md with the stream-versus-state split, the store choice, and the test gate; verify store unit tests for slice updates and a manual review that components select narrowly.
- [x] 6.2 Implement run and graph loading plus the coalesced lifecycle refetch: fetch `GET /api/runs/{id}` and the graph on open, refetch on lifecycle events with a ~120 ms debounce, never refetch for deltas, and surface refetch failures without discarding the last known state; verify fake-timer and fetch-stub tests assert the fetch counts for delta bursts and lifecycle bursts.
- [x] 6.3 Implement the action hook with the exact payloads for start, answer, inject, approve, cancel, retry, and resume, in-flight disabling per action, and status-aware error handling (`410` closes the interaction and refetches, `409` refetches and shows the refusal); verify unit tests assert every request body, the disabled state, and both conflict paths.
- [x] 6.4 Prove there is no polling: a fetch spy during a run with a simulated event stream records requests only for opening, lifecycle refetches, and actions; verify the assertion fails if a timer-driven run fetch is added.

## 7. Start screen, header, and footer

- [x] 7.1 Build the start screen: project path persisted in `localStorage`, workflow selector fed by the catalog with invalid entries shown as disabled with their problems, a topology preview of the selected workflow, a required objective, and a non-blocking harness readiness warning from `/api/harnesses`; verify Testing Library tests for validation, catalog rendering, persistence, and the warning.
- [x] 7.2 Implement the start flow: submit through the client, disable while in flight, enter the new run on success, keep the form on `400`, and open the returned run plus the error on a start failure that carries a run; verify tests for each branch.
- [x] 7.3 Build the header and footer: workflow and run identity, status chip, project basename, new-run action, live duration while running, node progress, total tokens and cost summed from node usage with an em dash when absent, last event time, and the SSE connection indicator; verify tests with a running fixture and a completed fixture.
- [x] 7.4 Document the dashboard quickstart in the README (start a run, open history, where the project path is stored) and verify the documented commands and clicks match the running UI.

## 8. Graph panel

- [x] 8.1 Implement the deterministic layered layout: longest-path levels from edges, declaration-order tie-breaks, multiple roots at level zero, and a declaration-order fallback on a cycle; verify unit tests for a chain, a diamond, two roots, and a cyclic input.
- [x] 8.2 Build `WorkflowGraph` and `NodeCard` on React Flow with module-scope `nodeTypes`, non-draggable and non-connectable nodes, `fitView` only on topology change, and the derived node states including waiting for an answer from `run.pending` and not run for pending nodes in a terminal run; verify Testing Library tests for the state labels and a manual check that a status change does not move the graph.
- [x] 8.3 Build the failed-node drawer with error, failure class, attempts, checkpoint, and retry controls for forward and clean modes, and handle a refused retry by refreshing the run; verify tests for the payload and the refusal.
- [x] 8.4 Make the graph keyboard-usable and status readable without color: nodes are buttons with status in the accessible name, selection opens the drawer, and the running animation respects reduced motion; verify a keyboard-only manual pass and tests that assert the accessible names.

## 9. Event stream panel

- [x] 9.1 Build `EventStreamPanel`, `EventList`, and the per-kind `EventItem` renderers with timestamps, sequence keys, and distinct rendering for lifecycle, thinking, text, tool, file, notice, status, and unrecognized items; verify Testing Library tests render one fixture per kind and assert the distinguishing text.
- [x] 9.2 Implement stick-to-bottom auto-scroll with a "jump to live" control: scroll only when the view is at the bottom, stop when it scrolls up, and resume on return; verify a fake scroll container test for all three behaviors.
- [x] 9.3 Add the windowed "show earlier" control and the connection state row; verify a test with more than 80 items renders the newest window and reveals earlier items on demand.

## 10. Interaction dock

- [x] 10.1 Build the dock from `run.pending`: choose question, approval, or injection presentation, render a question skeleton until its kind is known, and hide injection when no node is running; verify tests for each pending kind and for the skeleton.
- [x] 10.2 Build `AskCard` for `input`, `select`, `confirm`, and `editor` with the prefill, placeholder, options, default, and deadline, plus a cancel action; verify tests assert `value` for input/select/editor, `confirmed` for confirm, and `cancelled` for cancellation.
- [x] 10.3 Build `InjectBox`: submit only while a node is running, disable in flight, report "queued" on success, keep the text on failure, and surface the `409` reason; verify tests for the request body and both outcomes.
- [x] 10.4 Build `ApprovalCard`: show node, action, attempt, and deadline, send `decision` with `nodeId` and `attempt`, and state that rejecting ends the run cancelled; verify tests assert the approve and reject payloads and the rejection copy.
- [x] 10.5 Recover the question kind after a reload: when `run.pending` is a question and the replayed `question.asked` event has not arrived, keep controls disabled, then enable the correct control; verify an integration test that mounts the run view with a pending question and a delayed replay.
- [x] 10.6 Handle a resolved interaction: a `410` closes the dock with a short notice and refetches the run, and a `409` shows the message and refetches; verify both tests.

## 11. History and unified run view

- [x] 11.1 Build the history view: list `GET /api/runs?project=` newest first with status, workflow, objective, start time, and node progress, and open a run through `?run=`; verify tests for ordering, empty state, and navigation. Superseded by the Chat catalog in `ui-harness-shell`, which is the history surface and opens runs through `?run=`.
- [x] 11.2 Use one run view for live, completed, and failed runs: replay the stream for a terminal run, close on the terminal event, and show final status, per-node results, checkpoints, and error; verify tests for a completed and a failed fixture.
- [ ] 11.3 Wire cancel with confirmation, resume for failed or cancelled runs, retry for failed nodes, and the stream reopen after retry or resume; verify the store and action tests already written pass against the integrated view and a manual run confirms the sequence.
- [x] 11.4 Support deep links and browser navigation: `?run=` and `?view=history` restore state on load, back and forward move between history and a run, and a run without a project shows the project prompt; verify tests for each path and a manual reload of both URLs.

## 12. Integration verification

- [x] 12.1 Run `make test`, `make build`, and the built `./specflow` binary from a real project; verify the dashboard loads cleanly with no console errors and `go test ./...` plus the frontend suite pass.
- [ ] 12.2 Execute the manual matrix against real Pi: start, each question kind, injection, approval approve, approval reject, postcondition failure and retry, cancel and resume, and a stream reconnect; record the observed result for each row in the change before archiving.
- [x] 12.3 Run `openspec validate run-dashboard --strict` and `openspec validate --changes --strict`; verify both pass.
- [ ] 12.4 Do the presentation pass at desktop and tablet widths with keyboard-only navigation and reduced motion enabled; verify every panel and control remains reachable and the running animation is suppressed.

## Workflow follow-up

- Archive the change only after the manual matrix is recorded and both validation gates pass.
- Land the implementation, the synced main specs, and the archive in one commit, per the repository's commit cadence.
