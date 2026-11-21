# Workflows

A workflow is a YAML file that describes a directed graph of steps. A **block is
configuration, not a type**: every node is either an `agent` node, run through a
harness, or a `command` node, run by the orchestrator as a deterministic gate.
Everything that makes a node "explore" or "verify" is configuration — the engine
only knows the two kinds.

## Finding a workflow

A run's `workflow` field is resolved as:

1. a path,
2. `<project>/.specflow/workflows/<name>.yaml`,
3. `<project>/workflows/<name>.yaml`.

## Seeding the default flow

A project with no workflow can be given the default one without a SpecFlow
checkout: the templates are compiled into the executable.
`GET /api/templates` lists the references this build carries, and
`POST /api/project/flow` writes one to `.specflow/workflows/<reference>.yaml`:

```json
{ "project": "/path/to/repo", "reference": "standard-feature" }
```

The reference defaults to `standard-feature`. Seeding never overwrites: an
existing file — including one the operator edited — is reported as `409` with
nothing changed. The written file is an ordinary project workflow, so the editor
opens and saves it like any other.

The template written depends on the project's workspace mode. A **git** project
gets `workflows/standard-feature.yaml`; a **folder** project gets
`workflows/standard-feature-folder.yaml`, the same graph without the merge node,
because applying the run's copy is the final approval. A project that is not a
repository can run in folder mode; a git project also needs at least one
commit, because a run branches a worktree from `HEAD`. The onboarding card can
initialise a repository, but it never commits: the first commit is yours to
make.

The flow binds skills by name, so a project is only runnable once they are
installed. `GET /api/project/readiness?project=<path>` reports which of a flow's
skills resolve and which do not, alongside the repository, tool, and harness
state.

## Shape

```yaml
name: Standard feature
description: Explore, propose, implement, review, test, verify, archive, merge.
version: 1

defaults:
  harness: pi
  model: default
  interactive: false
  allow_inject: true

nodes:
  - id: explore
    title: Explore
    kind: agent
    skills: [openspec-explore]
    prompt: |
      The run objective is: {{objective}}
      Explore the repository and that objective.
    interactive: true

  - id: apply
    title: Apply
    kind: agent
    skills: [openspec-apply-change]
    prompt: |
      The run objective is: {{objective}}
      Implement the change "{{change}}".
    postcondition:
      type: tasks_complete
      change: "{{change}}"

  - id: verify
    title: Verify
    kind: command
    command: openspec validate "{{change}}" --strict && make test
    timeout: 10m
    postcondition:
      type: exit_zero

  - id: merge
    title: Merge
    kind: command
    workdir: project
    command: git merge --no-ff specflow/run-{{run_id}} -m "Merge {{change}}"
    require_approval: true
    postcondition:
      type: exit_zero

edges:
  - {from: explore, to: apply}
  - {from: apply, to: verify}
  - {from: verify, to: merge}
```

The canonical template is `workflows/standard-feature.yaml`; folder projects are
seeded from `workflows/standard-feature-folder.yaml`.

## Nodes

| Field | Applies to | Meaning |
| :--- | :--- | :--- |
| `id` | both | Unique node identifier, referenced by edges |
| `title` | both | Display name in the graph and event stream |
| `kind` | both | `agent` or `command` |
| `harness` | `agent` | Adapter name; default `pi` |
| `model` | `agent` | Model id, or `default` to let the harness choose |
| `thinking` | `agent` | Reasoning effort, when the model supports one |
| `skills` | `agent` | Skill names resolved to absolute paths before the run |
| `skill` | `agent` | Older singular form, still readable; do not set both |
| `tools` | `agent` | Tool allowlist; empty means the harness default |
| `prompt` | `agent` | Prompt text; supports the closed variable set below |
| `command` | `command` | Shell command to run |
| `workdir` | `command` | `run` (worktree, default) or `project` (the checkout) |
| `postcondition` | both | Repository check that defines success |
| `routes` | `command` | Exit code to route label; a declared code routes instead of failing |
| `timeout` | both | Execution deadline; defaults 10m (command), 30m (agent) |
| `interactive` | `agent` | Whether the node may ask the operator questions |
| `allow_inject` | `agent` | Whether the operator may inject mid-run guidance |
| `require_approval` | both | Gate every attempt behind an approval prompt |
| `ask_timeout` | `agent` | Optional deadline for a pending question |
| `approval_timeout` | both | Optional deadline for an approval gate |

