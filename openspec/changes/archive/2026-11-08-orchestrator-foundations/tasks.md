# Tasks

## 1. Workspace foundations

- [x] 1.1 Add `internal/workspace` with project resolution and the `.specflow` layout — `config.yaml`, `runs/<id>/`, `worktrees/<id>/`, and a `.specflow/.gitignore` containing `worktrees/`; verify a test creates the tree on first open and asserts a fresh worktree is absent from `git status --porcelain`.
- [x] 1.2 Implement worktree creation on `specflow/run-<id>` from a base commit, recording source branch, base commit, branch, and worktree path; verify a test in a temporary git repository asserts the project stays on its original branch and the worktree exists at the recorded path.
- [x] 1.3 Refuse to start on a project that is not a git repository or has a dirty working tree, naming the condition; verify tests for both refusals assert nothing was created and no stash was made.
- [x] 1.4 Implement checkpoint commits (commit only when the index is dirty, fixed fallback identity) and checkpoint restore (`reset --hard` plus untracked removal, never `-x`); verify a test asserts an unchanged node reuses the previous identifier and a restore removes an added file while keeping an ignored one.
- [x] 1.5 Implement merge preconditions for the source branch and a clean project tree; verify tests assert a merge is refused when the project is on another branch and when it has uncommitted changes, without modifying any branch.
- [x] 1.6 Implement workspace lifetime — worktree removed on completion and on discard, retained after failure, branch always retained; verify a test covers each of the three transitions.

## 2. Run model and store

- [x] 2.1 Define `Run`, `NodeRun`, run and node statuses, checkpoints, the resolved change, and the run context record; verify a table-driven test round-trips every status and the context through JSON.
- [x] 2.2 Persist a run to `.specflow/runs/<id>/run.json` on every transition using write-temp-and-rename; verify a test loads a run after a simulated restart and another asserts a truncated temporary file leaves the previous record readable.

## 3. Workflow definition

- [x] 3.1 Model and load the workflow YAML — nodes, kinds, edges, workflow defaults, postconditions, working directory, approval, retry, model, and thinking; verify `workflows/standard-feature.yaml` loads without error.
- [x] 3.2 Validate a workflow before a run starts: duplicate identifiers, edges to missing nodes, cycles, unknown node kinds, an `agent` node without a harness, and a `command` node without a command; verify one test per defect and one asserting two defects are reported together.

## 4. Node preparation

- [x] 4.1 Implement the closed-variable prompt renderer for `{{objective}}`, `{{change}}`, `{{run_id}}`, and `{{workdir}}`; verify a test substitutes each and asserts an unsupported placeholder fails the node before any execution starts.
- [x] 4.2 Implement model and reasoning-effort resolution from node to workflow defaults to omission, treating empty and the literal `default` as "omit"; verify tests for a node override, a workflow default, and both omit forms.
- [x] 4.3 Implement skill-name resolution to an absolute directory across the configured roots; verify tests resolve a known skill and fail an unknown one with the searched locations in the message.
- [x] 4.4 Select the node working directory (`run` by default, `project` when declared) and run harness preflight through the registry's existing `Validate`; verify tests for both working directories and for a preflight refusal that does not start the node.

## 5. Postconditions

- [x] 5.1 Implement the postcondition evaluator for `none`, `file_exists`, and `exit_zero`; verify a test per type, including a present and a missing file.
- [x] 5.2 Implement `new_change` against a baseline recorded in the run record before the asserting node starts; verify tests for one new change, no new change, and several new changes reported as ambiguous.
- [x] 5.3 Implement `tasks_complete` by reading the change's `tasks.md` for line-anchored unchecked items; verify tests for all-checked, one-unchecked, and mixed conventions, with the failure naming the offending task.

## 6. Execution engine

