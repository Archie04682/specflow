# Tasks

## 1. Embedded templates

- [x] 1.1 Add a repo-root package (for example `templates.go`, `package specflow`) with `//go:embed workflows/*.yaml` and an accessor returning the template files by reference. Verify a test lists the embedded references and asserts the `standard-feature` bytes are identical to `workflows/standard-feature.yaml`.
- [x] 1.2 Add a second embedded template only if the change needs one; otherwise assert the accessor reports exactly the files in `workflows/`. Verify `go build ./...` and `go test ./...` pass from a clean checkout with no generation step.

## 2. Shared skill roots

- [x] 2.1 Extend `workspace.defaultSkillRoots` to `.specflow/skills`, `.agents/skills`, `.dsh/skills`, `.pi/skills`, `~/.dsh/skills`, `~/.agents/skills`, and update the comment in the shipped `.specflow/config.yaml` so a new project's file names the same defaults. Verify `workspace` tests assert the order and that `skill-catalog` resolution tests cover `.agents/skills` winning over `.dsh/skills` and losing to `.specflow/skills`.
- [x] 2.2 Update `docs/workflows.md`'s skill-root section and the `AGENTS.md` decision that lists the roots, so both name the new directories and the shared-project-before-harness rule. Verify every documented order matches `defaultSkillRoots` exactly.

## 3. Readiness report

- [x] 3.1 Add `GET /api/project/readiness?project=` returning the repository check and branch, the `.specflow` presence, the flow list, the resolved and missing skills, the `openspec` CLI and harness findings, offline mode, and the uncommitted paths. Verify handler tests for a fresh git project, a non-git directory (200 with the check failed), and a missing path (400 naming it).
- [x] 3.2 Derive each flow's required skills from its node bindings through `Node.EffectiveSkills`, resolving each name against the project's roots. Verify tests for a missing skill named under its flow, a singular `skill:` binding being counted, and a workflow that fails to parse being listed invalid while the others still resolve.
- [x] 3.3 Report the paths a project has uncommitted and state that a run requires them committed, without writing, staging, or ignoring anything. Verify a test reads readiness twice and asserts the project tree, `git status`, and the report are unchanged.

## 4. Seeding the default flow

- [x] 4.1 Add `POST /api/project/flow` accepting a project and optional reference, writing `.specflow/workflows/<reference>.yaml` from the embedded template and creating the directory. Verify a test seeds an empty project, reads the file back byte-for-byte against the embedded copy, and confirms the catalog lists it as valid.
- [x] 4.2 Refuse to overwrite: an existing file at that path is reported as a conflict with nothing written. Verify a test seeds twice and one that edits the project copy first, asserting the bytes and modification state are unchanged in both.
- [x] 4.3 Document seeding in `docs/workflows.md` and add the root package to the `AGENTS.md` repository layout. Verify the documented endpoint and file path match the handler test's expectations.

## 5. Offline skills source

- [x] 5.1 Implement source discovery over a directory, a `.zip`, a `.tar`, a `.tar.gz`, and a `.tgz`: locate every `SKILL.md` and treat its parent directory as a candidate named by that directory. Verify table-driven tests over a flat directory, a single skill directory, a GitHub-style archive with a `OpenSpec-<tag>/skills/` prefix, and a source with no `SKILL.md` (refused, nothing written).
- [x] 5.2 Confine and bound extraction: reject absolute paths, `..` segments, symlink and hardlink entries, more than 1000 entries, and more than 25 MiB extracted; write only candidate directories, into a scratch directory under `.specflow/`, then copy each into `.agents/skills/<name>`. Verify tests for a traversal entry, a link entry, an oversized archive, and an archive whose non-skill files never reach the project.
- [x] 5.3 Report per skill whether it was installed, unchanged, or present with different content, and replace only when the request asks. Verify tests for a second identical install, a different-content conflict left untouched, an explicit replace, and a partial source whose report lists the still-missing skills.
- [x] 5.4 Validate candidates with the same parser the catalog uses, so a directory that would not appear in `GET /api/skills` is not reported as installed. Verify a test installs a directory with a `SKILL.md` and asserts the catalog lists exactly the names the install reported.

## 6. Online provider and offline mode

