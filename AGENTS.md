# AGENTS.md

Context for AI agents and human contributors. Read this before changing code.
Product overview and setup live in `README.md`; the public manual lives in
`docs/`; durable behavior lives in `openspec/specs/`.

## What this is

SpecFlow is a harness-agnostic visual orchestrator for spec-driven
development. A user draws a graph of agent steps (explore, propose, apply,
review, test, verify, archive), each step binds one or more skills, a harness,
a model, and interaction rules, and the backend executes the graph while streaming
progress to the UI.

**Current phase: orchestrator foundations.** The repository contains the
single-binary shell, the harness adapter contract, the first adapter (Pi), and
the workflow engine. A run loads a workflow, executes its nodes in an isolated
git worktree, checks each node's postcondition against the repository, and
streams events. Workflows are still canonical YAML under the hood; the flow
editor edits a structured draft and the server writes the file.

## Architecture

```text
+---------------------------------------------+
| React UI: graph, live log, Q&A, inject box  |
+----------------------+----------------------+
                       | REST + SSE (WebSocket for Q&A)
+----------------------v----------------------+
| Go orchestrator: DAG state machine,         |
| sessions, OpenSpec mapping, embedded UI     |
+----------------------+----------------------+
                       | harness protocol (JSON events)
+----------------------v----------------------+
| Adapters: Pi first; Claude, others later    |
+---------------------------------------------+
```

The orchestrator stays thin on agent mechanics: adapters own the LLM loop and
tool execution, the orchestrator owns graph state, OpenSpec validation, and
user interaction.

## Repository layout

```text
backend/cmd/api        server entrypoint
backend/internal/api   HTTP router, handlers, SPA fallback
backend/internal/config  port resolution
backend/internal/orchestrator  workflow model, engine, postconditions, retry, run store
backend/internal/workspace     project, .specflow layout, worktree, checkpoints, merge
backend/internal/harness       adapter contract, registry, preflight, scripted mock
backend/internal/harness/pi    Pi adapter: RPC transport, translation, ask bridge
backend/internal/openspec      skill resolution, change and task helpers for gates
backend/internal/session       harness.Host: events, questions, injections
backend/web            go:embed of the built frontend
frontend               Vite + React + TypeScript
workflows              reusable workflow templates
openspec               specs and in-flight changes
docs                   public manual: architecture, workflows, API, UI, harnesses
```

`.dsh/` (harness skills) and `.specflow/` (run journals, worktrees) are local
only and gitignored. `docs/` is the public manual; keep it in sync when setup or
behavior changes.

## Decisions that bind

1. **One executable.** The frontend is built by Vite directly into
   `backend/web/dist` and embedded with `//go:embed all:dist`. The committed
   `backend/web/dist/.gitkeep` keeps compilation working before the first
   frontend build. `make build` always builds the frontend first.
2. **Module path is a placeholder.** `example.com/specflow` is intentional and
   unresolvable; it is kept as the module path for now.
3. **Harness-agnostic.** The orchestrator talks to an adapter interface, never
   to a specific runtime. Adding an adapter must not change the orchestrator.
4. **OpenSpec is the source of truth.** Behavior changes start as a change
   under `openspec/changes/`. Archive only after `openspec validate <change> --strict`
   and the test suite pass.
5. **One terminal result per execution.** An adapter returns exactly one
   outcome; completion is that return value, never a stream event (`agent_end`
   is not the end of a Pi run — only `agent_settled` is). Questions and
   injection are gated by declared capabilities; telemetry is best-effort and
   may be absent. A node's needs are checked against capabilities before it
   starts.
6. **Verify is a gate, not a skill.** Run `openspec validate <change> --strict`
   for the change plus `make test`; `openspec validate --changes --strict` is
   the repository-wide gate. Do not invent a verify skill or prompt your way around the
   gate.
7. **Port precedence.** `--port` flag > `PORT` environment variable > `8080`,
   validated to 1–65535. Keep any new config in `internal/config` with tests.
8. **pnpm, lockfile committed.** `frontend/package.json` pins the pnpm major;
   use `--frozen-lockfile` in anything automated.
9. **The repository is the run's state.** Edges carry ordering, not data. A
   node receives a prompt rendered from a closed set (`objective`, `change`,
   `run_id`, `workdir`), a working directory, and a resolved skill; its result
   is what it wrote to disk. Do not add a context map or a template language.
