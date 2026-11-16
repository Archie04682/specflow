# Tasks

## 1. Question descriptions from the Pi adapter

- [x] 1.1 Record each `specflow_ask` tool call's arguments on `tool_execution_start` in the Pi translator and merge them into the next dialog ask (message, plus missing title/options), dropping the entry on `tool_execution_end`; verify `translate_test.go` covers select, input, and editor dialogs gaining the question text, a confirm dialog keeping its own message, a FIFO match when two asks are outstanding, and an unmatched dialog still answering.
- [x] 1.2 Extend the scripted fake-pi ask round trip with a `specflow_ask` tool start before the dialog and verify the end-to-end adapter test observes the description on the ask and still answers it.
- [x] 1.3 Note the description derivation in the AGENTS.md Pi adapter bullets and verify the sentence matches what the translator test asserts.
- [x] 1.4 Recover a pending question's description from the ask tool call in the stream when the question event carries none, and verify the dock test and the selector's boundary tests.

## 2. Markdown rendering foundation

- [x] 2.1 Add `react-markdown` and `remark-gfm` to `frontend/package.json` with the committed lockfile; verify `pnpm install --frozen-lockfile` and the TypeScript build succeed with React 19.
- [x] 2.2 Build `components/common/Markdown.tsx` with component overrides for fenced code, inline code, links, images, and tables, plus `.markdown` styles; verify component tests for rendered structure, raw HTML shown as text, an image reference producing no `<img>`, links carrying `target` and `rel`, and a table scrolling in its container.
- [x] 2.3 Wire the markdown component into assistant messages, expanded thinking, question descriptions in `AskCard`, interaction cards, and notices, while a block still streaming renders as plain preformatted text; verify `EventItem` and `AskCard` tests for the plain-to-markdown switch, a markdown description above the options, and error/tool text staying verbatim.

## 3. Collapsed thinking blocks

- [x] 3.1 Extract `ThinkingBlock` as a collapsed disclosure (label, size, streaming indicator, `aria-expanded` and a controlled body) and move expansion state to `EventList` keyed by item id; verify tests for collapsed-by-default with no body rendered, expanding reveals markdown, a streaming block stays collapsed while reporting activity, expansion surviving new events, and the toggle re-rendering only that row.
- [x] 3.2 Update `EventItem` and `EventList` tests for the disclosure and for expansion surviving windowing past the visible item count.

## 4. Tool call input and output rendering

- [x] 4.1 Add the pure `describeTool(name, args)` helper returning command, file, search, question, or generic descriptions and verify unit tests for `bash`, `read`, `write`, `edit`, `grep`, `find`, `ls`, `specflow_ask`, and an unknown tool.
- [x] 4.2 Build `ToolCard` with the tool-specific input line, a bounded output block that expands on request, a failure state, and file chips; verify component tests for a bash prompt line, a failed finish, long output expanding, and an unknown tool rendered as labelled rows.
- [x] 4.3 Raise the Pi translator's tool summary bound to the stream's 16k summary limit and verify a long tool result survives into the projection.
- [x] 4.4 Replace the raw-JSON `ToolView` in `EventItem` with `ToolCard` and verify the activity test asserts a command line and no JSON blob for a bash call.

## 5. Command node visibility

- [x] 5.1 Render each command node's command once, record it on the node run, and publish `command.started` and `command.finished` with the command, working directory, exit code, captured output, and duration for success, non-zero exit, timeout, and cancellation; verify engine tests for both event payloads, the recorded command, and a timeout still publishing its finish before the failure.
- [x] 5.2 Add `command.started` and `command.finished` to the SSE event-type list and teach the projection a command item that starts running and settles on the finish event; verify projection tests including an out-of-order finish and a replayed journal.
- [x] 5.3 Render command cards in the activity window and a command section in the node details drawer from the run record; verify component tests for the running command line, the settled output with exit code and duration, a failed command, and a reloaded run's drawer.
- [x] 5.4 Add `command` to the frontend node run type and verify the run-record type test accepts a command node with and without it.

## 6. Resizable activity split

- [x] 6.1 Add `lib/split.ts` with the default, minimum, and clamp helpers and verify unit tests for both bounds, a container too narrow for both minimums, and a default reset.
- [x] 6.2 Persist the chosen width through `lib/storage.ts` under `specflow.run-split` with clamping on read and a corrupt value falling back to the default; verify storage tests.
- [x] 6.3 Build the run split with a separator (`role="separator"`, orientation, value attributes, pointer capture, arrow-key steps, home/end bounds, double-click reset) and a `ResizeObserver` that re-clamps on window changes; verify the keyboard, reset, and clamp tests and record a manual pointer-drag check.
- [x] 6.4 Replace the fixed run-body grid with the split layout, keep the stacked layout at the existing breakpoint with no separator, and verify the run view test still renders the graph, activity stream, and dock plus the separator at desktop width.
- [x] 6.5 Extend the browser-storage sentence in AGENTS.md to name the run-split preference and verify no other keys were added to the documented set.

## 7. Integration verification

- [x] 7.1 Run `make test`, `openspec validate activity-console --strict`, and `openspec validate --changes --strict`; verify all three pass and record the results.
- [ ] 7.2 Walk a live run with real Pi through the four changes: collapsed thinking, markdown rendering, a bash tool card's command and output, a command node's command and output, a select question showing its description, and the separator drag surviving a reload; record the observed result for each.
- [ ] 7.3 Do the presentation pass at desktop and tablet widths with keyboard-only navigation and reduced motion enabled; verify every activity card, the separator, and the dock remain reachable and the streaming indicator is suppressed under reduced motion.

## Workflow follow-up

- Archive `run-dashboard` before this change so the `run-dashboard` capability keeps its Purpose; if this change must archive first, backfill the Purpose in `openspec/specs/run-dashboard/spec.md` by hand.
- Archive this change after both validation gates and the manual pass, landing the implementation, the synced main specs, and the archive in one commit per the repository's commit cadence.
