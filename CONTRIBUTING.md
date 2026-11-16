# Contributing

Thanks for taking a look. SpecFlow is at an early stage, so the most useful
contributions are bug reports, focused fixes, and new harness adapters.

## Before you start

- Read [docs/architecture.md](docs/architecture.md) and
  [AGENTS.md](AGENTS.md) — the latter records decisions that bind the code.
- Behavior changes start as an OpenSpec change under `openspec/changes/`.
  Documentation, cleanup, and test-only changes do not need one.
- Check `openspec list` before opening a large pull request, so two people do
  not implement the same change twice.

## Setup

```bash
make install   # pnpm install --frozen-lockfile + go mod download
make test      # go test ./... + frontend typecheck + Vitest
```

`make build` produces the single `./specflow` binary; see
[docs/development.md](docs/development.md) for the dev loop and build identity.

The OpenSpec skills are not vendored in this repository. Install them for your
harness with the OpenSpec CLI, for example `openspec init --tools dsh` (or `pi`,
`claude`, `codex`, …).

## Gates

A change is ready to merge when:

```bash
openspec validate <change> --strict
make test
```

Both must pass. For a repository-wide check, run
`openspec validate --changes --strict`.

## Commits

Conventional prefixes, scoped where useful:

```text
feat(orchestrator): add parallel branch scheduler
feat(ui): stream node thinking events
fix(pi-adapter): handle SIGTERM gracefully
docs(agents): update the adapter contract
test(e2e): cover explore-to-archive flow
build(release): add darwin/arm64 target
```

Archive the OpenSpec change before committing. The implementation, the synced
main specs, and the archive move land in one commit per change — not a commit
after apply and another after archive.

## Pull requests

- Keep the diff focused; separate cleanup from behavior changes.
- Say which harness you tested with, if the change touches an adapter.
- `make test` must be green, and CI runs the same suites.
- Do not commit `.dsh/`, `.specflow/`, build output, or agent run journals;
  `.gitignore` already covers them.

## License

By contributing, you agree that your contribution is licensed under the
[MIT License](LICENSE).
