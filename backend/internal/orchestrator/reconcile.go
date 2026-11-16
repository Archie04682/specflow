package orchestrator

import (
	"fmt"
	"log"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/workspace"
)

// interruptedRunStatus reports a persisted status that implies an execution
// existed. A pending run never started and is left alone.
func interruptedRunStatus(status RunStatus) bool {
	switch status {
	case RunRunning, RunWaitingApproval, RunWaitingAnswer:
		return true
	default:
		return false
	}
}

// reconcile repairs a run that a previous process left running or waiting.
// reconcileMu makes the repair once-only: the record is re-read under the lock,
// so a concurrent observation sees the repaired status and does nothing. A run
// that became live while waiting for the lock is not orphaned and is returned
// as its current snapshot.
func (e *Engine) reconcile(project *workspace.Project, runID string) (*Run, error) {
	e.reconcileMu.Lock()
	defer e.reconcileMu.Unlock()

	e.mu.Lock()
	live := e.live[liveKey(project.Root, runID)]
	e.mu.Unlock()
	if live != nil {
		return live.snapshot(), nil
	}

	run, err := NewStore(project).Load(runID)
	if err != nil {
		return nil, err
	}
	if !interruptedRunStatus(run.Status) {
		return run, nil
	}

	e.repairInterrupted(project, run)
	return run, nil
}

// repairInterrupted turns an orphaned execution into an ordinary failed run:
// the interrupted node fails with a retryable reason naming the restart, the
// nodes that never ran are skipped, the pending interaction is cleared, and the
// run ends failed. The record is authoritative; the journal is best-effort.
func (e *Engine) repairInterrupted(project *workspace.Project, run *Run) {
	failure, nodeID := interruptionFailure(run)
	live := &liveRun{
		run:     run,
		project: project,
		worktree: &workspace.Worktree{
			Path:   run.Worktree,
			Branch: run.Branch,
			Base:   run.BaseCommit,
			Source: run.SourceBranch,
		},
		done: make(chan struct{}),
	}

	// A journal that cannot be opened must not keep the record stuck.
	if _, err := e.hub.Open(run.ID, project.EventsFile(run.ID)); err != nil {
		log.Printf("specflow: run %s: interrupted execution journal: %v", run.ID, err)
	} else {
		defer e.hub.Close(run.ID)
	}

	log.Printf("specflow: run %s: repaired interrupted execution: %v", run.ID, failure)
	if nodeID != "" {
		e.markNodeFailed(live, nodeID, failure, string(failure.Class))
	}
	e.skipPendingAfter(live, nodeID)
	e.failRun(live, nodeID, failure)
}

// interruptionFailure names what the interrupted node was doing, and returns
// the node it belongs to. A failure carries the closest fixed class —
// harness_exit, because the harness ended with the server — and stays
// retryable so the run can be retried or resumed.
func interruptionFailure(run *Run) (*harness.Failure, string) {
	nodeID := interruptedNodeID(run)

	activity := "running"
	switch {
	case run.Pending != nil && run.Pending.Kind == "approval", run.Status == RunWaitingApproval:
		activity = "waiting for approval"
	case run.Pending != nil && run.Pending.Kind == "question", run.Status == RunWaitingAnswer:
		activity = "waiting for an answer"
	}

	message := "the SpecFlow server restarted while the run was " + activity
	if nodeID != "" {
		message = fmt.Sprintf("the SpecFlow server restarted while node %q was %s", nodeID, activity)
	}
	return &harness.Failure{
		Class:     harness.FailureHarnessExit,
		Message:   message,
		Retryable: true,
	}, nodeID
}

// interruptedNodeID prefers the interaction's node, then the run's current
// node, then a node whose own status says it was executing. A node that already
// finished is never chosen: between two nodes the record still names the
// previous, completed node, and rewriting it would lose completed work.
func interruptedNodeID(run *Run) string {
	if run.Pending != nil {
		if node := run.Node(run.Pending.NodeID); node != nil && !finishedNodeStatus(node.Status) {
			return node.NodeID
		}
	}
	if node := run.Node(run.CurrentNode); node != nil && !finishedNodeStatus(node.Status) {
		return node.NodeID
	}
	for _, node := range run.Nodes {
		if node.Status == NodeRunning || node.Status == NodeWaitingApproval {
			return node.NodeID
		}
	}
	return ""
}

// finishedNodeStatus reports a node whose work is over; the repair must never
// rewrite one of these as the interrupted node.
func finishedNodeStatus(status NodeStatus) bool {
	return status == NodeCompleted || status == NodeSkipped || status == NodeCancelled
}

// skipPendingAfter marks the nodes that never ran after the interrupted node as
// skipped: a failed run must not leave pending nodes behind. Completed, failed,
// and already-skipped nodes keep their records, so a later retry does not
// duplicate their events.
func (e *Engine) skipPendingAfter(live *liveRun, nodeID string) {
	live.mu.Lock()
	nodes := make([]NodeRun, len(live.run.Nodes))
	copy(nodes, live.run.Nodes)
	live.mu.Unlock()

	started := nodeID == ""
	changed := false
	for _, node := range nodes {
		if !started {
			if node.NodeID == nodeID {
				started = true
			}
			continue
		}
		if node.Status != NodePending {
			continue
		}
		id := node.NodeID
		e.updateNode(live, id, func(record *NodeRun) { record.Status = NodeSkipped })
		e.emit(live, "node.skipped", id, nil)
		changed = true
	}
	if changed {
		e.save(live)
	}
}
