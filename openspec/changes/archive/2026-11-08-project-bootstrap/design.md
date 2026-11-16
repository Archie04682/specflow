# Design

## Context

See `proposal.md` — Why. This is a greenfield repository: `openspec/` exists, there is no source code, no `go.mod`, and no git history. The only prior art is the ideation chat, whose final message contains a full `AGENTS.md` draft and a proposed tree.

Two constraints shape everything below:

- **Toolchain on the target machine:** Go 1.27.2, Node 26.12, pnpm 10.18.2, git 2.56. `pi` is not installed — that does not matter yet, because this change ships no harness adapter.
- **The publish path is unknown**, so the module path is deliberately a placeholder.

## Goals / Non-Goals

**Goals:**

- A fresh clone can run `make dev`, `make build`, and `make test` with no manual steps beyond installing Go and Node.
- `make build` produces `./specflow`, and running it serves the UI and the API with no Node process alive.
- The four requirements in `single-binary-distribution` are covered by Go tests.
- `AGENTS.md` is short enough to be loaded into every agent context, and accurate against the skills this project actually has.

**Non-Goals:**

- No orchestrator, DAG engine, session manager, or harness adapter — only empty package boundaries with a doc comment.
- No graph editor, React Flow, or state library.
- No CI, release automation, cross-compilation matrix, or installers.
- No auth, persistence, or telemetry.
- No `openspec/config.yaml` project context (a later, separate change).

## Decisions

### D1 — Module `example.com/specflow` at the repository root

`go.mod` lives at the repo root and declares `module example.com/specflow`. Go sources live under `backend/`, so imports read `example.com/specflow/backend/...`.

- **Why not `backend/go.mod`:** the module path would have to become `…/specflow/backend` once published, which does not match the repository URL. A root `go.mod` makes module path and repo path the same value.
- **Why not commit the real path now:** the publish target is undecided. `example.com/...` signals "placeholder" unambiguously and cannot be mistaken for a resolvable import.
- **Cost, stated honestly:** changing the module path is not a one-line edit. `go mod edit -module` does not rewrite imports, so every self-import must be rewritten too. Mitigation: perform the rename before the package count grows, with a single `grep -rl` + `sed` pass, and verify with `go build ./...`.
- **Rejected:** a dotless module name (`specflow`) — legal but confusing, and it breaks the `go install` story the chat draft relies on.

### D2 — `backend/` and `frontend/` split, not a flat Go tree

Keep the two top-level directories from the ideation chat. The Go module root is the repo root, but Go packages stay inside `backend/`.

- **Why:** the two halves have different toolchains, different CI jobs later, and different owners. A flat `cmd/`/`internal/`/`web/` root would bury the frontend next to Go internals.
- **Cost:** every import carries a `backend/` segment. Accepted for the clearer boundary.

```
specflow/
├── go.mod                  module example.com/specflow
├── AGENTS.md
├── README.md
├── Makefile
├── .gitignore
├── backend/
│   ├── cmd/api/main.go
│   ├── internal/           orchestrator, harness, openspec, session, api
│   └── web/                embed.go + dist/.gitkeep
├── frontend/               Vite + React + TS
├── workflows/              standard-feature.yaml
└── openspec/               already present
```

`internal/` is deliberate: these packages are not a public API, and Go enforces that for free. Each package gets a one-paragraph `doc.go` naming its future responsibility, so the empty directories are self-explanatory rather than mysterious.

### D3 — Vite writes directly into `backend/web/dist`

`vite.config.ts` sets `build.outDir: "../backend/web/dist"` and `emptyOutDir: false`. `backend/web/embed.go` uses `//go:embed all:dist`.

- **Why not a copy step:** a `cp -r frontend/dist backend/web/dist` in the Makefile creates two sources of truth and a stale-copy failure mode. Building straight into the embed target removes the copy.
- **Why `emptyOutDir: false`:** the directory must exist at compile time, even on a fresh clone where the frontend has never been built. A committed `backend/web/dist/.gitkeep` satisfies `go:embed`; emptying the directory would delete it. `.gitignore` ignores `backend/web/dist/*` but negates `.gitkeep`.
- **Cost:** hashed bundles from previous builds accumulate. `make clean` prunes them; this is invisible to users because `index.html` is overwritten each build and references only the current hashes.
- **Rejected:** `emptyOutDir: true` + a Makefile that recreates `.gitkeep` — breaks plain `go build`/`go test` on a fresh clone, which is exactly when an agent runs them.
- **Rejected:** generating the embed directory only at build time — same failure, worse.

