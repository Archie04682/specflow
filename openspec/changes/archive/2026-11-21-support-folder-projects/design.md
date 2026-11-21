# Design

## Context

The workspace layer assumes git at its root. `workspace.Open` resolves a
repository with `git rev-parse --show-toplevel`, so opening a project's flows,
skills, catalog, or run history fails for a folder that is not one — which is
why onboarding grew a "not a repository" branch instead of a normal read.
`CreateWorktree` branches from `HEAD`; `Worktree.Checkpoint` and `Restore` are
`git add -A` + commit and `reset --hard` + `clean -fd`; `finishCompleted` and
`Discard` call `RemoveWorktree`; `Run` records `SourceBranch`, `BaseCommit`,
`Branch`, and the worktree path; `ResolveWorkdir` lets a node run in the project
directory for the merge step. `Config` currently holds only `skills`.

The run lifecycle around that — plan compilation, node readiness, postconditions,
events, resume, retry — is already mode-agnostic. Everything folder mode needs is
confined to the workspace backend, creation, completion, and the surfaces that
report the project's state.

## Goals / Non-Goals

**Goals:**

- A project selects `git` or `folder` once, and every run in it uses that mode.
- A folder run keeps the same three promises as a git run: the folder is
  untouched while the agent works, any step can be undone, and the result
  arrives as one change the operator approves.
- Readiness, the card, the tree, and the run view state the truth for the mode
  without special-casing it at every call site.
- Existing git projects behave exactly as they do today.

**Non-Goals:**

- A merge, diff, branch, or history for folder projects.
- Partial application of a run's changes.
- Changing a project's mode while runs exist.
- Running two folder runs concurrently, or isolating a folder run from the
  operator's own edits beyond the base-manifest check.
- Replacing git for projects that already use it.

## Decisions

### 1. The mode lives in `.specflow/config.yaml`, defaulting to git

`Config` gains `vcs: git | folder`. An absent value reads as `git`, so every
existing project keeps today's behaviour with no migration, and the readiness
report distinguishes the effective mode from whether it may still change (no
runs yet). The alternative — inferring the mode from `.git` — cannot express a
repository that chooses folder mode, which is a supported combination.

### 2. `Open` stops requiring a repository; git operations check the mode

`workspace.Open` will open any existing directory and load its config. A new
`Project.GitStatus` reports repository, branch, commit, and reason, and the
git-only operations (`CreateWorktree`, `CheckIsolatable`, `CurrentBranch`,
`Head`, `CheckMergeable`, `RemoveWorktree`) call `RequireGit` or are only reached
after a mode check. This is the enabler for everything else: the flow catalog,
skill resolution, workflow source, and run history are ordinary file reads and
must not depend on a repository.

The alternative — a second constructor for folder projects — duplicates the
layout and skill-resolution paths and leaves every caller one wrong choice away.

`onboarding.ReadRepository` becomes a thin wrapper over `Project.GitStatus`
instead of treating `Open`'s error as the answer.

### 3. The folder backend lives inside `Worktree`, not behind an interface

The engine passes `*workspace.Worktree` through preparation, execution, retry,
and completion, and reads `.Path` in most of them. Rather than introduce a
`RunWorkspace` interface and touch every call site, `Worktree` gains the
mode, the project root, and the run id, and dispatches `Checkpoint`, `Restore`,
and `Release` on the mode. The git-only fields (`Branch`, `Base`, `Source`)
stay, empty for folder runs, so the record keeps one shape.

An interface would be cleaner if a third backend were coming. It is not, and the
engine still needs the git-specific fields for the run record, so the interface
would expose both shapes anyway.

### 4. A folder run works on a copy, and apply is add / replace / delete

Creation copies the project into `.specflow/worktrees/<id>`, excluding
`.specflow/` and `.git/`, and writes `.specflow/runs/<id>/base.json`: every
relative path and its content hash. Nodes execute in the copy; the project is
untouched. Git metadata is never copied: a repository may use folder mode, and a
run that wrote git objects back on apply could corrupt it.

Completion keeps the copy and records `apply: pending`. Applying walks the copy:
a file missing from the base or carrying a different hash is written into the
project; a base file missing from the copy is deleted. The report names both
lists. Discarding removes the copy and touches nothing in the project.

The alternative — running in place with snapshots — is cheaper to build but
removes the promise that the folder does not move until approval, and a failed
node would leave the operator's folder half-edited.

### 5. The lock is serialization, not a trust boundary

One folder run at a time per project, from creation until it is applied or
discarded; the refusal names the holding run. A failed run holds it too, so the
run view offers discard for a failed folder run as well as for one awaiting
apply. Creation, apply, and discard are serialized, so the one-run check cannot
be raced and a copy cannot be applied twice. This is what makes "apply is a plain copy" true for the app's own clients.

