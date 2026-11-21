# Proposal

## Why

A run currently requires a git repository, and the onboarding surfaces that as a
hard gap: a folder that is not a repository can only be told to become one. That
is right for a codebase and wrong for the people SpecFlow is now reaching. The
work a run produces is files — a spec, a note, a document set — and an operator
who does not use version control cannot satisfy a requirement invented by the
workspace implementation.

Git is not what the product promises. The promise is that an agent works
somewhere you can undo, that nothing in your folder moves until you approve it,
and that the result arrives as one reviewable change. Git is one way to keep
that promise. A copy of the folder, a snapshot after each step, and an
approval-gated apply keeps the same three promises without a repository.

So the operator chooses, per project: **git** (today's behaviour) or **folder**
(the same lifecycle on a copy, no repository).

## What Changes

- `.specflow/config.yaml` gains `vcs: git | folder`, defaulting to `git` when
  absent. The effective mode and whether it may still change are reported by
  readiness. The mode is changeable only while the project has no runs, is
  recorded in every run created after it, and applies to future runs only.
- A new setup action records the choice: `POST /api/project/setup` with
  `{project, vcs}`. Choosing git initialises a repository when the folder does
  not have one; choosing folder writes the mode and creates nothing else. It
  never seeds and never installs.
- Folder runs work on a copy: creation copies the project into
  `.specflow/worktrees/<id>` (excluding `.specflow/`) and records a manifest of
  the copied files as the run's base. The project's own files are untouched.
- Every node snapshots the copy. A clean retry restores the copy from the
  snapshot, exactly as a git checkpoint restores a worktree.
- One folder run at a time per project, from creation until the run is applied
  or discarded. The refusal names the run that holds the project.
- A completed folder run keeps its copy and records `apply: pending`. The
  operator applies (added and changed files written back, deleted files
  removed, a report of both) or discards (copy removed, project untouched).
  Apply refuses when a project file no longer matches the base manifest, naming
  the paths. Nothing is staged, committed, or added to history.
- A folder project seeds a flow without a merge node: `workflows/` gains a
  `standard-feature-folder` template and the seed action picks the template for
  the project's mode. A folder run refuses a flow that contains a node running
  in the project directory.
- `workspace.Open` stops requiring a repository, so catalogs, flows, skills, and
  reads work in a folder; git-only operations check the mode explicitly.
- Readiness, the setup card, the project tree, and the composer become
  mode-aware: a folder project has no repository or commit gap, the tree shows
  its leaves, and the card offers the mode choice while it can change.
- The run view offers Apply and Discard for a folder run awaiting apply, and the
  run record gains the mode and the apply outcome.

Not in this change: mixing modes inside one run; applying part of a run;
changing the mode while runs exist; a merge step for folder flows; history,
branches, or diffs beyond the run's own snapshots.

## Capabilities

### New Capabilities

None. Folder projects are a second workspace backend for the existing run
lifecycle, not a new capability: the isolation, checkpoint, and retry
requirements already live in `run-workspace`.

### Modified Capabilities

- `run-workspace`: seven git requirements become scoped to git-mode projects,
  and folder-mode isolation, snapshots, the per-project lock, and the
  applied-or-discarded lifecycle are added.
- `project-onboarding`: readiness reports the workspace mode and counts the
  repository and commit gaps only for git projects; a setup action records the
  mode; repository initialisation applies only to git projects; seeding picks
  the template for the mode.
- `harness-shell`: the setup card offers the mode choice and the tree renders
  leaves for a folder project instead of a Create git repository row.
- `run-dashboard`: the run controls offer Apply and Discard for a folder run
  whose changes are not yet in the project.

## Impact

- `backend/internal/workspace`: `Open` stops requiring git; a mode-aware
  `Worktree` (copy, snapshot, restore, release) and the base manifest.
- `backend/internal/orchestrator`: creation for both modes, the folder lock,
  completion that keeps the copy, and apply/discard; the run record and API gain
  the mode and the apply outcome.
- `backend/internal/onboarding`: the setup action, mode-aware readiness, and
  template selection when seeding.
- `workflows/standard-feature-folder.yaml`, embedded by the root package.
- `backend/internal/api`: `POST /api/project/setup`, `POST /api/runs/{id}/apply`,
  readiness and run payloads.
- `frontend`: mode choice in the setup card, mode-aware tree and composer,
  Apply and Discard in the run view.
- `docs/`, `AGENTS.md` decisions 10 and 14, and `docs/workflows.md` for the
  folder template.
- Tests: workspace copy/snapshot, engine lifecycle for both modes, onboarding
  setup, and the UI surfaces.