10. **Every run is isolated.** A run executes in `.specflow/worktrees/<id>` on
    `specflow/run-<id>`, checkpoints after each node, and merges only through
    an approval-gated command node that runs in the project checkout.
    `.specflow/worktrees/` is never version-controlled; the rest of
    `.specflow/` is committable if the user wants run history in git.
11. **A run is pinned to a compiled workflow.** Before a run is created, the
    workflow is parsed, validated, and normalized into a plan; the source that
    compiled is retained at `.specflow/runs/<id>/workflow.yaml`, and resume
    compiles that copy. Editing a workflow after creation does not change a
    run; a resumed run fails if the environment no longer satisfies the
    recorded definition.
12. **Skills resolve from one shared root.** A skill name resolves from
    `.specflow/skills` first, then `.dsh/skills`, `.pi/skills`, and
    `~/.dsh/skills`; a `skills` list in `.specflow/config.yaml` replaces that
    order. The harness receives the resolved absolute path of every selected skill (Pi
    runs with `--no-skills --skill <path>` repeated), so never copy a skill into
    a harness-specific folder. The editor's multi-select skill catalog reads the
    same roots; a node stores `skills:` as a list, and older `skill:` files stay
    readable.
13. **One project tree, one shell, many projects.** The workbench is a
    persistent left control panel plus a workspace. The panel is a project tree:
    each project node expands to **Chats** and **Flows** leaves, and there are
    no Chat/Flows tabs and no flows index page. The registry is an ordered path
    list plus one active project in `localStorage` (`specflow.projects`,
    `specflow.project`); **Settings** in the panel foot switches, adds, and
    removes projects. Routes are `/`, `?run=<id>&project=<path>`, and
    `?view=editor[&edit=][&project=<path>]`; a run or flow deep link adopts its
    project, while the chat home uses the active one. There is no separate
    history page; the tree is the navigation. Keep the tree and Settings in
    `frontend/src/components/Shell`, shell layout in `styles/shell.css`, and the
    harness-derived token set as the single visual language (flat near-black
    surfaces, hairlines, one blue accent, near-white primary actions).

### Harness adapters

- `internal/harness` owns the contract; `internal/harness/pi` is the first
  adapter. Adding an adapter must not change the orchestrator.
- Pi runs as a child process in RPC mode; the adapter owns its lifecycle,
  framings, and shutdown escalation.
- Interactive nodes work because the binary embeds a Pi extension
  (`pi/bridge/specflow-bridge.ts`) registering the `specflow_ask` tool; it is
  materialized at runtime, which keeps single-binary distribution.
- Tests use the scripted `testdata/fake-pi`; the real-Pi smoke test is opt-in
  with `PI_SMOKE=1`.
- An adapter may implement the optional `harness.CatalogProvider`; the harness
  listing then exposes models, thinking levels, and tools to the editor. Do not
  widen `Adapter` for this, and keep a catalog failure non-fatal (`catalogError`
  on the listing). Pi reads its agent configuration for the catalog; it does not
  start a runtime.
- Every failure carries one class from a fixed set: `adapter_missing`,
  `unsupported`, `config`, `auth`, `provider`, `start`, `protocol`,
  `harness_exit`, `timeout`, `cancelled`. Authentication and configuration
  failures are not retryable; provider, start, protocol, harness-exit, and
  timeout failures are.
- An in-band error is not a completion: when Pi settles after an assistant
  message with an error stop reason, the execution fails with that message even
  though the process exited normally.
- Every bound skill is confirmed with `get_commands` before prompting; a
  directory Pi accepts but does not load (for example a `SKILL.md` without a
  description) fails as configuration, and a Pi too old to answer degrades to a
  warning.
- With several skills Pi loads all paths but only expands one `/skill:` command,
  so the adapter inlines every `SKILL.md` body in Pi's skill-block shape before
  the node prompt. A single skill keeps the native `/skill:` expansion.
- Pi sends a tool's arguments on `tool_execution_start` and omits them from
  `tool_execution_end`; the translator remembers them per call so `write` and
  `edit` still produce file-change telemetry.
