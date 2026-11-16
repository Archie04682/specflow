// Package harness defines the contract between the SpecFlow orchestrator and
// the coding-agent runtimes that execute workflow nodes.
//
// An adapter owns the runtime mechanics of one harness: process lifecycle,
// protocol translation, and shutdown. The orchestrator owns graph state,
// context between nodes, retries, and persistence. Adapters never learn what a
// workflow is; they receive one rendered request and return one terminal
// result.
//
// Signals fall into three classes: the terminal result (mandatory), questions
// and injections (gated by declared capabilities), and telemetry (best-effort).
// A harness that emits no telemetry still completes.
package harness
