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

The canonical template is `workflows/standard-feature.yaml`.

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
representable but the engine runs one node at a time.

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

1. `.specflow/skills`
2. `.dsh/skills`
3. `.pi/skills`
4. `~/.dsh/skills`

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
