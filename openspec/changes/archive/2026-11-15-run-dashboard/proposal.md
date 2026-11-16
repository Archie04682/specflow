# Proposal

## Why

SpecFlow's premise is watching an agent work and intervening at the right moment, but the backend can only be driven with `curl`. The engine, worktree isolation, postconditions, interactive waits, and the SSE stream are proven end to end against real Pi; the product still has no user-facing surface. The dashboard is the first real consumer of the event stream, and building it now shows whether the machine-shaped event contract is actually renderable by a person under time pressure.

## What Changes

- Replace the placeholder `frontend/` app with the Run Dashboard SPA: a start screen (project, workflow, objective), a React Flow graph whose nodes track execution state, a live event stream, an interaction dock for questions, injections, and approvals, run controls, run history, and a completed-run review. Dark theme, desktop-first, tablet-usable.
- Add two read-only HTTP endpoints the dashboard cannot work without:
  - `GET /api/workflows?project=` lists the project's workflow files with their structural topology, without compiling them.
  - `GET /api/runs/{id}/graph?project=` returns the topology pinned to a run, read from the retained workflow snapshot, without resolving skills.
- Render the real event taxonomy rather than the brief's assumed one: dotted lifecycle events plus `node.event` records whose `data.kind` discriminates thinking, text, tool calls, file changes, usage, notices, and status lines. Token deltas are coalesced into readable blocks; the stream never constructs run state.
- Keep the project root as a client-held absolute path in `localStorage`; every request carries it in the existing `project` parameter. No server-owned project, no project registry.
- Add a frontend test suite (component and pure-logic tests) and wire it into `make test`.
- Update the README and AGENTS.md with the dashboard workflow, the new endpoints, and the UI testing procedure.

**Not included:** workflow editing or drag-and-drop graph changes, a diff endpoint, harness configuration, authentication, settings, run discard, a server default project, or any change to run execution, persistence, the compiled plan, or the event taxonomy.

## Capabilities

### New Capabilities

- `run-dashboard`: the dashboard's user-visible behavior — starting a run, rendering workflow state, streaming and rendering events, handling the three interactions, run controls, history and detail, and the error/loading/responsive contract.

### Modified Capabilities

- `workflow-orchestration`: add the workflow catalog and run graph read surfaces, including their shapes, ordering, error outcomes, and the rule that both are structural projections that never compile, resolve skills, or mutate a run.

## Impact

- `backend/internal/api`: new workflow-catalog and run-graph handlers and their `httptest` coverage; the existing run endpoints are unchanged.
- `backend/internal/orchestrator`: read-only projections for workflow discovery and a run's pinned definition; no engine, worktree, postcondition, retry, or event changes. No `run.json` field changes.
- `frontend/`: the dashboard source tree, styles, and tests. New runtime dependencies `@xyflow/react` and `zustand`; new dev dependencies `vitest`, `jsdom`, and Testing Library. The built assets still land in `backend/web/dist`, so the single-binary build is unchanged.
- `Makefile`: `make test` gains the frontend test run alongside `go test ./...` and the typecheck.
- `README.md` and `AGENTS.md`: dashboard usage, the two endpoints, and the UI test procedure.
- Operational note: the dashboard is an unauthenticated local tool that can cancel and approve runs. It exposes no new mutation surface — it only makes the existing one usable.
