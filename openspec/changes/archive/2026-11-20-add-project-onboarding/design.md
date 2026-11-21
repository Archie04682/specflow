# Design

## Context

See `proposal.md` — Why. This section records only the mechanics the approach
has to fit.

**Adding a project writes nothing.** `Workbench.addProject` closes Settings and
activates the path; the registry is `localStorage`. The server learns about a
project only through query parameters. `.specflow/` is created by
`Project.EnsureLayout`, called from `Engine.Create` — that is, on the first run.

**A project has no template to seed from.** `ReadWorkflowCatalog` scans
`.specflow/workflows/` and `workflows/` under the project. The canonical
template lives in *this* checkout's `workflows/`, so an arbitrary project has
none.

**Skill roots are an ordered list.** `workspace.defaultSkillRoots` is
`.specflow/skills`, `.dsh/skills`, `.pi/skills`, `~/.dsh/skills`; a non-empty
`skills:` in `.specflow/config.yaml` replaces it. `openspec.ResolveSkill` matches
a **directory name**; `openspec.ListSkills` treats a directory as a skill when
it holds a `SKILL.md`.

**A dirty tree blocks a run.** `Engine.Create` calls `CheckIsolatable`, which
fails on any `git status --porcelain` path except `.specflow/` (`filterStatus`).
The merge gate applies the same rule.

**The skills CLI installs to per-agent directories.** For `pi`, `codex`,
`cursor`, `gemini-cli`, `opencode`, and many others the project path is
`.agents/skills/`; `claude-code` uses `.claude/skills/`. `dsh` is not in that
table. Its default telemetry is on, and it fetches from a git source.

**`openspec new change` bootstraps a missing root** (verified), so `openspec
init` is not required for the seeded flow; the `openspec` binary is still needed
by the flow's gates.

**Go cannot embed a parent directory.** `//go:embed` patterns resolve against
the package directory, so a package under `backend/` cannot reach `workflows/`.

## Goals / Non-Goals

**Goals:**

- One read that answers "what is missing for this project to start a run", so
  the four walls become one card.
- Provisioning that works with a network and without one, from the same
  mechanism, with the offline case a guarantee rather than a hope.
- Ship the default flow so a project can have one without a checkout.
- Never change the project except in response to an explicit action.

**Non-Goals:**

- Installing the harness, `openspec`, Node, or a model endpoint.
- Authenticating a private skills source, or mirroring a registry.
- A general skill/package manager. The offline path installs a directory of
  skills; it does not resolve dependencies.
- Committing, staging, or ignoring the operator's files.

## Decisions

### 1. Readiness is derived, never a "first run" flag

The registry is `localStorage`, so "first time" is per browser and resets on a
new machine, a new browser profile, or a cleared store. A durable fact is
missing better signal: the card shows while a gap exists and disappears when it
does not. Cost: the card can appear on a project the operator has used for
weeks — correct, because it is not runnable as it stands.

The read composes existing helpers rather than new state: `workspace.Open` (a
git repository), `Project.CheckIsolatable`, `ReadWorkflowCatalog`,
`Project.SkillRoots` + `openspec.ResolveSkill`, and a PATH lookup for
`openspec`. It writes nothing.

### 2. `.agents/skills` sits in the shared-project family, before harness roots

```
.specflow/skills      project's own, highest
.agents/skills        what skills.sh installs
.dsh/skills           harness-specific
.pi/skills            harness-specific
~/.dsh/skills         global
~/.agents/skills      global, what a global skills.sh install writes
```

The rule reads *shared project → harness project → global*, which is why
`.agents/skills` precedes `.dsh/skills`. A project that already keeps a skill
name in `.specflow/skills` keeps winning, so the change is additive for every
project that does not use `.agents/skills`.

The global root is `~/.agents/skills`, not `~/.dsh/skills`' sibling: that is
where the CLI writes `pi`'s global scope.

### 3. Two providers, one choke point

```
                       +-------------------------------+
  install request ----> |  provisioner                  |
                       |  canReachNetwork() bool       |--false--> refuse
                       +---------------+---------------+
                                       |
                local source configured|        otherwise, online allowed
                                       v                       v
                            extract + copy            run the skills CLI
                              (stdlib only)          (pinned, telemetry off)
```

`--skills-source` (or an equivalent project-independent setting) makes the local
provider *the* provider: the online branch is not merely deprioritized, it is
unreachable. `--offline` does the same for every dependency. Both decisions live
in one predicate so a test can prove the guarantee, and so a future dependency
cannot bypass it by accident.

### 4. Archives are indexed, then selectively extracted

Discovery and extraction are two passes over the archive index:

1. Find every path ending in `SKILL.md`; its parent directory is a candidate,
   named by that directory's base name.
2. Reject the whole archive when it declares more than 1000 entries, expands
   beyond 25 MiB, contains an absolute path or a `..` segment, or contains a
   symlink or hardlink entry.
3. Extract only the candidate directories, into
   `.specflow/tmp/skills-<random>/`, then copy each into `.agents/skills/<name>`.

The two-pass shape is what lets a GitHub repository archive work without
assuming a `skills/` subdirectory, while refusing to write the repository's
sources into the project. `archive/zip`, `archive/tar`, and `compress/gzip` are
standard library, so the offline path adds no dependency — which is the entire
point of the offline path.

The 1000-entry / 25 MiB bounds mirror the skills CLI's own defaults, so the two
providers refuse the same archives.

### 5. Required skills come from the flow, not from a constant

