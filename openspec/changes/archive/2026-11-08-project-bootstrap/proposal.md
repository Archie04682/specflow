# Proposal

## Why

SpecFlow exists only as a design conversation: the architecture is settled, but there is no repository to build on. Every future change — the DAG orchestrator, harness adapters, the graph UI — needs a home, a build pipeline, and shared conventions, or each one will invent its own.

This change creates a runnable skeleton and proves the riskiest assumption on day one: that a single Go executable can serve the React UI and the HTTP API from one process. That is the product's distribution promise, and it is cheap to validate now and expensive to discover broken after the orchestrator exists.

## What Changes

- Initialize the Go module as `example.com/specflow` — a placeholder path, to be replaced with the real repository path at publish time. The module sits at the repository root so that swap touches exactly one line.
- Add a `backend/` skeleton: `cmd/api` entrypoint, `internal/` package boundaries (`orchestrator`, `harness`, `openspec`, `session`, `api`), and `web/embed.go` embedding the built frontend.
- Implement the minimal shell: `/api/health`, static serving of the embedded UI at `/`, SPA fallback, and a configurable listen port.
- Add a `frontend/` skeleton: React + TypeScript + Vite, a placeholder App, and a dev proxy from `/api` to the backend.
- Add `Makefile` (`dev`, `build`, `run`, `test`, `clean`), `.gitignore`, `README.md`, and `AGENTS.md`.
- Add `workflows/standard-feature.yaml` as the canonical Explore → Propose → Apply → Review → Tests → Verify → Archive template.
- No product features: no orchestrator logic, no harness adapters, no graph editor.

## Capabilities

### New Capabilities

- `single-binary-distribution`: the system is delivered as one executable that serves the embedded web UI and the HTTP API from the same process, with a configurable port and a health endpoint.

### Modified Capabilities

None — this is the first change; the project has no specs yet.

## Impact

- **New files only.** There is no existing code to modify; `openspec/` is already initialized.
- **Dependencies:** Go standard library only for the backend shell; React, Vite, and TypeScript for the frontend. No runtime dependency on Node after build.
- **Module path:** `example.com/specflow` is intentionally not resolvable. It must be renamed in `go.mod` and the import strings before any `go get` or release.
- **Docs:** `AGENTS.md` becomes the always-loaded agent context and is derived from the ideation chat, not copied verbatim.
