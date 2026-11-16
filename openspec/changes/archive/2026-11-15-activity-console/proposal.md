# Proposal

## Why

The Activity window is the operator's primary view of a running agent, but it is the least
readable part of the dashboard. A measured run produced 27 thinking blocks totalling ~175k
characters, all rendered fully expanded as plain text; assistant and question bodies are wrapped
in bare paragraphs, tool calls are raw `JSON.stringify` output, and `command` nodes (the
`openspec validate` / `make test` gates) show only a start/finish marker with no command and no
output. A select question renders as a title plus options with no description, because Pi's dialog
protocol carries no description field and the bridge never forwards the ask's actual text. The
panel also has a fixed width, so operators cannot trade graph space for log space.

## What Changes

- **Thinking collapses by default.** Each thinking block becomes a one-row disclosure (label, size,
  streaming indicator) that the operator expands. The change keeps per-block expansion state while
  the stream windows and re-renders it.
- **Activity content renders as markdown.** Assistant messages, thinking blocks, question
  descriptions and answers, injection text, and notices render markdown (GFM tables, lists, fenced
  code) through one shared component; errors, command lines, and command output stay preformatted.
  Raw HTML is never injected, images are not fetched, and links open externally with safe rel.
- **The activity split is resizable.** A draggable, keyboard-operable separator between the graph
  and the activity column changes its width within bounds; the width is remembered per browser and
  the handle disappears in the stacked layout.
- **Tool calls render their input and output.** `bash` shows a shell prompt line and a terminal
  block; `read`/`write`/`edit`/`grep`/`find`/`ls` show a path or pattern line; `specflow_ask` shows
  the question as asked; unknown tools show labelled key/value rows instead of a JSON blob. Long
  output is expandable, and the adapter's 2k tool-result truncation is raised to match the stream's
  16k summary bound.
- **`command` nodes report their command and output.** The compiled node record gains the rendered
  command, the run publishes `command.started` and `command.finished` events carrying the command,
  the working directory, the exit code, the captured output, and the duration, and both the node
  details drawer and the Activity window render them.
- **Questions carry their description.** The Pi adapter folds the bridged ask tool's `question`
  argument into the contract's `Ask.Message`, so the interaction dock and the Activity card show the
  operator the context the model actually asked with, for every ask kind.

## Capabilities

### New Capabilities

<!-- None: every requirement lands in a capability that already exists or that the in-flight
     run-dashboard change introduces. -->

- None.

### Modified Capabilities

- `run-dashboard`: adds activity-window presentation requirements (collapsed thinking, markdown
  content, resizable split, tool input/output rendering, question description and command
  rendering). The capability is introduced by the in-flight `run-dashboard` change, so this change
  contributes ADDED requirement blocks rather than rewriting that change's requirements.
- `harness-pi-adapter`: the embedded question bridge now transports the ask's description into the
  adapter contract, so a question reaches the operator with its context.
- `workflow-orchestration`: `command` nodes record the command they ran and publish
  `command.started` / `command.finished` events with their exit code, captured output, and duration.

## Impact

- **Backend**: `internal/harness/pi/translate.go` (ask enrichment, tool summary bound),
  `internal/orchestrator/engine.go` (command events, node record), `internal/orchestrator/run.go`
  (`NodeRun.Command`), plus adapter/orchestrator tests.
- **Frontend**: `components/RunDashboard` (activity panel, tool cards, command cards, split,
  interaction dock), a shared markdown component, `projection/eventProjection.ts` (command item
  kind), `api/sse.ts` (event-type subscription list), `types/run.ts`, `lib/storage.ts` and
  `lib/split.ts`, `styles/globals.css`.
- **Dependencies**: adds `react-markdown` and `remark-gfm` to `frontend/package.json` and the
  committed lockfile; the embedded single-binary build is unaffected.
- **Docs**: AGENTS.md names the run-split preference in its browser-storage rule.
- **Out of scope**: syntax highlighting, remote image fetching, live streaming of `command` output
  while a process runs, resizing the interaction dock, and any Pi protocol change.