- [x] 6.1 Add server configuration for a skills source and offline mode, with `SPECFLOW_SKILLS_SOURCE` / `SPECFLOW_OFFLINE` and their flags, following the existing port-resolution shape in `internal/config`. Verify config tests for default (neither), env-only, flag-only, and flag-over-env, plus rejection of an empty source value.
- [x] 6.2 Put the network decision behind one predicate that both the online install and any future network action consult, and make a set local source make the online branch unreachable. Verify a test asserts no command is constructed when offline mode is active or a local source is configured.
- [x] 6.3 Add `POST /api/project/skills` with an injectable command runner for tests: the online branch runs the `skills` CLI against a pinned OpenSpec release, targeting `.agents/skills`, with telemetry disabled, and returns the exit status and captured output. Verify handler tests with a fake runner for a successful install, a non-zero exit reported with its output, and the refusal under offline mode.
- [x] 6.4 Document offline mode and the skills source in `docs/architecture.md`, and add an `AGENTS.md` bullet that every network-capable action goes through the one predicate. Verify the documented flag precedence and env names match the config tests.

## 7. Onboarding card

- [x] 7.1 Add the typed API client methods and response types for readiness, seeding, and the skills install. Verify `frontend/src/api` tests for the request shapes and for a `400`/`409` surfaced as an error with the server's message.
- [x] 7.2 Render the card for a project with gaps: one row per gap with its action, the uncommitted paths with the commit guidance, and nothing rendered for a project with no gaps. Verify `harness-shell` tests for the unready project, the ready project, and the uncommitted state.
- [x] 7.3 Wire each action to its request and re-scan readiness afterwards, so a finished action drops its row and leaves the others. Verify a test that seeds the flow from the card and asserts the flow row disappears while a remaining skills row stays.
- [x] 7.4 Show the offline explanation: with offline mode active, offer the local install when a source is configured and mark the network action unavailable with its reason. Verify tests for both offline shapes and for the online case offering the CLI action.

## 8. Repository initialisation

- [x] 8.1 Add `POST /api/project/repository` running `git init` on `main`, refusing a directory that is already a repository (including one nested inside another) and refusing to commit. Verify tests for a plain directory, a second call, and a nested directory.
- [x] 8.2 Report whether the repository has a commit in readiness, make it part of ready, and offer an `init_repository` action only when the directory is not a repository. Verify tests for an initialised repository with no commit being reported not ready and for the action being refused once a repository exists.
- [x] 8.3 Offer the initialisation action and the first-commit instruction in the card, re-scanning after initialisation. Verify tests for a non-repository project, a repository with no commit, and the row disappearing once `.git` exists.
- [x] 8.4 Document initialisation in `docs/workflows.md` and update the `AGENTS.md` provisioning decision to say the server initialises a repository but never commits. Verify the documented command and endpoint match the handler test.

## 9. One surface per setup gap

- [x] 9.1 Lift the readiness read into the chat home and pass it to the card, so the page can decide what it already explains. Verify `ChatHome` tests for the card receiving readiness and the card tests driven by a readiness prop.
- [x] 9.2 Suppress the catalog error for a non-repository project and the harness warning the card already lists, leaving genuine failures reported. Verify tests for both suppressions and one test that the harness warning still appears when the card says nothing about it.
- [x] 9.3 Replace the composer with a setup hint while the project is not a repository or has no commit, and strengthen the card's surface so it reads as the primary element. Verify tests for both composer states.
- [x] 9.4 Gate the tree's leaves on a cheap repository read: one Create git repository action instead of two sections, and a refresh of every project read after it succeeds. Verify `ControlPanel` tests for the action, the absent leaves, and the leaves appearing after creation.

## 10. Integration checks

- [x] 10.1 Run `make test` and confirm the Go suite, the frontend typecheck, and the frontend tests pass.
- [x] 10.2 Run the readiness, initialise, seed, and offline install against a scratch directory with the real binary, confirming the project becomes ready after committing and that `--offline` refuses the online path.
- [x] 10.3 Run `openspec validate add-project-onboarding --strict` and `openspec validate --changes --strict` and confirm both are clean.

## Workflow follow-up

- Archive the change after the gates above pass, in the same commit as the implementation and the synced specs.