**Model sentinel.** `model: default` means "let the harness choose". It is never
forwarded as a model identifier.

**Prompt variables.** A prompt is rendered from a closed set: `{{objective}}`,
`{{change}}`, `{{run_id}}`, and `{{workdir}}`. There is no template language, and
an unknown placeholder fails compilation before the node starts. The repository
is the run's state: edges carry ordering, not data, and a node's result is what
it wrote to disk.

**Edges.** `from`/`to` pairs define ordering. Independent branches are
representable but the engine runs one node at a time. An edge may declare
`when`, naming a route label its source node declares.

## Conditional routing

A workflow can choose a path instead of running every node. The decision is a
repository fact measured by a `command` node, never a model's report of what it
did:

```yaml
- id: review_gate
  title: Review gate
  kind: command
  # Exit 0 means the file is absent or empty; exit 1 means it has findings.
  command: test ! -s .specflow/review.md
  routes:
    0: clean
    1: issues

edges:
  - {from: review, to: review_gate}
  - {from: review_gate, to: fix, when: issues}
  - {from: review_gate, to: tests, when: clean}
  - {from: fix, to: tests}
```

**How the branch is chosen.** `routes` maps an exit code to a label. A declared
code completes the node under that label and selects the one outgoing edge with
the matching `when`. Any code the node does not declare still fails it: a
timeout, a cancellation, or a shell error can never masquerade as a branch.
Every declared label must be used by exactly one outgoing edge, and every
outgoing edge of a routing node must name a declared label, so routing is a
total function with no "no route" state.

**How the run continues.** A node runs once every predecessor has resolved and
at least one incoming edge was taken; a node that no taken edge reaches is
reported `skipped` and takes none of its own edges. A join runs as soon as
either branch arrives, so `tests` above runs after `fix` or after the clean
edge, whichever the gate chose. Among nodes that are ready, the engine runs the
one earliest in the compiled order, one node at a time. A workflow that declares
no `routes` takes every edge and runs exactly as it did before routing existed.

**Resuming.** The label a node completed under is recorded on the run. A resumed
run re-derives the taken path from those labels alone: a node skipped because
its branch was not taken stays skipped, and a node skipped because the run
failed is re-run once a taken edge reaches it.

### Where a gate keeps its evidence

The engine knows `routes` and `when`; it knows nothing about reviews. The
canonical template happens to use a findings file, and that convention is worth
copying:

```yaml
- id: review
  kind: agent
  prompt: |
    If you find something that must be fixed, write every finding to
    .specflow/review.md, one per heading. If the changes are clean, do not
    create that file.

- id: fix
  kind: agent
  prompt: |
    Read .specflow/review.md and fix every issue it lists. Delete that file
    when you believe all of them are addressed.
```

Paths in a prompt and in a gate command resolve against the node's working
directory, which is the run's worktree by default. The findings file therefore
lives at `.specflow/worktrees/<run-id>/.specflow/review.md` — a **different**
`.specflow/` from the project's run journals — and it is ignored by git at any
depth because `.gitignore` names `.specflow/` without a leading slash.

Because the file is the channel, no prompt variable carries the verdict and the
edge carries no data. `fix` reads the findings from the working tree because
that is where the work is.

## Postconditions

Success is a postcondition checked against the repository, not an exit code.

| Type | Passes when |
| :--- | :--- |
| `file_exists` | The named path exists in the worktree |
| `new_change` | A change exists that was not present before the run |
| `tasks_complete` | Every task in the named change is checked |
| `exit_zero` | The command exited zero |

