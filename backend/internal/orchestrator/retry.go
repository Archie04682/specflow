package orchestrator

import (
	"context"
	"fmt"
)

// Retry modes for a failed node.
const (
	// RetryForward re-runs the node on the current workspace.
	RetryForward = "forward"
	// RetryClean restores the node's starting checkpoint first.
	RetryClean = "clean"
)

// Retry re-executes a failed node and continues the run from there. Completed
// nodes are never re-executed: their effects are already on the branch.
func (e *Engine) Retry(ctx context.Context, projectRoot, runID, nodeID, mode string) error {
	live, err := e.load(ctx, projectRoot, runID)
	if err != nil {
		return err
	}

	live.mu.Lock()
	running := live.running
	status := live.run.Status
	record := live.run.Node(nodeID)
	var nodeStatus NodeStatus
	if record != nil {
		nodeStatus = record.Status
	}
	live.mu.Unlock()

	if running {
		return fmt.Errorf("run %s is running", runID)
	}
	if status != RunFailed {
		return fmt.Errorf("run %s is %s, not failed", runID, status)
	}
	if record == nil {
		return fmt.Errorf("run %s has no node %q", runID, nodeID)
	}
	if nodeStatus != NodeFailed {
		return fmt.Errorf("node %q is %s, not failed", nodeID, nodeStatus)
	}

	plan, err := e.planFor(live, true)
	if err != nil {
		return err
	}
	order := plan.Order
	index := indexOf(order, nodeID)
	if index < 0 {
		return fmt.Errorf("node %q is not in the workflow", nodeID)
	}

	switch mode {
	case RetryForward:
		// Nothing to restore: the workspace is the state the failed attempt left.
	case RetryClean:
		if err := live.worktree.Restore(ctx, e.checkpointBefore(live, order, index)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown retry mode %q", mode)
	}

	e.resetFrom(live, order, index)
	return e.launch(live, index)
}

// Resume continues a failed or cancelled run at its first unfinished node.
func (e *Engine) Resume(ctx context.Context, projectRoot, runID string) error {
	live, err := e.load(ctx, projectRoot, runID)
	if err != nil {
		return err
	}

	live.mu.Lock()
	running := live.running
	status := live.run.Status
	live.mu.Unlock()

	if running {
		return fmt.Errorf("run %s is running", runID)
	}
	if status != RunFailed && status != RunCancelled {
		return fmt.Errorf("run %s is %s, not resumable", runID, status)
	}

	plan, err := e.planFor(live, true)
	if err != nil {
		return err
	}
	order := plan.Order

	index := -1
	for position, id := range order {
		if e.nodeStatus(live, id) != NodeCompleted {
			index = position
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("run %s has no unfinished node", runID)
	}

	e.resetFrom(live, order, index)
	return e.launch(live, index)
}

func (e *Engine) resetFrom(live *liveRun, order []string, index int) {
	live.mu.Lock()
	live.run.Status = RunRunning
	live.run.Error = ""
	live.run.FinishedAt = nil
	live.run.CurrentNode = ""
	for position := index; position < len(order); position++ {
		if record := live.run.Node(order[position]); record != nil {
			record.Status = NodePending
			record.Error = ""
			record.FailureClass = ""
			record.EndedAt = nil
		}
	}
	live.mu.Unlock()
	e.save(live)
}

func (e *Engine) checkpointBefore(live *liveRun, order []string, index int) string {
	live.mu.Lock()
	defer live.mu.Unlock()

	for position := index - 1; position >= 0; position-- {
		if checkpoint, ok := live.run.Checkpoints[order[position]]; ok {
			return checkpoint
		}
	}
	return live.run.BaseCommit
}

func indexOf(values []string, want string) int {
	for index, value := range values {
		if value == want {
			return index
		}
	}
	return -1
}
