# Development

## Prerequisites

- Go 1.27 or newer
- Node.js 22 or newer
- pnpm 10 — `corepack enable` picks up the version pinned in
  `frontend/package.json`
- The [OpenSpec](https://github.com/Fission-AI/OpenSpec) CLI for the canonical
  workflow's verify and archive steps
- Optional: [Pi](https://pi.dev) on `PATH` with credentials, to execute agent
  nodes. Building and testing do not need it.

## Make targets

| Target | What it does |
| :--- | :--- |
| `make install` | `pnpm install --frozen-lockfile` and `go mod download` |
| `make dev` | Runs the API and the Vite dev server together |
| `make build` | Builds the frontend into `backend/web/dist`, then compiles `./specflow` |
| `make run` | `make build`, then runs the binary |
| `make test` | `go test ./...`, the frontend typecheck, and the frontend test suite |
| `make clean` | Removes the binary and the embedded frontend assets (keeps `.gitkeep`) |

`make install` uses `--frozen-lockfile`, so a fresh clone fails loudly instead of
silently changing `pnpm-lock.yaml`.

## Dev loop

```bash
make dev
# backend: http://localhost:8080
# UI:      http://localhost:5173  (/api proxied to :8080)
```

Running the two processes in separate terminals works, but only `make dev` and
`make build` stamp the build identity, so a bare `go run` reports a partial one:

```bash
go run ./backend/cmd/api --port 8080
```

```bash
cd frontend && pnpm dev
```

`make dev` compiles the API once. Restart it after Go changes, or the workbench
warns that the running build is behind the checkout.

## Which build is the API running?

`GET /api/server` names the running build, so a page can never be confused about
which engine is answering it:

```bash
curl -s localhost:8080/api/server
```

The response carries the service name, the build stamps (version, revision,
revision time, dirty flag, build time), and the process start time. When the
process was launched from a SpecFlow checkout the response also carries a
`checkout` object with that checkout's current revision and, if the running build
is behind it, a `staleReason`:

- `checkout-ahead` — the checkout has a commit the running build does not
  include.
- `sources-newer` — a backend source or `go.mod`/`go.sum` is newer than the
  build.

The revision, revision time, dirty flag, and build time are injected with
`-ldflags` in `make dev` and `make build`, because the Go toolchain records no
version-control settings for a `go run` binary; the version comes from the Go
toolchain's module version. Fields the build cannot determine are omitted, and
the endpoint never fails for a missing stamp or an unreadable checkout.

## Tests

`make test` runs three suites:

1. `go test ./...` — orchestrator, workspace, harness (against the scripted Pi
   stand-in), API handlers, build identity.
2. `pnpm exec tsc --noEmit` — the frontend typecheck.
3. `pnpm test` — Vitest and Testing Library.

Projection, layout, and state logic must stay testable outside a browser. The
real-Pi smoke test and the e2e ladder are opt-in; see
[Harnesses](harnesses.md).

## Skills for contributors

This repository does not vendor harness skills. Install the OpenSpec skills for
the harness you use with the OpenSpec CLI, for example:

```bash
openspec init --tools dsh      # or: pi, claude, codex, ...
```

The product resolves skills from `.specflow/skills`, `.dsh/skills`,
`.pi/skills`, then `~/.dsh/skills`; a `skills` list in `.specflow/config.yaml`
replaces that order.

`.dsh/` and `.specflow/` are local-only and gitignored. `.specflow/` is recreated
on demand, and a missing `config.yaml` means an empty configuration.

## Continuous integration

`.github/workflows/ci.yml` runs on pushes to `main` and on pull requests:

- Go tests with the toolchain selected from `go.mod`
- `pnpm install --frozen-lockfile`
- the frontend typecheck and the Vitest suite

CI does not need Pi, the OpenSpec CLI, or network access to a model provider.