A postcondition failure is a distinct outcome from a harness failure, and it is
not retried automatically.

## Skills

A skill name resolves from, in order:

1. `.specflow/skills` — the project's own skills
2. `.agents/skills` — what the [skills CLI](https://skills.sh) installs for most harnesses
3. `.dsh/skills`
4. `.pi/skills`
5. `~/.dsh/skills`
6. `~/.agents/skills` — what a global `skills` install writes

The rule is shared project root, then harness-specific project root, then
global. A skill in `.specflow/skills` therefore shadows an installed copy of the
same name, and an installed copy shadows a harness-specific one.

A `skills` list in `.specflow/config.yaml` replaces that order. The harness
receives the resolved absolute path of every selected skill, so a skill lives in
one shared root and is never copied into a harness-specific folder. A missing
skill fails run creation, not a node.

## Timeouts

- **Execution timeout** bounds active work. For an agent node it covers harness
  startup and prompt acceptance. When it expires the harness is stopped and the
  node fails as timed out, which is retryable.
- **Waiting for an answer does not spend the execution budget.** The timeout
  pauses while a question is pending and resumes when it resolves.
- **`ask_timeout` and `approval_timeout`** are optional. With neither set, a
  question or approval gate waits until the operator resolves it or the run is
  cancelled. A configured approval deadline that passes fails the node.
- The resolved values are compiled into the run, so editing the file later does
  not change an existing run.

## Approval gates

`require_approval: true` gates every attempt, including an automatic retry. A
rejected gate does not start that attempt and ends the run as cancelled. A gate
that expires fails the node and the run.

## Compilation and pinning

Creating a run compiles the workflow first: defaults and the `default` model
sentinel resolve, skill names become absolute paths, and the graph is validated.
A missing skill, or any value that cannot be resolved, fails the request before
a worktree exists.

The definition that compiled is copied to `.specflow/runs/<id>/workflow.yaml`,
and resume compiles that copy. Editing or deleting the project's workflow file
therefore does not change an existing run. Skill roots and skill directories are
checked again on resume.

## Retry and resume

- **Forward retry** adds the failure to the prompt and runs the node again in
  the same worktree.
- **Clean retry** returns the worktree to the node's checkpoint before running
  it again.
- **Resume** continues at the first unfinished node; completed nodes are never
  re-executed.
- A run left `running`, `waiting_approval`, or `waiting_answer` by a previous
  process is repaired the first time this process observes it: the interrupted
  node fails with a retryable `harness_exit` reason naming the restart.

## What a run leaves behind

```text
<project>/.specflow/
|-- .gitignore          # "worktrees/" - metadata may be committed, worktrees never
|-- config.yaml         # optional skill roots
|-- skills/<name>/      # optional shared skills, one copy for every harness
|-- runs/<id>/          # run.json + events.jsonl + workflow.yaml
`-- worktrees/<id>/     # the run's checkout; removed when the run completes
```

While a run waits for an answer or an approval, `run.json` carries a `pending`
object with the interaction's kind, id, node, attempt, question or action, and
deadline. It is cleared when the wait resolves. The record also keeps the last
100 accepted clarifications under `injections`.

Checkpoints are commits on `specflow/run-<id>`, so `git -C <project> log --oneline
specflow/run-<id>` shows them and `git diff <pre>..<post>` is a node's change.

## Saving from the editor

The flow editor opens a workflow with `GET /api/workflows/{reference}`, which
returns the exact source, a content hash, and a structured draft. It saves with
`PUT /api/workflows/{reference}`, which validates the draft and writes canonical
YAML to the project copy at `.specflow/workflows/<reference>.yaml`. A workflow
that exists only as a template under `workflows/` is copied rather than modified.

A save must carry the hash it opened with, so a file that changed in the meantime
is a `409` with the current hash. The generated YAML is canonical: comments and
formatting in the source file are not preserved.
