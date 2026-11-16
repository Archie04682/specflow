# Proposal

## Why

SpecFlow can execute one node on one harness, but it cannot run a workflow. Every step of the OpenSpec lifecycle still has to be started by hand, there is no record of what a step changed, and nothing prevents an agent from mutating the user's working tree. The harness contract was built so the layer above it could own exactly this; until an orchestrator exists, the product has no workflow in it.

## What Changes

- Load and validate workflow definitions from YAML: node identifiers, kinds, edges, and acyclic ordering.
- Execute a run: start each node when its predecessors have completed, and give every run exactly one terminal outcome.
- Support two node kinds: `agent`, which runs through a harness adapter, and `command`, which runs a shell command in the run workspace and carries the deterministic gates.
- Resolve everything a node needs before it starts: prompt interpolation from a fixed run context, model and reasoning effort with defaults, skill names to absolute directories, the working directory, and harness capability preflight.
- Check a declared postcondition against the repository after every node instead of trusting exit codes.
- Retry a node forward with its failure, retry it cleanly from its checkpoint, and resume a failed run at the node that failed without re-running earlier nodes.
- Pause a run at an approval gate before a node that declares one.
- Isolate every run in its own git branch and worktree, commit a checkpoint after each node, and merge only through an explicit approval-gated node.
- Persist run metadata per project under `.specflow/`, with worktrees excluded from version control.
- Expose runs over HTTP: create, inspect, stream events with resumable replay, answer questions, inject clarifications, cancel, retry, and approve.

Out of scope: the visual graph editor, node-internal parallel workers, loops and condition nodes, SQLite, multi-user support, and automatic merge without approval.

## Capabilities

### New Capabilities

- `workflow-orchestration`: turning a workflow definition into an executed run — node ordering, node preparation, postconditions, retry, approval gates, interactive question and injection routing, and the run event stream.
- `run-workspace`: the git-isolated workspace a run executes in — branch and worktree lifecycle, per-node checkpoints, the `.specflow/` run metadata layout, and the approval-gated merge.

### Modified Capabilities

None. `harness-protocol`, `harness-pi-adapter`, and `single-binary-distribution` keep their requirements unchanged; the orchestrator consumes the adapter contract as it is already written.

## Impact

- `backend/internal/orchestrator` — workflow model, validation, execution engine, postconditions, retry.
- `backend/internal/session` — the `harness.Host` implementation: questions, answers, and injections.
- `backend/internal/openspec` — skill resolution and the validation gate used by `command` nodes.
- `backend/internal/api` — run endpoints and the SSE stream.
- `backend/cmd/api` — wiring the engine and session manager into the server.
- `workflows/standard-feature.yaml` — gains postconditions, model and thinking fields, and a merge node.
- New per-project `.specflow/` directory holding configuration and run metadata.
- One new Go module, `gopkg.in/yaml.v3`, parses workflow files; it is pure Go, so the single binary still cross-compiles. The workspace layer uses the installed `git`, and run metadata is JSON.
