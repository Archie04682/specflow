# Design

## Context

See `proposal.md` for motivation and `specs/` for the behavior contract. What shapes the approach:

- **The activity feed is a pure projection.** Harness telemetry becomes `harness.Event`, the engine
  wraps it in a sequenced `RunEvent`, the SSE endpoint names the event type, and
  `projection/eventProjection.ts` folds it into `StreamItem`s. The projection is a pure reducer with
  no DOM; `EventItem` renders one item and is memoized so a delta burst re-renders one row. Rendering
  logic belongs in components, not the reducer.
- **The run record is the only source of node state.** REST `GET /api/runs/{id}` carries node
  status, output, and exit code; the stream is narrative. A reload must still show command details.
- **Two run states already exist for text.** Thinking and message items carry `complete` and are
  truncated at `MAX_TEXT` (20k); tool args are truncated at `MAX_ARGS` (4k). Long blocks are bounded
  by windowing (`INITIAL_VISIBLE_ITEMS = 80`) and "show earlier" paging.
- **The frontend has four runtime dependencies** and no markdown renderer. The build is embedded into
  the single Go binary by Vite; dependency additions are frontend-only.
- **Pi's dialog protocol has no description slot.** The RPC union (`@earendil-works/pi-coding-agent`
  `dist/modes/rpc/rpc-types.d.ts`) gives `select` a title and options, `input` a title and
  placeholder, `editor` a title and prefill, and only `confirm` a message. The bridge's
  `specflow_ask` tool receives the full question in its arguments, and Pi emits
  `tool_execution_start` with those arguments before the dialog request — the retained journal for a
  real run proves both sides (`question.asked` with `message: ""`, `specflow_ask` tool starts
  carrying `question`).
- **Command nodes are silent on the stream.** The engine captures stdout and stderr into
  `NodeRun.Output` and reports `exitCode` only in the failure text; the node record has no command,
  and the run graph deliberately carries authored topology (no `command` field).
- **The run layout is one CSS grid** (`globals.css` `.run-body`) with a fixed `38%` first column and a
  stacked variant at `1180px`. There is no resizer anywhere in the app.
- **AGENTS.md constraints that bind here**: one component per file; the projection stays pure; no
  second state library; typed API client; browser storage through `lib/storage.ts`; the single-binary
  build stays intact.

## Goals / Non-Goals

**Goals:**

- A readable activity feed: collapsed thinking, markdown for prose, structured tool/command cards.
- Operator control of the activity column's width, remembered across runs.
- A question that shows the context the model actually asked with, for every ask kind.
- Command-node input and output visible live and after reload, without changing the harness contract.

**Non-Goals:**

- Syntax highlighting, image rendering, or remote fetches from markdown.
- Streaming command output chunk-by-chunk while a process runs (start and finish only).
- Resizing the interaction dock or making the split vertical.
- Changing Pi's RPC protocol, the harness `Ask`/`ToolEvent` contract, or the run graph shape.

## Decisions

### D1. Markdown through `react-markdown` + `remark-gfm`

One `components/common/Markdown.tsx` renders to React elements. It never injects raw HTML, so no
sanitizer is needed; component overrides give block code the existing `.code-block` treatment, links
`target="_blank"` with `rel="noreferrer noopener"`, images replaced by their description text, and
tables a scroll container. `.markdown` styles live beside the other feed styles.

*Alternatives:* `marked` + `DOMPurify` is two packages but requires `dangerouslySetInnerHTML` plus a
sanitizer on every render; a hand-rolled subset renderer has no dependencies but makes every markdown
edge case our maintenance burden indefinitely.

*Streaming policy:* a block renders as plain preformatted text while `complete === false` and as
markdown once it settles. Markdown is therefore parsed once per block, not once per token, and a
half-emitted `**` never flickers into a broken tag.

### D2. Thinking expansion state lives in `EventList`

`EventList` owns `expanded: Set<number>` (seq ids) plus a stable `toggle(id)` callback, passing
`expanded` to `EventItem` and down to a new `ThinkingBlock`. `EventItem` stays memoized, and only the
toggled row re-renders. Collapsed is the default for complete and streaming blocks alike; a streaming
row shows its label, size, and an activity indicator instead of its body. The state survives
windowing and re-renders because it is keyed by the projection's stable `seq` id.

*Alternatives:* local `useState` inside `EventItem` loses expansion when an item scrolls out of the
80-item window and back; a zustand slice puts view state into a store the projection owns; an
expand-all header toggle is useful but is deferred scope.

### D3. The Pi adapter, not the bridge, supplies the description

The translator records each `specflow_ask` tool call's arguments on `tool_execution_start` (a short
FIFO of `{callId, title, question, options}`). When a dialog `Ask` arrives, it merges the oldest
matching entry — title match first, else the oldest — into `Ask.Message` when the dialog carried no
message, fills a missing title or options from the same entry, and drops the entry. Entries are
discarded on that call's `tool_execution_end`. Nothing changes in the bridge, the `Ask` contract, the
session layer, or the UI: `question.asked`, the pending record, and the ask-by-id map all already
carry `message`, so filling it upstream is enough.

A display-only fallback covers streams whose question event still lacks the text: when the dock's
question shape has no message, `recoverQuestionDescription` reads the question argument from the ask
tool call in the same stream (stopping at an interaction or node boundary so an older ask cannot lend
its text to a newer question). That keeps a journal written before this change readable and stays a
pure selector outside the reducer; the adapter remains the source of truth for new runs.

