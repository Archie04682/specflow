# Tasks

## 1. Repair the interrupted execution

- [x] 1.1 Add the interruption repair: resolve the interrupted node (pending
      interaction, then current node, then a running/waiting node), fail it with
      a retryable `harness_exit` failure whose message names the restart and
      what the node was doing, skip the later nodes that are still pending, and
      end the run failed with the same error. Factor the node-failure tail out of
      `markFailed` so it can be given a node ID rather than a plan node. Verify
      with an engine test that stages a `waiting_approval` record in one engine,
      observes it from a second engine over the same project, and asserts the run
      is failed, the pending approval is gone, the node carries the restart
      reason, and the later nodes are skipped.
- [x] 1.2 Cover the other two interrupted states with the same fixture style:
      `waiting_answer` clears the pending question and fails the node, and
      `running` fails the running node with no interaction. Verify that Retry and
      Resume are accepted once each record is repaired.

## 2. Observation paths and events

- [x] 2.1 Wire repair into `loadRun` for both the cached and uncached paths, and
      into `List` for non-terminal records that are not live. Verify a `Get` and
      a `List` after a restart both report failed and agree with each other.
- [x] 2.2 Publish `node.failed`, `node.skipped`, and `run.failed` by opening the
      run's journal, emitting, sealing, and closing, with the record repair still
      applied when the journal cannot be opened. Verify the journal replays
      `node.failed` then `run.failed` in sequence order, and that a directory
      placed at the journal path leaves the record repaired.
- [x] 2.3 Confirm the interaction aftermath: `Approve` and `Answer` for a
      repaired run return the resolved interaction error, not the not-pending
      error. Verify with engine tests asserting the error identity for both.

## 3. Concurrency and exclusions

- [x] 3.1 Add the engine-level repair lock and re-read the record under it so
      concurrent observations repair once. Verify with concurrent `Get` calls
      that exactly one `run.failed` event is appended and the skip pass runs
      once.
- [x] 3.2 Leave the excluded cases untouched: a run executing in the current
      process keeps its status and pending interaction, a terminal run changes
      nothing and appends nothing, and a persisted `pending` run stays pending.
      Verify each with a test.

## 4. Documentation and gates

- [x] 4.1 Document the behavior in `README.md` and `AGENTS.md`: a restart
      interrupts an in-flight run, the first read repairs it into an ordinary
      failed run, and it stays resumable; state the one-server-per-checkout
      assumption. Verify the documented commands and behavior match the tests.
- [x] 4.2 Run `make test` and `openspec validate reconcile-orphaned-runs
      --strict`; verify both pass.
- [x] 4.3 Replay the motivating failure on the real record
      `.specflow/runs/e914c5985495dc09`: start the new server, read the run, and
      confirm it flips from `waiting_approval` to failed with the restart reason,
      the dock's approval card is gone, and Resume re-asks the approval. Verify
      with the REST record and the dashboard.

## Workflow follow-up

- Archive the change after `make test` and `openspec validate reconcile-orphaned-runs --strict` pass.