- Pi's dialog protocol carries no description for `select`, `input`, or
  `editor`, so the translator folds the `specflow_ask` call's `question`
  argument into the ask's message; `confirm` keeps the dialog's own message.
  Tool summaries are bounded at the stream's 16k summary limit.
- The real-Pi ladder is opt-in with `PI_E2E=1`; the full `standard-feature`
  workflow additionally needs `PI_E2E_FULL=1` and a cheap `PI_E2E_MODEL`.

### Orchestrator

- A block is configuration, not a type: `agent` runs through an adapter,
  `command` runs a shell command in the node's working directory. The OpenSpec
  lifecycle lives in `workflows/*.yaml`, not in Go.
- Node success is a postcondition checked against the repository
  (`file_exists`, `new_change`, `tasks_complete`, `exit_zero`), not an exit
  code. A postcondition failure is distinct from a harness failure.
- Retry is forward (the failure is added to the prompt) or clean (the worktree
  returns to the node's checkpoint). A failed run resumes at the failed node;
  completed nodes are never re-executed.
- A persisted run left `running`, `waiting_approval`, or `waiting_answer` by a
  previous process is repaired the first time this process observes it: the
  interrupted node fails with a retryable `harness_exit` reason naming the
  restart, later pending nodes are skipped, the pending interaction is cleared,
  the run ends failed, and the events are sealed. Repair is lazy (there is no
  boot-time project registry), serialized by `reconcileMu`, and assumes one
  server per checkout.
- A node's execution timeout measures active work: it pauses while a question
  is pending and resumes when the question resolves, so waiting for the
  operator never spends the node's budget. Questions and approval gates carry a
  deadline only when `ask_timeout` or `approval_timeout` configures one.
- A workflow is compiled before a run: defaults, model sentinels, skill paths,
  and ordering are resolved once. A missing skill fails creation rather than a
  node, and the engine never consults workflow defaults.
- Run metadata lives in `.specflow/runs/<id>/` as `run.json`, `events.jsonl`,
  and the retained `workflow.yaml`. The change baseline is captured before the
  run so a resumed `new_change` check still works.
- Run events are sequenced per run and replayable. On a terminal status the
  engine seals every subscriber, the SSE stream delivers the terminal event and
  ends, and a reconnect at the tail gets `204` rather than an endless
  EventSource loop. Live streams send a heartbeat while idle.
- The event hub keeps a bounded replay cache and reopens `events.jsonl` on
  demand. `GET /api/runs/{id}` never opens the journal, and terminal runs are
  evicted from the live set. The journal is repaired at open; an append failure
  is logged, not fatal. On-disk retention is not truncated — discarding the run
  removes it.
- The engine runs one node at a time. Independent branches are representable
  but not concurrent yet.
- The canonical template passes `{{objective}}` into explore, propose, and
  apply, and `{{change}}` into apply, archive, and merge. Its verify gate
  validates the run's change: `openspec validate "{{change}}" --strict && make
  test`; `--changes` is the repository-wide gate. Bare `openspec validate`
  reports nothing to validate even when a change is active.

## Conventions

### Go

- Wrap errors with context: `fmt.Errorf("execute node %s: %w", node.ID, err)`.
- Thread `context.Context` through everything that can block; use it for
  cancellation, not background goroutines.
- Pass events over channels; never block the UI stream on a slow consumer.
- Keep `internal/` internal. Exported names use full words:
  `HarnessAdapter`, `WorkflowRun`, `NodeExecution`.
- Table-driven tests next to the package; `httptest` for handlers.

### TypeScript / React

- One component per file, named after the component.
- The workbench keeps two data flows apart: the run record from REST
  (`GET /api/runs/{id}`) is the only source of node and run state, and the SSE
  stream is narrative. Lifecycle events trigger a coalesced refetch; detail
  events never construct state.
- The event projection (`frontend/src/projection`) is a pure reducer: delta
  coalescing, tool-call pairing, and `seq` idempotency are tested without a DOM.
  Keep new rendering logic in components, not in the reducer.
- The shell owns navigation. Workspace views receive the current object and its
  project plus a `navigate` callback; do not read `window.location` inside a
  view or duplicate the catalog queries. Leaf reads live in the expanded
  project node (one `useRuns`/`useWorkflows` pair), refetch on project change
  and window focus, never on a timer; a live run's row follows the
  REST-refreshed run record. The project registry, the active project, and the
  panel's collapsed flag are the only shell state in `localStorage`; the run
  view keeps its activity-split width under `specflow.runSplit` through the
  typed helpers in `frontend/src/lib/storage.ts`.
- The shell reads `GET /api/server` on mount and on window focus: Settings shows
  the running build and its checkout state, and the workbench warns when the
  running build lags the checkout or predates its backend sources. Identity is
  best-effort — a failed read is not an error state — and the warning is
  dismissed per server process in memory, not in storage. `make dev` compiles
  the API once, so restart it after Go changes; the warning exists because
  forgetting is easy.
- `zustand` is the chosen state library. It is justified by the EventSource
  writer living outside React and by selector-scoped subscriptions; do not add a
  second state library.
- Graph topology comes from the pinned workflow (`GET /api/runs/{id}/graph`),
  never from `Run.Nodes` order. Positions are computed deterministically from
  edges; status changes must not move the graph.
- The flow editor transports a structured draft and names the project copy as
  its only write target; Go generates canonical YAML. Do not add a client YAML
  parser/serializer, persist node positions, or let a save write the `workflows/`
  template.
- Editor pickers follow the harness catalog and always keep a manual fallback.
  Model changes clear unsupported thinking values; tools and skills are
  multi-select dropdowns with chips; an empty tool allowlist means the harness
  default; prompt variables are the closed `RunContext` set, not a template
  language.
- A node binds skills through the plural `skills` list. Keep the singular
  `skill` field readable for older files, but do not write both forms; the
  validator rejects a node that sets both.
- A workflow save is guarded by the content hash the editor opened with; keep
  the `409` conflict path instead of adding a silent overwrite.
- Use the typed API client in `frontend/src/api`; no ad-hoc `fetch` outside it.
  `pending` is the source of truth for interactions, and a question is rendered
  only after its `question.asked` kind is known.
- Tests run with Vitest and Testing Library (`frontend/src/test` provides the
  harness). `make test` runs `go test ./...`, the frontend typecheck, and the
  frontend suite; pure projection, layout, and state logic must stay testable
  outside a browser.

### Commits

Conventional prefixes, scoped where useful:

```text
feat(orchestrator): add parallel branch scheduler
feat(ui): stream node thinking events
fix(pi-adapter): handle SIGTERM gracefully
docs(agents): update the adapter contract
test(e2e): cover explore-to-archive flow
build(release): add darwin/arm64 target
```

### Commit cadence

Archive the change before committing. The implementation, the synced main
specs, and the archive move land in one commit per change — not a commit after
apply and another after archive.

## Commands

```bash
make install   # pnpm install --frozen-lockfile + go mod download
make dev       # API :8080 + Vite :5173 (proxies /api)
make build     # frontend build -> go build -> ./specflow
make run       # build, then run the binary
make test      # go test ./... + frontend typecheck + frontend tests
make clean     # remove the binary and embedded assets (keeps .gitkeep)
```

## OpenSpec workflow

- `openspec list` — in-flight changes.
- `openspec list --specs` — durable capabilities.
- `openspec status --change "<name>" --json` — artifact progress.
- `openspec validate <name> --strict` — the gate before archive.
- Change an existing plan with `/openspec-update-change`; implement with
  `/openspec-apply-change`; archive with `/openspec-archive-change`.

The OpenSpec skills (`openspec-explore`, `openspec-propose`,
`openspec-update-change`, `openspec-sync-specs`, `openspec-apply-change`,
`openspec-archive-change`) are installed by the OpenSpec CLI, not vendored in
this repository: `openspec init --tools dsh` (or `pi`, `claude`, `codex`, …).
See `CONTRIBUTING.md`.

## Non-goals

- Not a new agent harness; we orchestrate existing ones.
- Not a generic no-code automation tool; scope is spec-driven development.
- No auth, billing, or multi-user support in the MVP.
- No Docker requirement for basic usage.

## Before you change something

Ask:

1. Does this complicate the harness adapter contract?
2. Does it keep the single-binary build working?
3. Is there a spec that already covers this behavior, and does it need a delta?
4. Can this run in parallel, or does it depend on ordering?
5. Did `make test` and `openspec validate --changes --strict` pass?
