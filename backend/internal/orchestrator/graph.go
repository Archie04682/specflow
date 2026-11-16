package orchestrator

import (
	"context"
	"fmt"
	"os"

	"example.com/specflow/backend/internal/workspace"
)

// RunGraph is the topology of one run's pinned workflow: the nodes in authored
// declaration order and the authored edges. It is a read-only projection of
// the retained definition, never of the execution record.
type RunGraph struct {
	RunID      string      `json:"runId"`
	WorkflowID string      `json:"workflowId"`
	Name       string      `json:"name"`
	Nodes      []GraphNode `json:"nodes"`
	Edges      []GraphEdge `json:"edges"`
}

// ReadRunGraph returns the graph a run was pinned to. It resolves the source
// the same way planFor does — the retained snapshot, else the recorded path —
// and parses it structurally: no compile, no skill resolution, no harness
// probe, no event, and no run record write. A skill that has disappeared since
// creation therefore does not prevent the graph from being read.
func ReadRunGraph(ctx context.Context, projectRoot, runID string) (*RunGraph, error) {
	project, err := workspace.Open(ctx, projectRoot)
	if err != nil {
		return nil, err
	}

	run, err := NewStore(project).Load(runID)
	if err != nil {
		// ErrRunNotFound is wrapped, so errors.Is keeps the handler's 404.
		return nil, err
	}

	source := run.WorkflowSnapshot
	if source == "" {
		source = run.WorkflowPath
	}
	if source == "" {
		return nil, fmt.Errorf("run %s has no workflow definition", runID)
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("read recorded workflow for run %s: %w", runID, err)
	}

	workflow, err := ParseWorkflow(data, source)
	if err != nil {
		return nil, err
	}

	graph := &RunGraph{
		RunID:      run.ID,
		WorkflowID: run.WorkflowID,
		Name:       workflow.Name,
		Nodes:      graphNodes(workflow),
		Edges:      graphEdges(workflow),
	}
	if graph.RunID == "" {
		graph.RunID = runID
	}
	if graph.WorkflowID == "" {
		graph.WorkflowID = workflow.ID
	}
	return graph, nil
}