### D4 — Separate API mux from the SPA catch-all

`http.ServeMux` (Go 1.22+ patterns):

```go
mux.Handle("GET /api/health", healthHandler())
mux.Handle("/api/", jsonNotFound())        // unknown API paths never reach the UI
mux.Handle("/", spaHandler(web.StaticFS))  // files, then index.html fallback
```

- **Why:** the spec requires unknown `/api/*` paths to return JSON `404`, not the SPA shell. Registering an explicit `/api/` subtree makes that structural instead of an `if` inside the file server.
- **Why `all:dist`:** without `all:`, `embed` skips files whose names start with `.` or `_`; Vite can emit such assets.
- **SPA fallback rule:** for `GET`/`HEAD` requests that are not under `/api/` and do not map to an embedded file, serve `dist/index.html` with `200`. Non-GET misses return `404`.

### D5 — Port precedence: flag > `PORT` > `8080`

Parse with the standard `flag` package; track whether `--port` was explicitly set; fall back to `PORT`; default `8080`. Validate `1..65535` and exit non-zero naming the offending value.

- **Why flag over env:** a user debugging two instances reaches for a flag; env is for containers and CI.
- **Rejected:** a config file — premature for a single integer.

### D6 — pnpm for the frontend

pnpm 10 is already installed and the lockfile is committed.

- **Why not npm:** slower, larger `node_modules`, no practical benefit here.
- **Rejected:** Yarn — nothing in the design needs it.
- The `Makefile` calls `pnpm` explicitly; `packageManager` in `package.json` pins the major so Corepack users get the same version.

### D7 — `AGENTS.md` is a context file, not the product spec

Target ~150–200 lines. Move the ideation chat's roadmap, release matrix, and long protocol excerpts into `README.md` (product) and `docs/` (later), keeping in `AGENTS.md` only what an agent must hold in working memory: architecture summary, the decisions above, conventions, commands, non-goals, and where truth lives (`openspec/`).

Two corrections to the chat draft:

- It names `openspec-verify-change`, which **does not exist** in this environment. The installed skills are `openspec-explore`, `openspec-propose`, `openspec-update-change`, `openspec-sync-specs`, `openspec-apply-change`, `openspec-archive-change`. The Verify step becomes a validation gate (`openspec validate --strict` plus the test suite), and the Apply step is named `openspec-apply-change`.
- It dates the file and marks the status "Draft / Active Development". A date in a context file rots; drop it, and state the current phase instead.

### D8 — Tests cover the spec scenarios, not the framework

`backend/internal/api` gets table-driven `httptest` tests for: health JSON, SPA fallback, unknown API `404`, and port resolution precedence. `make test` runs `go test ./...` and `pnpm build` (typecheck). No frontend test runner yet — there is no behavior to test beyond the build.

## Risks / Trade-offs

- **Placeholder module path leaks into imports.** Every new package until publish uses `example.com/specflow/...`. → Rename in one pass before Phase 1 feature work; add the rename command to `README.md` so it is not folklore.
- **`go:embed` couples compilation to the frontend build.** A missing/empty `dist` fails `go build`. → Committed `.gitkeep`, `emptyOutDir: false`, and `make build` always builds the frontend first.
- **Stale hashed assets in `dist`.** → `make clean`; harmless because `index.html` is regenerated.
- **The UI is a placeholder.** The single-binary claim is proven, but there is no product behavior yet. That is intentional for a bootstrap and must not be mistaken for progress on the orchestrator.
- **No `pi` binary on the machine.** Harness work cannot be smoke-tested yet; the adapter interface is deferred to its own change.
- **`AGENTS.md` drifts from the skills.** → It points at `openspec/` and the installed skill names rather than duplicating lifecycle details.

## Migration Plan

Greenfield: no existing code or users. Apply creates files and initializes git. Rollback is `git revert` of the bootstrap commit (or deleting the tree before it is pushed) — nothing external depends on it.

## Open Questions

- **Real module path.** Deferred by the user; `example.com/specflow` is explicit placeholder until publish. Resolve before any external import or `go install` instruction ships.
- **Whether `make dev` should also run a harness process.** Out of scope now; revisit with the Pi adapter change.
