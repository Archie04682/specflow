# Architecture

SpecFlow is one Go binary with an embedded React workbench. The orchestrator
owns graph state, isolation, and verification; harness adapters own the agent
loop and tool execution.

```text
+-------------------------------------------------------+
|  React workbench: project tree, run view, flow editor |
+---------------------------+---------------------------+
                            | REST + SSE (WebSocket for Q&A)
+---------------------------v---------------------------+
|  Go orchestrator: DAG engine, sessions, OpenSpec      |
|  mapping, embedded UI                                 |
+---------------------------+---------------------------+
                            | harness protocol (JSON events)
+---------------------------v---------------------------+
|  Adapters: Pi first; other harnesses later            |
+-------------------------------------------------------+
```

## Components

| Package | Responsibility |
| :--- | :--- |
| `backend/cmd/api` | Server entrypoint: flags, port resolution, build identity, shutdown |
| `backend/internal/api` | HTTP router, handlers, SSE, SPA fallback |
| `backend/internal/orchestrator` | Workflow model, graph validation, engine, postconditions, retry, run store, reconciliation |
| `backend/internal/workspace` | Project layout, `.specflow` paths, git worktrees, checkpoints, merge |
| `backend/internal/harness` | Adapter contract, registry, preflight, scripted test harness |
| `backend/internal/harness/pi` | Pi adapter: RPC transport, event translation, ask bridge |
| `backend/internal/openspec` | Skill resolution, change and task helpers for gates |
| `backend/internal/session` | Harness host: events, questions, injections |
| `backend/web` | `go:embed` of the built frontend |
| `frontend` | Vite + React + TypeScript workbench |

The orchestrator never learns harness-specific details. An adapter returns
exactly one terminal result per execution, and telemetry is best-effort.

## Run lifecycle

```text
POST /api/runs
  |
  v
resolve project + workflow
  |
  v
compile: defaults, `default` model sentinel, skill paths, graph validation
  |                                    (a missing skill fails here)
  v
snapshot the definition to .specflow/runs/<id>/workflow.yaml
  |
  v
create worktree .specflow/worktrees/<id> on specflow/run-<id>
  |
  v
+--> node.started
|      |
|      v
|    agent node  -> adapter execution (streamed events)
|    command node-> shell command in the node's working directory
|      |
|      v
|    postcondition checked against the repository
|      |
|      v
|    checkpoint commit  -> next node
|
+--- on failure: retry forward / clean, or resume at the failed node
  |
  v
run.completed | run.failed | run.cancelled
```

A node's success is a postcondition, not an exit code. A postcondition failure
is distinct from a harness failure and is not retried automatically.

## Isolation

- Every run executes in `.specflow/worktrees/<id>` on `specflow/run-<id>`.
- The worktree checkpoints after every node, so a node's changes are a commit
  and its diff is `git diff <pre>..<post>`.
- Nothing in the user's checkout moves until the approval-gated merge node runs
  in the project checkout.
- The merge node runs with `workdir: project` because the worktree already has
  the run branch checked out and cannot also check out the merge target.
- `.specflow/worktrees/` is never version-controlled; run metadata
  (`.specflow/runs/<id>/`) may be committed if a project wants run history.

## Single binary

```text
frontend/src  --pnpm build-->  backend/web/dist  --go:embed-->  ./specflow
```

Vite writes directly into `backend/web/dist`, and `backend/web/embed.go` embeds
that directory. The committed `backend/web/dist/.gitkeep` keeps `go:embed` valid
on a fresh clone before the first frontend build, so `make build` always builds
the frontend first.

## Repository layout

```text
backend/cmd/api              server entrypoint
backend/internal/api         HTTP router, handlers, SPA fallback
backend/internal/config      port resolution
backend/internal/orchestrator  workflow model, engine, postconditions, retry
backend/internal/workspace   project, .specflow layout, worktree, checkpoints
backend/internal/harness     adapter contract, registry, preflight, scripted mock
backend/internal/harness/pi  Pi adapter: RPC transport, translation, ask bridge
backend/internal/openspec    skill resolution, change and task helpers for gates
backend/internal/session     harness.Host: events, questions, injections
backend/web                  go:embed of the built frontend
frontend                     Vite + React + TypeScript
workflows                    reusable workflow templates
openspec                     specs and in-flight changes
docs                         the public manual
```

## Where truth lives

- `docs/` — the public manual: architecture, workflows, API, UI, harnesses,
  development.
- `AGENTS.md` — decisions that bind contributors and agents working on the code.
- `openspec/specs/` — durable capability requirements.
- `openspec/changes/` — in-flight and archived changes, with the design record
  for each one.