Readiness asks each catalogued workflow which names its nodes bind
(`Node.EffectiveSkills`, so the older singular `skill:` still counts) and checks
each against `ResolveSkill`. A hardcoded `openspec-*` list would be a second
source of truth that drifts from the templates; deriving it also makes the card
correct for any flow the operator adds.

### 6. Templates are embedded by a root package

`//go:embed` cannot reach a parent directory, and moving `workflows/` would
break the docs, the tests that read it, and the resolver's documented layout. So
the embed lives where the templates already are, relative to it:

```go
// templates.go, package specflow, module root
//go:embed workflows/*.yaml
var workflowTemplates embed.FS
```

A repo-root package that carries only the embedded templates is unusual but
zero-drift: there is one copy of every template, and `make build` needs no copy
step. The alternative — generating a copy under `backend/internal/...` — needs a
drift test to stay honest, and a checkout that forgot to generate fails to
compile.

### 7. Seeding writes one file and refuses to overwrite

`POST /api/project/flow` writes `.specflow/workflows/<reference>.yaml` from the
embedded template, creating the directory. An existing file is a conflict, not
an overwrite: a project copy may carry the operator's edits, and the editor
already owns that path. The response names the path and the reference.

### 8. The uncommitted step is reported, never performed

Installing into `.agents/` and seeding into `.specflow/workflows/` both leave
untracked paths, and `CheckIsolatable` refuses a run on an untracked path. The
app therefore cannot end provisioning at "installed". It reports the paths and
the remaining instruction. Committing is the operator's decision because the
message, the branch, and whether the skills belong in the repository at all are
theirs — some teams will prefer a global install or a `.gitignore` entry.

An install can be invisible to this check when the project ignores `.agents/`;
readiness then reports no gap, which is the operator's stated preference.

### 9. Initialisation is a repository, not a commit

`git init -b main` (with a fallback that names the branch after the fact for git
without `-b`) turns a folder into a repository. It creates no commit: a run's
worktree branches from `HEAD`, so the first commit is the next required step,
but making it would stage whatever the tree holds — a missing `.gitignore` can
drag in build output — and author it as the operator. The card shows the command
and stops there.

Readiness therefore carries `hasCommit`. Without it an initialised-but-empty
repository would report ready and then fail in `CreateWorktree`, which is the
kind of false positive the whole read exists to prevent. A directory already
inside another repository is refused rather than nested: `git rev-parse
--show-toplevel` answers with the outer root, and the refusal names it.

### 10. Provisioning is explicit, per project

Nothing runs when a path joins the registry. The card offers actions; each
action is one request that does one thing and reports what it did. That keeps
"add a project" cheap, makes every write attributable to a click, and means a
mistyped path cannot trigger an install inside the wrong repository.

### 11. A setup gap has one surface

A state that blocks every run is described once. When the project is not a
repository, the flow catalog genuinely cannot be read — so the card states it
and the derived "could not load flows" banner, with a Retry that would fail
identically, is suppressed. The same applies to a harness warning the card
already lists. While a structural gap exists the composer is replaced by a
hint, because a disabled flow selector and a New flow button that cannot save
are dead affordances below the only useful action.

Everything else keeps its own message: a start that fails, a readiness read that
fails, and a repository whose catalog cannot be read still report themselves.
The rule is "one surface per state", not "silence errors".

The same rule reaches the navigation: an expanded project that is not a
repository shows one initialise action instead of two sections that could only
report an error, read through a cheap repository check rather than the full
readiness report. Creating the repository refreshes every project read on the
page, so the tree and the workspace agree without a shared store.

## Risks / Trade-offs

- **[An archive is third-party content]** → Only skill directories are written,
  links are never created, traversal is rejected, and the result is bounded. The
  operator chose the file, but the repository behind it is not trusted.
- **[Installed skills become agent instructions]** → The card states the source
  and the file count before installing, and the offline path names the local
  file. There is no silent install.
- **[The first run is refused after a successful onboarding]** → The uncommitted
  step is part of the card, not a surprise at run creation.
- **[Version skew between skills and the `openspec` CLI]** → The online install
  pins a release and readiness reports the CLI version it found, so a mismatch
  is visible rather than mysterious.
- **[Offline mode rots]** → Every network-capable action goes through one
  predicate with a test that asserts no command is constructed, rather than
  scattered checks.
- **[A project's `.gitignore` hides `.agents/` from teammates]** → Out of the
  app's hands; documented, and readiness cannot see what git ignores.
- **[The root package is a small architectural addition]** → It carries only the
  embed and is documented in the repository layout.

## Migration Plan

1. Add the root package and embed; no behavior changes yet.
2. Extend the skill roots and update the shipped `.specflow/config.yaml`
   comment. Existing projects are unaffected unless they already use
   `.agents/skills`.
3. Land readiness as a read-only endpoint and the card that consumes it, with
   no actions. Every project with a flow and its skills shows nothing.
4. Add the offline provider, then the online one, then the seed action.
5. `--offline` and `--skills-source` ship inert: unset, behavior is today's.
6. Docs last, so no page describes a flag the build does not have.

Rollback is a revert: no on-disk format changes, no run record changes, and the
seeded file is an ordinary workflow the editor can open.

## Open Questions

- Whether the archive source is chosen with the existing folder dialog plus a
  typed path, or a new file dialog. The path field is enough to ship; a file
  dialog is polish.
- Whether the card is dismissible per session. Leaning no: a dismissible
  readiness card becomes a dismissed one.
- The exact pinned skills release. It should track the `openspec` CLI version
  the templates were written against, which is a value to read at release time.
