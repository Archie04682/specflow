# Proposal

## Why

The Pi adapter has only ever been exercised against a scripted stand-in. Running
the real Pi 1.5.2 binary proved the wire contract — flags, RPC handshake, bridge
extension, and `skill:<name>` registration — but it also exposed failure paths
the stand-in cannot represent. Invalid models, missing credentials, malformed
skills, and unbounded agent turns are today misclassified, silently ignored, or
never terminated. Until those paths are truthful and bounded, a real workflow
can report success it did not achieve.

## What Changes

- Fail the execution when Pi reports a provider error (`stopReason: "error"`)
  in the message stream, instead of returning `completed` with empty text, and
  add the `provider` failure class to the contract.
- Classify Pi's missing-credential rejection as `auth`, and a permanent startup
  failure such as an unknown model as non-retryable configuration.
- After Pi starts, verify every bound skill actually loaded (`get_commands`
  contains `skill:<name>`) and fail before prompting when it did not.
- Enforce the node timeout for `agent` nodes: terminate Pi, report the node as
  timed out, and keep user cancellation reported as cancelled.
- Swallow the bridge handshake notification instead of streaming it to clients
  as a node event.
- Add an opt-in real-Pi verification ladder: a fixture project and fixture
  skills, a `pi` shim for crash and slow-process cases, an SSE auto-operator
  that answers questions and approvals, and tests gated by `PI_E2E=1`. The
  existing adapter smoke test remains the canary.
- Document the real-Pi procedure, the cheap-model requirement, and the
  environment constraints discovered while probing.

Out of scope: new harness adapters, UI work, persistence changes, and any change
to the HTTP surface.

## Capabilities

### New Capabilities

None. SpecFlow's behavior is unchanged; the e2e harness is test infrastructure,
so no new capability is introduced for it.

### Modified Capabilities

- `harness-pi-adapter`: provider-error detection, skill-load verification,
  bridge-marker suppression, and timeout vs. cancellation classification.
- `harness-protocol`: authentication, configuration, and start failures are
  distinguished so a permanent failure is not reported as retryable.
- `workflow-orchestration`: agent nodes enforce their declared timeout rather
  than running until a human cancels.

## Impact

- `backend/internal/harness/pi` — adapter lifecycle, translation, and their
  tests; the bridge source is unchanged.
- `backend/internal/orchestrator` — the agent execution path gets a deadline and
  deadline-aware classification.
- New opt-in test assets (fixture repository, fixture skills, shim, SSE
  auto-operator) and tests gated by `PI_E2E=1`, with a model required only for
  the rungs that execute one.
- `README.md` and `AGENTS.md` gain the real-Pi procedure; `make test` still
  needs no Pi, credentials, or network.
- No public HTTP API change. Failure payloads carry `auth`, `provider`, and
  `timeout`; `provider` is new, and only transient provider errors are
  retryable.