The operator can still edit the folder outside SpecFlow while a run is pending,
so apply re-verifies the project against the base manifest and refuses with the
changed paths when they disagree. Correctness therefore does not depend on the
lock, which is only there to keep two copies from being applied over each other.

A three-way merge was rejected: it is the thing git exists to do, and a
single-user local tool does not need it.

### 6. Snapshots are directory copies under the run's metadata

A snapshot is `.specflow/runs/<id>/snapshots/<node>-<attempt>/`, a copy of the
run's tree, and its identifier is what `Run.Checkpoints` stores. The copy as
created is snapshotted as `snapshots/base`, which is what a clean retry of the
first node restores: a folder run has no base commit. `Restore`
replaces the copy with the snapshot. Snapshot and copy are removed together on
apply or discard, so a run's disk cost ends with the run.

Git's content addressing is why checkpoints are cheap today; a folder project
trades disk for independence. Hardlinks or an archive were considered and left
out: they add failure modes (cross-device links, partial archives) for a saving
that is not the constraint yet.

### 7. A folder flow has no merge, and no node runs in the project directory

`workflows/` gains `standard-feature-folder`, the same graph without the merge
node. Seeding writes the template that matches the mode, under the same
reference. A folder run refuses a compiled plan containing a node whose workdir
is the project directory: the copy is the isolation boundary, and a node that
ignored it would edit the live folder.

A per-node `vcs:` condition was rejected. It would hide behavior in the graph,
need validation and editor support, and the two templates differ by one block.

### 8. Apply is not a commit, and the run records what happened

No staging, no history, no branch. `Run` gains `VCS` and `Apply`
(`pending | applied | discarded`); the git fields are empty for folder runs. The
record stays the source of truth for the view, which offers Apply and Discard
only while the run is applied-pending.

### 9. Readiness and the shell read the mode instead of probing

Readiness reports `vcs`, whether it was recorded, and whether it can still
change, and counts the repository, branch, and commit checks as gaps only in git
mode. The setup card is a stepper over the gaps: numbered steps with the
current one expanded by default and completed ones summarised, and every step
selectable with Next/Back so the operator can look ahead or return. Readiness
drives the marks; the selection is local, which is what keeps a step whose
action is a terminal command (the first commit) from being a dead end, alongside
a re-check for work done outside the card. The finish line is visible before the
card disappears and the ordinary chat home takes over. The
mode choice is deliberately not a step: it is one project decision, so it
collapses to a summary line once recorded, and a repository needs no click
because git is the default. The tree gates its leaves on "git mode without a
repository", so a folder project shows them. `POST /api/project/setup` writes the
mode and, for git, initialises the repository; it never seeds or installs.

This collapses the previous special cases into one setting and keeps the "one
surface per state" rule: a folder project has no git gap to explain twice.

### 10. Committing is explicit, and secrets are refused

Setup ends at a clean tree, and for many operators the first commit is the last
thing in the way. The card therefore offers it, but the server never commits on
its own: `POST /api/project/commit` stages everything and commits once, deriving
"Initial commit" when there is no HEAD and falling back to a `SpecFlow` author
when the machine has no git identity (reported, not hidden).

A blind commit would publish `.env` files, keys, and certificates. The commit
therefore refuses paths that look like those, names them, and offers the other
explicit action, `POST /api/project/ignore`, which appends the path to
`.gitignore`. Two clicks with a stated reason beat one click that leaks a
secret. The secret check is a heuristic and says so; it is a guard, not a
guarantee.

## Risks / Trade-offs

- **Disk.** Each snapshot is a full copy, so a project of size *N* with *K*
  nodes holds up to *N × K* under the run. Snapshots die with the run; a failed
  run holds them until discarded. Documented, with compression deferred.
- **No history.** A folder project cannot diff two checkpoints, revert an
  applied run, or recover a file after applying. Applying is the point of no
  return; discarding is the escape hatch, and it only exists before apply.
- **One at a time.** A folder project cannot run two workflows concurrently,
  and a failed run blocks new runs until discarded. The refusal must be clear
  enough that this never looks like a bug.
- **`Open` no longer fails for a non-repository.** Every caller that relied on
  that error must be audited; the compiler will not catch a semantic change.
  Readiness and the repository read are the known ones.
- **Folder-mode git projects.** A repository can run in folder mode, so its
  runs never become commits. That is the operator's choice, but the run view
  should not imply otherwise.
- **The verify gate.** The folder template keeps `make test`, which a
  documentation project may not have. The flow is editable, and the template is
  a starting point rather than a promise about every project.

## Migration Plan

None. `vcs` absent reads as `git`, so every existing project keeps its
worktree, checkpoints, and merge. The setup action is additive, and the run
record's new fields are empty for existing runs.

## Open Questions

None blocking. Snapshot compression and partial apply are candidates for a
later change if disk or review granularity becomes a real constraint.