- [x] 6.1 Implement DAG ordering, one node at a time, terminal run status, and `skipped` reporting for nodes that never started; verify with the mock adapter that a three-node graph completes and that a failing second node leaves the third `skipped`.
- [x] 6.2 Implement `agent` nodes by looking up the harness and calling `Run` with a prepared request and host; verify with the mock adapter that the node status comes from the returned result and not from the last streamed event.
- [x] 6.3 Implement `command` nodes through a shell with timeout, environment, and bounded output, treating non-zero exit as failure; verify tests for exit zero, non-zero, and a timeout that terminates the process.
- [x] 6.4 Wire a checkpoint and the postcondition check around every node, recording the checkpoint against the node and failing the node on an unmet postcondition; verify a test asserts the checkpoint identifier is recorded and that a postcondition failure stops the run.
- [x] 6.5 Implement approval gates: enter a waiting status before a gated node, name the node and the action, continue on approval, and end the run cancelled on rejection; verify a test per outcome and assert the node did not execute on rejection.

## 7. Session manager

- [x] 7.1 Implement `session.Session` as the per-execution `harness.Host` — `Emit`, `Ask`, and `Injections` — with the run reported as waiting while a question is outstanding; verify with the mock adapter that an asked question is published and an answer resumes the node.
- [x] 7.2 Route answers by question identifier and discard an answer that arrives after resolution; verify a test asserts a late answer leaves the execution unaffected.
- [x] 7.3 Accept an injection only while a node is running and only when the node and harness allow it; verify tests for delivery, for a run that is not running, and for a node that disallows injection.
- [x] 7.4 Implement run cancellation that cancels the running node within a bounded period and leaves no execution running; verify a test asserts the run ends cancelled and the mock adapter observed a cancellation.

## 8. Retry and resume

- [x] 8.1 Implement fix-forward retry bounded by the node's limit, including a bounded failure excerpt in the prompt, for retryable adapter failures automatically and for postcondition failures only on request; verify tests assert the attempt count and the prompt content, and that a non-retryable failure is not retried.
- [x] 8.2 Implement clean retry by restoring the node's starting checkpoint before re-execution; verify a test asserts the worktree matches the checkpoint before the retry runs.
- [x] 8.3 Implement resume at the failed node, restoring status, node results, checkpoints, and the resolved change from the persisted record; verify a test asserts completed nodes are not re-executed and that the same change reaches later nodes.

## 9. Events and HTTP API

- [x] 9.1 Implement the run-scoped event sequence and append events to `.specflow/runs/<id>/events.jsonl`; verify a test asserts strictly increasing sequence numbers and that replay from a sequence number returns only later events.
- [x] 9.2 Add `GET /api/runs/{id}/events` as an SSE stream honouring `Last-Event-ID`; verify an `httptest` reconnect receives later events only and no duplicates.
- [x] 9.3 Add the remaining run endpoints — create, list, get, answer, inject, cancel, approve, retry, resume, discard — with JSON errors naming the run and node; verify one `httptest` case per route and document the surface in `README.md` with a command sequence that works as written.

## 10. Wiring and integration

- [x] 10.1 Update `workflows/standard-feature.yaml` with postconditions, `model` and `thinking` fields, a `command`-kind `verify` and `tests` node, and an approval-gated merge node declaring `workdir: project`; verify a test loads the file and asserts the merge node's kind, working directory, and approval.
- [x] 10.2 Wire the workspace, store, engine, session manager, and skill resolver into `backend/cmd/api`, and update `AGENTS.md` and `README.md` so the orchestrator is no longer described as a stub and the `.specflow/` layout is documented; verify `make build` succeeds and `make test` passes.
- [x] 10.3 Add an end-to-end test: a temporary git project, the mock adapter, and the canonical workflow executed through the HTTP API — verify the run completes, checkpoints exist per node, `run.json` and `events.jsonl` are readable afterwards, and the run branch exists while the project remains on its original branch.

## Workflow follow-up

- Run `openspec validate --strict orchestrator-foundations` and `make test` before archiving.
- Archive this change, syncing both delta specs into `openspec/specs/`, and land implementation, synced specs, and archive in one commit as `AGENTS.md` requires.