*Alternatives:* smuggling the description through the dialog title requires a private delimiter
contract and pollutes the heading Pi shows; extending Pi's protocol is not ours to do; rebuilding the
whole ask from the tool card would make the UI depend on one harness's tool call instead of using it
only as a read-only fallback.

*Fallback:* a dialog with no matching tool call keeps the dialog's own fields and remains answerable,
which is today's behavior.

### D4. Command nodes publish `command.started` / `command.finished`

The renderer emits `command.started` with the rendered command and working directory before
`exec.CommandContext` runs, and `command.finished` with exit code, captured output, and duration
after it returns — on success, non-zero exit, timeout, and cancellation. The rendered command is also
recorded on `NodeRun.Command` (new `omitempty` field) so the drawer can show it from the run record
after a reload without reading the journal. Output reuses the engine's existing bounded buffer
(64 KiB), so the event stays far below the 4 MiB event limit. The two names are added to the SSE
subscription list in `frontend/src/api/sse.ts`; they are deliberately outside the `node.*` lifecycle
prefix, so they are narrative events and do not trigger a run-refetch per command.

*Alternatives:* reusing `node.event` with new `kind` values would overload the harness-telemetry
namespace with orchestrator lifecycle; emitting only at the end would hide the command line while a
slow gate runs; chunked live output needs pipe plumbing, throttling, and journal growth, and is
deferred because the requested rendering is satisfied by start plus finish.

### D5. The split is a clamped px width with a keyboard-operable separator

The activity column's width is a pixel value (`DEFAULT_RUN_SIDE = 520`, `MIN_RUN_SIDE = 360`,
`MIN_GRAPH = 320`) exposed as a CSS variable consumed by the desktop grid rule only; the stacked
media query replaces the columns and hides the separator, so no inline style fights the responsive
layout. A pure `clampRunSide(width, containerWidth)` in `lib/split.ts` is unit tested. The separator
is `role="separator"` with `aria-orientation="vertical"`, `aria-valuemin/max/now`, pointer capture,
arrow-key steps, home/end bounds, and double-click reset; a `ResizeObserver` re-clamps when the
window shrinks. The width persists through `lib/storage.ts` under `specflow.run-split`, which extends
the browser-storage sentence in AGENTS.md.

*Alternatives:* a percentage keeps the graph's minimum unknown when the viewport changes; storing the
width in the run store mixes view state into data; a second vertical separator for the dock is
unrequested and the dock is content-sized.

### D6. Tool rendering is a pure description plus a card

`describeTool(name, args)` in `lib/toolArguments.ts` maps arguments to a discriminated union:
`command` (shell line, optional timeout), `file` (path, optional range or bytes), `search` (pattern,
optional path), `question` (title, description, options), or `generic` (labelled rows). A new
`ToolCard` renders the header, the description, the output block, and file chips; long output starts
bounded and expands on request, and a failed tool gets failure treatment. Pi's summary bound rises
from 2,000 to 16,000 characters to match `MAX_SUMMARY`, because expanding a 2k-truncated block would
still hide the real output.

*Alternatives:* a collapsible raw-JSON block keeps the JSON problem at the surface the operator looks
at most; one component per tool duplicates layout and state; adding `exitCode` to `ToolEvent` would
widen the harness contract for a cosmetic gain, and success/failure already arrives as `isError`.

### D7. Two in-flight changes share `run-dashboard`

The in-flight `run-dashboard` change introduces that capability and has not archived. Following the
existing `native-project-picker` precedent, this change contributes ADDED blocks to the capability's
delta and lists it under Modified Capabilities, leaving `run-dashboard`'s requirements and Purpose
untouched. `harness-pi-adapter` and `workflow-orchestration` are main specs, so their deltas are
MODIFIED blocks that keep every existing scenario name.

## Risks / Trade-offs

- **Markdown bundle growth** → the build is embedded and served locally; the dependency is
  frontend-only and `make build` is the guard. Verify `react-markdown` and `remark-gfm` install and
  typecheck against React 19 before wiring the component.
- **Markdown link and image safety** → raw HTML is rendered as text by default, URL schemes are
  filtered by the renderer, and images are replaced with their description so agent output cannot
  beacon out of the local app.
- **Ask correlation mismatch on parallel tool calls** → FIFO with title preference, cleared on tool
  end; a miss degrades to the dialog's own fields, which remain answerable.
- **Journal and SSE volume from command output** → the engine's 64 KiB bound caps one finish event;
  a gate that prints megabytes cannot inflate the journal past that bound.
- **Resizer interacts with the graph canvas and window resizes** → the separator is a sibling of the
  canvas, pointer-captured, and re-clamped by a `ResizeObserver`; jsdom cannot exercise pointer
  capture, so the clamp helper and keyboard path are unit tested and the drag is checked manually in
  the run.
- **Persisted width from a larger display** → every read is clamped against the current container
  width, not trusted as stored.
- **Archive ordering with `run-dashboard`** → if this change archives first, the new main spec is
  created without a Purpose; the tasks include archiving after `run-dashboard`, and backfilling the
  Purpose in the main spec if that order cannot hold.
- **A missed event-type subscription** → a frontend test asserts the new command types are in
  `EVENT_TYPES`, mirroring the file's own comment that the list is the client's subscription contract.

## Migration Plan

No data migration. The new event types are additive: an older client ignores them, and a replayed
older journal simply contains none. `NodeRun.Command` is additive with `omitempty`, so existing run
records stay readable. Rollback is reverting the frontend dependency and code plus the adapter and
engine changes; run journals and records written meanwhile need no cleanup.
