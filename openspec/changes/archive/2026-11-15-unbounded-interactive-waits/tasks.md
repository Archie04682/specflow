# Tasks

## 1. The paused execution clock

- [x] 1.1 Add `executionClock` in `internal/orchestrator` as a `context.Context` that tracks active time, with `Pause`, `Resume`, and `Stop`, and verify unit tests for a paused budget extending past the timeout, resuming with the remaining time, expiry reporting `context.DeadlineExceeded`, parent cancellation, a zero timeout never expiring, and stopping safely.
- [x] 1.2 Wire the clock into `runAgent` and `sessionSink` so `Question` pauses and `Resolved` resumes; verify an engine test where a fake harness asks, the answer arrives after the node timeout, and the node still completes.

## 2. Questions wait for the operator

- [x] 2.1 Remove the default question deadline from `internal/session`: a zero configured and suggested timeout means no timer and a zero deadline reported to the sink; verify tests for an unbounded question staying pending, a configured `ask_timeout` still timing out, and the harness suggestion still bounding a question.
- [x] 2.2 Omit the deadline from the `question.asked` event and the pending record when it is zero, and verify an engine test asserting the field is absent for an unbounded question and present for a configured one.

## 3. Approvals wait for the operator

- [x] 3.1 Remove the 30-minute approval fallback: `awaitApproval` arms a timer only for a configured `approval_timeout`, omits the deadline from the pending record and `approval.required` event when unbounded, and still resolves the request on run cancellation; verify engine tests for an unanswered approval staying waiting, a configured approval timeout still failing the run as timed out, and cancellation while waiting ending the run cancelled.
- [x] 3.2 Verify the approval event's deadline field is absent when unbounded and present when configured.

## 4. Cancellation while waiting

- [x] 4.1 Verify an engine test: a question pending on a paused clock, the run is cancelled, and the run ends cancelled rather than failed as timed out, with no timeout failure recorded.

## 5. Frontend expectations

- [x] 5.1 Verify `AskCard` renders "no deadline" and `ApprovalCard` omits the deadline suffix when the pending interaction carries none, and that a configured deadline still displays; add the component tests (no production change expected).

## 6. Documentation

- [x] 6.1 Update the README interactive-waits section: no fallback deadlines, configured values only, the harness suggestion for questions, and the execution timeout pausing while a question is pending; verify the YAML example and prose match the implementation.
- [x] 6.2 Add the AGENTS.md orchestrator note that waiting for the operator does not consume a node's execution timeout and that interaction deadlines exist only when configured.

## 7. Integration verification

- [x] 7.1 Run `make test`, `make build`, `openspec validate unbounded-interactive-waits --strict`, and `openspec validate --changes --strict`; verify all pass and record the results.
- [ ] 7.2 With a small node timeout and no `ask_timeout`, hold a question longer than the node timeout and answer it; verify the run continues. Repeat for an approval gate with no `approval_timeout`; record both observations.

## Workflow follow-up

- Archive this change after the validation gates and the manual wait pass, landing the implementation, the synced specs, and the archive in one commit.
- `activity-console` remains independent; archive `run-dashboard` before it so the shared capability keeps its Purpose.
