# Tasks

## 1. Interaction timeouts in the workflow model

- [x] 1.1 Add `ask_timeout` and `approval_timeout` to workflow `Defaults` and `Node`, parse them as durations, and reject a malformed value during validation with the node and field in the error; verify with compile tests for valid, omitted, and malformed values.
- [x] 1.2 Resolve configured values in `normalizeNode` — node value, else workflow default, zero when neither is set — and carry them on `PlanNode`; apply the 30-minute fallback and the question's harness-hint preference at the wait site. Verify with compile tests for node override, workflow default, and no configuration.
- [x] 1.3 Verify a resumed run keeps the interaction deadlines recorded at creation after the workflow file is edited; add the case to the existing plan-pinning tests.
- [x] 1.4 Document both fields and their precedence in the README "Running a workflow" section; verify the documented YAML parses through the compile test fixture.

## 2. Session question lifecycle

- [x] 2.1 Replace the pending channel map with a pending-question record carrying the ask, channel, and a `resolved` flag guarded by the session mutex; retain resolved entries for the session's lifetime and make `Answer` report delivered, unknown, or already-resolved, with the timeout/cancel path resolving exactly once; verify unit tests for the answer-first and timeout-first races.
- [x] 2.2 Serialize questions with a one-slot semaphore, reject a duplicate ID instead of overwriting, and start the effective deadline when the question becomes active; verify tests for a second queued question and for a duplicate ID.
- [x] 2.3 Resolve the effective ask deadline — configured, else the harness `Ask.Timeout`, else 30 minutes — and report the deadline plus a resolution reason (answered, timed out, cancelled) through `Sink`; update the engine's sink to compile; verify session tests assert each reason and the effective deadline.

## 3. Session injection queue

- [x] 3.1 Make `Session.Inject` non-blocking with a 256-entry queue and a typed queue-full error; verify a test that fills the queue and asserts the call returns without blocking.
- [x] 3.2 Verify messages accumulate in submission order while the adapter is not reading; verify a test that queues several and reads them back in order.
- [x] 3.3 Verify a session closed with a full queue rejects further injection with `ErrClosed`; verify the test.

## 4. Pending interaction in run state

- [x] 4.1 Add `Pending *PendingInteraction` to `Run` with kind, id, node, attempt, title, message, options, action, requestedAt, and deadline; verify JSON round-trip in the store tests and that a record without the field still loads.
- [x] 4.2 Add `Injections []InjectionRecord` with id, node, text, receivedAt, and disposition, capped at 100 entries; verify the cap and round-trip tests.
- [x] 4.3 Add locked helpers to set and clear the pending interaction and to append an injection, and clear pending on completed, failed, and cancelled; verify engine tests that a terminal run has no pending interaction.
- [x] 4.4 Document the `pending` and `injections` fields in the README run-metadata section; verify the documented shape matches the store test fixture.

## 5. Approval per attempt

- [x] 5.1 Move the approval gate into the attempt loop after workdir resolution and agent preflight, move `beginNode` behind the gate on the first attempt, and replace the channel plus node fields with an approval request (id, node, attempt, action, deadline, resolved) matched exactly by a decision; keep a last-resolved record so a duplicate decision maps to 410; verify the existing approval tests plus a stale-decision test.
- [x] 5.2 Add the approval deadline: on expiry clear the request, mark the node failed with class `timeout`, publish `approval.timed_out`, and fail the run; verify with a millisecond-deadline test.
- [x] 5.3 Verify every retry re-requests approval: an automatic retry and a user-initiated `Retry` each produce a second `approval.required` carrying the next attempt, and a rejected retry keeps the node's failed status; verify both retry tests and the rejection case.
- [x] 5.4 Publish `approval.required` with the request id, node, title, attempt, action, and deadline, and `approval.granted`, `approval.rejected`, or `approval.timed_out` with the id and attempt; set or clear the run's pending interaction; verify event tests and a `GET` while waiting.
- [x] 5.5 Verify cancellation during approval clears the pending interaction, marks the node cancelled, ends the run cancelled, and leaves no blocked handler; verify with a test.
- [x] 5.6 Update the README approval event rows and the approval field documentation in "Running a workflow"; verify the documented names match the test assertions.

## 6. Question integration

- [x] 6.1 On `question.asked`, set the run's pending interaction with the effective deadline and attempt, then emit the event with the deadline; verify a test that reads the run while waiting.
- [x] 6.2 On resolution, clear pending and emit `question.answered`, `question.timed_out`, or `question.cancelled` according to the session's resolution reason; verify one test per outcome.
- [x] 6.3 Return typed errors from `Engine.Answer` for an unknown question and an already-resolved interaction; verify engine tests.
- [x] 6.4 Verify cancellation during a question resolves it as cancelled, clears pending, and ends the run cancelled; verify the test.
- [x] 6.5 Update the README question event rows; verify the names match the test assertions.

## 7. Injection integration

- [x] 7.1 Make `Engine.Inject` publish `injection.received`, append the run record, and report `queued`; verify the event, the record, and the return value in engine tests.
- [x] 7.2 Publish `injection.rejected` with a reason for a non-running run, a node that disallows injection, a harness without the capability, and a full queue, and return a typed error for each; verify all four cases.
- [x] 7.3 Recognize the adapter's injection acknowledgement in `sessionSink.Emit`, update the recorded disposition, and publish `injection.acknowledged`; verify with the mock harness acknowledgement.
- [x] 7.4 Update the README injection event rows; verify the names match the test assertions.

## 8. HTTP contract

- [x] 8.1 Validate the answer request (`questionId` required) and map engine outcomes to 400, 404, 409, and 410; verify table-driven handler tests for each status.
- [x] 8.2 Validate the inject request (non-empty text, 64 KiB cap), return `queued`, and map each rejection reason to 409; verify the handler tests.
- [x] 8.3 Validate the approve request (`nodeId` required, `decision` of `approve` or `reject`, optional `attempt`, `approved` accepted as a compatibility alias) and map 400, 404, 409, and 410; verify the handler tests.
- [x] 8.4 Document the three request bodies, status codes, and the `queued` response in the README API table; verify the documented examples match the handler cases.

## 9. Integration and concurrency checks

- [x] 9.1 Add a mock-harness scenario that exercises approval, a question, injection, and completion in one run; verify the event order and the pending transition from set to cleared.
- [x] 9.2 Verify an SSE subscriber receives the interactive events in order for all three mechanisms; extend the SSE tests.
- [x] 9.3 Add a goroutine-leak check that records a baseline, runs the interactive scenario, calls `Engine.Close`, and polls back to the baseline with a bounded deadline; verify it passes with `-race`.
- [x] 9.4 Run `go test -race ./...` and fix any reported races.
- [x] 9.5 Run `openspec validate --strict` and `make test`; verify both pass.

## Workflow follow-up

- Archive the change after the project's review requirements are satisfied.
- Verify the archived `workflow-orchestration` spec contains the modified requirements and the new scenarios.
