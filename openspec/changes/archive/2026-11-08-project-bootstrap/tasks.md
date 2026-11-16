# Tasks

## 1. Repository initialization

- [x] 1.1 Initialize git in the repository root and confirm the working tree is a repository (`git init`; `git status` exits 0 and reports "No commits yet").
- [x] 1.2 Write `.gitignore` for `node_modules/`, `backend/web/dist/*` with `!backend/web/dist/.gitkeep`, the `specflow` binary, `*.test`, `coverage.out`, `.env*`, `.DS_Store`, `.idea/`, `.vscode/`; verify with `git check-ignore -v frontend/node_modules/x` (ignored) and `git check-ignore -v backend/web/dist/.gitkeep` (not ignored, exit 1).
- [x] 1.3 Create `backend/web/dist/.gitkeep` so the embed package compiles before the first frontend build; verify `git status --short` lists it as untracked and not ignored.

## 2. Go module and entrypoint

- [x] 2.1 Create `go.mod` at the repository root with `module example.com/specflow` and the installed Go version; verify `go list -m` prints `example.com/specflow` and `go build ./...` succeeds.
- [x] 2.2 Add `backend/internal/config/port.go` resolving the port with precedence `--port` > `PORT` > `8080` and rejecting values outside 1–65535; verify `go test ./backend/internal/config/` passes table cases for default, flag, env, precedence, and invalid input.
- [x] 2.3 Add `backend/cmd/api/main.go` parsing `--port`, delegating to `config`, and logging the resolved listen address; verify `go run ./backend/cmd/api --port 0` exits non-zero naming the value, and `--port 18080` starts and logs `:18080`.
- [x] 2.4 Add `doc.go` files for `internal/orchestrator`, `internal/harness`, `internal/openspec`, and `internal/session`, each naming the package's future responsibility; verify `go build ./...` and `go vet ./...` pass with no empty-directory gaps.

## 3. Frontend skeleton

- [x] 3.1 Scaffold `frontend/` (`package.json` with `packageManager: pnpm@10`, `tsconfig.json`, `index.html`, `src/main.tsx`) and `vite.config.ts` with `build.outDir: "../backend/web/dist"`, `emptyOutDir: false`, and a dev proxy from `/api` to `http://localhost:8080`; verify `pnpm install` completes and `pnpm exec tsc --noEmit` passes.
- [x] 3.2 Replace the Vite starter with a placeholder `src/App.tsx` that renders the SpecFlow name and the result of `GET /api/health`; verify `pnpm build` succeeds and `backend/web/dist/index.html` references a hashed JS bundle.
- [x] 3.3 Confirm the build preserves the embed sentinel: after a second `pnpm build`, verify `backend/web/dist/.gitkeep` still exists and `index.html` was regenerated.

## 4. Embedded serving

- [x] 4.1 Add `backend/web/embed.go` with `//go:embed all:dist`; verify `go build ./backend/web` succeeds and `go vet ./backend/web` passes.
- [x] 4.2 Implement the health handler and the JSON not-found handler for unknown `/api/` paths in `backend/internal/api`; verify `go test ./backend/internal/api/` passes both scenarios (`/api/health` returns `200` with `"status":"ok"`, `/api/nope` returns `404` JSON).
- [x] 4.3 Implement the SPA handler that serves embedded files and falls back to `index.html` for unknown non-API GET paths, returning `404` for other methods; verify tests cover a fresh client route (`/flows/example` → `200` HTML) and a non-GET miss (`POST /nope` → `404`).
- [x] 4.4 Wire the server in `main.go`: `http.Server` on the resolved port serving the API router plus the embedded FS, with shutdown on SIGINT/SIGTERM; verify `go run ./backend/cmd/api --port 18080` answers `curl -s localhost:18080/api/health` and `curl -s localhost:18080/` returns the app HTML.

## 5. Build pipeline and documentation

- [x] 5.1 Add `Makefile` targets `install`, `dev`, `build`, `run`, `test`, `clean`, where `build` runs the frontend build before `go build`; verify `make build` produces `./specflow` and `make test` runs `go test ./...` plus the frontend typecheck.
- [x] 5.2 Verify single-binary behavior: copy `./specflow` to an empty temporary directory, run it there, and confirm `curl /` and `curl /api/health` succeed with no Node process and no source tree present.
- [x] 5.3 Write `README.md` with the quickstart, `make` targets, repository layout, and the module-path rename procedure (`go mod edit -module` plus the `grep -rl example.com/specflow | xargs sed -i` pass); verify every command in it runs as written.
- [x] 5.4 Document the development workflow (backend on `:8080`, Vite on `:5173` proxying `/api`); verify `make dev` starts both and `curl -s localhost:5173/api/health` returns `"status":"ok"`.

## 6. Agent context and workflow template

- [x] 6.1 Write `AGENTS.md` (~150–200 lines): product summary, architecture, decisions distilled from `design.md`, conventions, commands, non-goals, and where truth lives; verify `wc -l` is under 220 and every OpenSpec skill it names exists under `.dsh/skills/` (no `openspec-verify-change`).
- [x] 6.2 Add `workflows/standard-feature.yaml` with Explore → Propose → Apply → Review → Tests → Verify → Archive nodes carrying `skill`, `harness`, `model`, `interactive`, and `allow_inject`; verify it parses with `ruby -ryaml -e 'YAML.load_file("workflows/standard-feature.yaml")'`.

## 7. Integration verification

- [x] 7.1 Commit the bootstrap (`git add -A && git commit`) and verify `git log --oneline -1` shows the commit and `git status` is clean.
- [x] 7.2 Verify `openspec validate project-bootstrap --strict` passes with no warnings.
- [x] 7.3 Fresh-clone check: `git clone . /tmp/specflow-bootstrap-check && cd /tmp/specflow-bootstrap-check && make test && make build`; verify both targets succeed from the clone alone.
- [x] 7.4 Run the built binary on the default port and verify all four `single-binary-distribution` scenarios: `GET /` (HTML), `GET /flows/example` (HTML), `GET /api/health` (JSON `ok`), `GET /api/nope` (JSON `404`).

## Workflow follow-up

- Archive this change after review and a green `openspec validate --strict`.
- Replace `example.com/specflow` with the real repository path in a dedicated change before any publish or external import.
