# Tasks

Rungs 1–7 require `PI_E2E=1` plus `PI_E2E_MODEL` and working credentials; rung 8
additionally requires `PI_E2E_FULL=1`. Everything in groups 1–4 runs in
`make test` without Pi, a model, or network.

## 1. Truthful failure classification

- [x] 1.1 Add `provider` to the contract's failure classes and a table test covering every class and its retryability; verify `go test ./backend/internal/harness/ -run FailureClass`.
- [x] 1.2 Record in-band assistant errors in the Pi translator (error stop reason, `errorMessage`, and the `message_update` error variant) and turn the settled result into a failure; verify translate tests for error-then-settle, error-then-retry, and clean settle.
- [x] 1.3 Classify Pi prompt rejections and startup diagnostics: credentials and 401/403 as `auth`, unknown model and permanent request errors as `config`, everything else as `provider` with explicit retryability; verify adapter tests using staged fake-pi stderr and a new prompt-rejection knob.
- [x] 1.4 Add orchestrator coverage that a `provider` failure is retried automatically while `auth` and `config` are not; verify `go test ./backend/internal/orchestrator/ -run Failure`.
- [x] 1.5 Document the failure classes and retry rules where the harness contract is described (AGENTS.md harness section); verify the text names every class.

## 2. Skill-load verification and handshake hygiene

- [x] 2.1 Extend fake-pi with a `get_commands` responder that lists the skill named by `--skill`, plus a knob to omit it; verify the adapter's existing skill tests still pass.
- [x] 2.2 After `get_state`, require `get_commands` to contain `skill:<name>` when a node binds a skill, fail `config` before prompting when it does not, and warn-and-continue when the command is unsupported; verify adapter tests for loaded, missing, and unsupported cases.
- [x] 2.3 Ignore the bridge-ready notification in the translator so neither the run loop nor the cancellation drain emits it as telemetry; verify a translate test and an adapter run test assert no event carries the marker.
- [x] 2.4 Document the skill preflight beside the existing Pi harness notes in AGENTS.md; verify the note matches the test names.

## 3. Agent timeout

- [x] 3.1 Wrap the agent execution in the node's compiled timeout and distinguish deadline expiry from cancellation in the adapter's shutdown result; verify adapter tests for `timed_out`/retryable and `cancelled`/non-retryable.
- [x] 3.2 Add orchestrator coverage that an agent node exceeding a short timeout is reported failed as timed out and retryable, while run cancellation still ends cancelled; verify `go test ./backend/internal/orchestrator/ -run Timeout`.
- [x] 3.3 Document agent-node timeout semantics and the 30-minute default in README; verify the documented YAML compiles under the existing compile test.

## 4. End-to-end foundation

- [x] 4.1 Build the fixture-project helper (git init, clean commit, openspec layout, Makefile, `.specflow` config); verify a unit test asserts a clean `git status` and the expected files.
- [x] 4.2 Add fixture skills — write-file, ask-and-record, slow-bash, create-change, complete-tasks — with valid Agent Skills frontmatter; verify a unit test checks each has a name and description.
- [x] 4.3 Build the `pi` shim generator (pass-through, crash, slow) that records its PID and execs the real binary; verify a unit test runs `--version` through it and asserts crash mode produces the expected exit.
- [x] 4.4 Implement the SSE auto-operator for questions, approvals, rejections, and timed injections, and prove it against a mock-harness run; verify the mock interactive test passes.
- [x] 4.5 Add the e2e gate and helpers (`PI_E2E`, `PI_E2E_MODEL`, `PI_E2E_FULL`, version logging, goroutine/FD/orphan assertions); verify the package builds, skips by default, and the helpers have unit tests.
- [x] 4.6 Document the real-Pi test contract in README: env vars, `PI_CODING_AGENT_DIR`, `GOCACHE`, cheap-model requirement, and cost warning; verify the commands run as written.

## 5. Ladder: execution and state

- [x] 5.1 Rung 1 — one node, no skill, "reply ok": assert argv, SSE event kinds, completed status, worktree removal, no orphan process, stable goroutines; verify `PI_E2E=1 PI_E2E_MODEL=… go test ./backend/internal/e2e -run OneNode`.
- [x] 5.2 Rung 2 — fixture skill writes a file: assert skill loaded, tool and file events, checkpoint commit, and the `file_exists` postcondition; verify with the same command, `-run Skill`.
- [x] 5.3 Rung 3 — two nodes create and complete a change: assert worktree persistence, `{{change}}` resolution, `new_change` and `tasks_complete`, and checkpoint order; verify `-run Chain`.
- [x] 5.4 Document how to run rungs 1–3 and what to inspect in `events.jsonl`; verify the README steps against a passing run.

## 6. Ladder: interactions

- [x] 6.1 Ask — a fixture skill forced to call `specflow_ask`, answered by the auto-operator: assert `question.asked` then `question.answered` and that Pi continues to completion; verify `-run Ask`.
- [x] 6.2 Inject — a slow bash tool with a mid-run injection: assert `injection.received`, `injection.acknowledged`, the reported disposition, and uninterrupted completion; verify `-run Inject`.
- [x] 6.3 Approval — approve and reject paths: assert the gate pauses, the events and statuses are published, and a decision only applies to its attempt; verify `-run Approval`.
- [x] 6.4 Document the interactive verification steps and expected SSE order in README; verify against a passing run.

## 7. Ladder: failures

- [x] 7.1 Crash — shim sends SIGKILL mid-execution: assert unexpected-exit classification, failed run, retry and resume behavior, and no orphan; verify `-run Crash`.
- [x] 7.2 Startup and configuration — invalid model yields non-retryable `config`, missing credentials yield non-retryable `auth`; verify `-run Startup`.
- [x] 7.3 Cancellation — cancel a long node through the API: assert cancelled run/node, the process is gone, and resume works; verify `-run Cancel`.
- [x] 7.4 Timeout — a short node timeout: assert timed out, retryable, and no orphan; verify `-run Timeout`.
- [x] 7.5 Postcondition failure — a skill that writes nothing: assert a postcondition failure distinct from a harness failure and no automatic retry; verify `-run Postcondition`.
- [x] 7.6 Document the error matrix, observed messages, and recovery path in README; verify each row names its test.

## 8. Full workflow

- [x] 8.1 Run `standard-feature.yaml` on the toy OpenSpec repository with the auto-operator (`PI_E2E_FULL=1`), capture events, checkpoints, branch, and merge commit, and fix any fallout with regression tests; verify the run completes, the verify node passes against the run's change, and the merged branch passes `make test` with no active change.
- [x] 8.2 Re-run every rung plus `make test` and `openspec validate --changes --strict`; verify no regressions and record the evidence in the change.
- [x] 8.3 Update README quickstart and AGENTS.md with the proven end-to-end procedure and any architectural insight from the real runs; verify a fresh reader can follow the commands.

## Workflow follow-up

- Archive the change only after every rung that can run without external credentials passes and the remaining ones are explicitly recorded as blocked.
- Keep `TestPiSmoke` as the always-available canary; the full ladder is not part of `make test`.
