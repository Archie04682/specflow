# Proposal

## Why

Adding a project to the workbench only writes a path into `localStorage`. The
first time a user opens a folder, nothing else happens: there is no flow to run,
the agents' skills are not installed, `.specflow/` does not exist yet, and
nothing tells the operator that a git repository, a harness, or the `openspec`
CLI is missing. The first run therefore fails at one of four walls, and the
message names the wall rather than the way past it.

The default flow also cannot be offered today: `workflows/*.yaml` lives in this
repository's checkout, so an arbitrary project has no template to copy and the
only path is hand-authoring a graph. And specs are commonly required to work in
an air-gapped environment, where fetching a skill or a package from the network
is not an option.

## What Changes

- A **readiness report** for one project: whether the path is a git repository
  and on which branch, whether a flow exists, which skills each flow binds and
  which of those resolve, and whether the `openspec` CLI and the default harness
  are present. It is a read-only scan, safe to repeat.
- **Repository initialisation** for a folder that is not a git repository: one
  action runs `git init` on `main`, and the card then asks for the first commit.
  A `git init` alone cannot back a worktree, so readiness reports whether the
  repository has a commit at all.
- **Default flow seeding** from a template embedded in the executable, written
  to `.specflow/workflows/<reference>.yaml` only when that file does not already
  exist.
- **Skill provisioning at init**, with two sources and one switch:
  - online — `skills.sh` through the `skills` CLI, pinned to a release;
  - offline — a `--skills-source` path that is a directory, a `.zip`, or a
    `.tar.gz`/`.tgz` (a GitHub "Download ZIP" archive is the expected artifact),
    installed with nothing but the Go standard library.
  Setting `--skills-source` means SpecFlow never constructs a network command
  for skills.
- **`.agents/skills` joins the shared skill roots**, project and global, so
  what the skills CLI installs is what SpecFlow resolves.
- **An offline mode** (`--offline` / `SPECFLOW_OFFLINE=1`) under which the
  server constructs no network command for any dependency: no `npx`, no npm, no
  git fetch. Readiness reports the other dependencies instead of fetching them.
- **The commit step init implies.** Installing into `.agents/` leaves the
  project with uncommitted paths, and every run needs a clean tree. Readiness
  reports those paths and says to commit them; the app never commits for the
  user.
- **An onboarding surface** in the workbench that appears for a project that is
  not ready, lists each gap with one action, and re-scans after an action.
- Workflow templates are compiled into the executable with `//go:embed`.

Out of scope: installing the harness, the `openspec` CLI, Node, or a model
endpoint. Those are detected and reported, never fetched.

## Capabilities

### New Capabilities

- `project-onboarding`: turns a registered folder into a runnable project —
  readiness reporting, flow seeding from the embedded template, skill
  provisioning from an online or offline source, and the expectations the app
  must state rather than perform (commit before run, install the CLI yourself).

### Modified Capabilities

- `skill-catalog`: the default shared-skill root order gains `.agents/skills` and
  `~/.agents/skills`, because the skills CLI installs there for the harnesses it
  supports.
- `single-binary-distribution`: the executable carries the workflow templates it
  seeds, and an offline mode is a property of how the executable runs.
- `harness-shell`: the workbench introduces a project that is not ready, with one
  action per gap, instead of leaving the user to discover each wall at run time.

## Impact

- **Server**: a new readiness read and provisioning actions under
  `/api/projects/...`; archive extraction (standard library only); an embedded
  template set; a server-level offline switch alongside the existing `--port`
  precedence.
- **Skill resolution**: `internal/workspace` default roots and the shipped
  `.specflow/config.yaml` comment change. A project that already keeps a skill
  name in `.dsh/skills` keeps winning unless it also exists in
  `.agents/skills`, so the order is chosen deliberately.
- **Workflow templates**: `//go:embed` of `workflows/`, and a seed action that
  writes the project copy the editor already owns.
- **Frontend**: an onboarding card with actions and a re-scan, the uncommitted
  notice, and the offline-mode explanation of what is not available.
- **Docs**: `docs/workflows.md` (skill roots), `docs/architecture.md` (offline
  mode), and `AGENTS.md` (the new root and the offline guarantee).
- **Compatibility**: the new root is additive; the seed only writes a missing
  file; the offline switch is off by default. An existing project behaves as it
  does today until an operator uses an onboarding action.
