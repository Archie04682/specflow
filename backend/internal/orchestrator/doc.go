// Package orchestrator will execute workflow graphs: it owns the DAG state
// machine, run lifecycle, parallel branch scheduling, and progress reporting.
//
// The bootstrap change creates the package boundary only; no execution logic
// exists yet.
package orchestrator
