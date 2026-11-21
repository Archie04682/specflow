# Tasks

## 1. The mode and opening a folder

- [x] 1.1 Add `vcs` to `workspace.Config` with `git` as the default, expose the effective mode and whether it may change (no runs yet) on `Project`, and refuse a change once a run exists. Verify config round-trip and default tests.
- [x] 1.2 Make `workspace.Open` open any existing directory: add `Project.GitStatus` and `RequireGit`, move `ReadRepository` and the readiness read onto them, and guard the git-only operations (`CreateWorktree`, `CheckIsolatable`, `CurrentBranch`, `Head`, `CheckMergeable`, `RemoveWorktree`). Verify a non-repository project opens, and every existing caller still behaves for a git project.
- [x] 1.3 Audit the callers that treated `Open`'s error as "not a repository" and fix each one (catalog, flows, skills, workflow source, run history, readiness). Verify the workflow catalog and the readiness read succeed in a plain folder.

## 2. The folder workspace

- [x] 2.1 Copy a folder project into `.specflow/worktrees/<id>` excluding `.specflow/`, preserving file modes, and write `.specflow/runs/<id>/base.json` as relative path to content hash. Verify the copy excludes run metadata and that the project's files are unchanged.
- [x] 2.2 Record a snapshot under `.specflow/runs/<id>/snapshots/<node>-<attempt>/` after each node, restore the copy from a snapshot discarding later files, and release the copy and its snapshots on demand. Verify a clean retry returns to the node's starting state.
- [x] 2.3 Dispatch checkpoint, restore, and release inside `Worktree` on the mode, so the engine keeps one workspace shape; leave the git fields empty for a folder run. Verify both modes through the same engine calls.

## 3. Runs in both modes

- [x] 3.1 Create a folder run: skip the clean-tree check, record `VCS`, refuse a plan with a node whose workdir is the project directory (naming the node), and refuse creation while the project holds another folder run (naming it). Verify each refusal.
- [x] 3.2 Keep a completed folder run's copy and record `apply: pending` instead of removing the workspace; keep a failed run's copy for retry. Verify the completion path does not delete the copy and that a git run still removes its worktree.
- [x] 3.3 Implement apply: verify the project against the base manifest, write added and changed files, delete the files the run deleted, report both lists, mark the run applied, and release the copy, its snapshots, and the lock. Verify a changed project is refused with the paths and left untouched.
- [x] 3.4 Implement discard for a folder run: remove the copy and its snapshots, mark the run discarded, leave the project untouched, and release the lock. Verify a discard is refused while the run is still running.

## 4. API, readiness, and the run record

- [x] 4.1 Add `POST /api/project/setup` accepting `{project, vcs}`: write the mode preserving the config's other settings, initialise a repository for git, refuse an unknown mode, and refuse a change once a run exists. Verify each response and that nothing else is provisioned.
- [x] 4.2 Extend readiness with the mode and whether it can change, count the repository, branch, and commit checks as gaps only in git mode, and make the initialise action unavailable for a folder project with its reason. Verify a folder project can be reported ready without a repository.
- [x] 4.3 Add `VCS` and `Apply` to the run record and payloads, add `POST /api/runs/{id}/apply`, and extend discard to a completed folder run. Verify the apply report and that a non-folder or already-applied run is refused.

## 5. The folder flow

- [x] 5.1 Add `workflows/standard-feature-folder.yaml`: the canonical graph without the merge node and without a project-directory node, embedded by the root package. Verify it compiles and validates like the canonical template.
- [x] 5.2 Make the seed action choose the template for the project's mode while keeping the `standard-feature` reference, and refuse to overwrite as before. Verify a folder project seeds the merge-free flow and a git project still seeds the canonical one.

## 6. The surfaces

- [x] 6.1 In the setup card, offer git or folder while the mode can change, drop the repository and commit rows for a folder project, and keep every other gap and action. Verify the choice, the re-scan, and the locked-mode display.
- [x] 6.2 Make the tree's repository check mode-aware: a folder project renders its Chats and Flows leaves and no initialise action. Verify both modes in `ControlPanel` tests.
- [x] 6.3 Keep the composer's setup hint and the catalog-error suppression for a git project without a repository or commit, and show the composer for a folder project. Verify the composer renders in folder mode.
- [x] 6.4 Offer Apply and Discard in the run view for a completed folder run that is not applied, confirm the discard, show the written and deleted paths, and refresh the record afterwards. Verify the controls and their refusals.
- [x] 6.5 Present setup as a numbered stepper with a progress count and collapsed completed steps, and collapse the mode choice to a summary once a mode is recorded. Verify the step projection and the card's stepper, including a folder project with no repository step.
- [x] 6.6 Make the stepper navigable: selectable steps, Next and Back, and a re-check for work done outside the card. Verify selecting a later step, moving between steps, and the re-check.

## 7. Documentation

- [x] 7.1 Update `AGENTS.md` (decision 10 scoped to git, a new decision for the folder workspace and its lock), `docs/architecture.md` (isolation for both modes), `docs/workflows.md` (the folder template), and `docs/api.md` (setup, apply, and the new run fields).

## 9. Committing from the card

- [x] 9.1 Add the commit and ignore actions: stage and commit on request, derive the initial message, fall back to a SpecFlow author when no identity is configured and report it, refuse paths that look like secrets, append refused paths to `.gitignore`, and report which uncommitted paths look like secrets. Verify the API and the guard.
- [x] 9.2 Offer Create initial commit, Commit changes, and the ignore action in the setup card, and state uncommitted work as a count rather than a path dump. Verify the commit, the secret refusal, and the ignore flow.

## 10. Integration checks

- [x] 10.1 Run `make test` and confirm the Go suite, the frontend typecheck, and the frontend tests pass.
- [x] 10.2 With the real binary, set up a scratch folder project, seed the folder flow, and run a command-only flow to completion; confirm the project is untouched before apply, apply writes the files, a second run is refused while one is pending, and the run records applied. Confirm a git project still creates a worktree and merges, and that a scratch repository commits through `/api/project/commit`, refuses a `.env`, then commits after `/api/project/ignore`.
- [x] 10.3 Run `openspec validate support-folder-projects --strict` and `openspec validate --changes --strict` and confirm both are clean.
