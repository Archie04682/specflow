// Package orchestrator turns a workflow definition into an executed run: it
// owns the graph model, node preparation, postconditions, retry, and the run
// record. It never talks to a harness directly; it goes through the adapter
// contract in package harness.
package orchestrator

import (
	"errors"
	"time"

	"example.com/specflow/backend/internal/harness"
)

// ErrRunNotFound reports a run identifier the project does not have.
var ErrRunNotFound = errors.New("run not found")

// RunStatus is the lifecycle state of a run.
type RunStatus string

const (
	RunPending         RunStatus = "pending"
	RunRunning         RunStatus = "running"
	RunWaitingApproval RunStatus = "waiting_approval"
	RunWaitingAnswer   RunStatus = "waiting_answer"
	RunCompleted       RunStatus = "completed"
	RunFailed          RunStatus = "failed"
	RunCancelled       RunStatus = "cancelled"
)

// AllRunStatuses lists every run status, for tests and exhaustive switches.
var AllRunStatuses = []RunStatus{
	RunPending, RunRunning, RunWaitingApproval, RunWaitingAnswer,
	RunCompleted, RunFailed, RunCancelled,
}

// Terminal reports whether the run can no longer advance.
func (s RunStatus) Terminal() bool {
	return s == RunCompleted || s == RunFailed || s == RunCancelled
}

// NodeStatus is the lifecycle state of one node within a run.
type NodeStatus string

const (
	NodePending         NodeStatus = "pending"
	NodeRunning         NodeStatus = "running"
	NodeWaitingApproval NodeStatus = "waiting_approval"
	NodeCompleted       NodeStatus = "completed"
	NodeFailed          NodeStatus = "failed"
	NodeSkipped         NodeStatus = "skipped"
	NodeCancelled       NodeStatus = "cancelled"
)

// AllNodeStatuses lists every node status, for tests and exhaustive switches.
var AllNodeStatuses = []NodeStatus{
	NodePending, NodeRunning, NodeWaitingApproval, NodeCompleted,
	NodeFailed, NodeSkipped, NodeCancelled,
}

// NodeRun is one node's record inside a run.
type NodeRun struct {
	NodeID       string        `json:"nodeId"`
	Title        string        `json:"title,omitempty"`
	Kind         NodeKind      `json:"kind"`
	Status       NodeStatus    `json:"status"`
	Attempts     int           `json:"attempts,omitempty"`
	Text         string        `json:"text,omitempty"`
	Error        string        `json:"error,omitempty"`
	FailureClass string        `json:"failureClass,omitempty"`
	ExitCode     int           `json:"exitCode,omitempty"`
	Output       string        `json:"output,omitempty"`
	Command      string        `json:"command,omitempty"`
	Checkpoint   string        `json:"checkpoint,omitempty"`
	Usage        harness.Usage `json:"usage,omitempty"`
	StartedAt    time.Time     `json:"startedAt,omitempty"`
	EndedAt      *time.Time    `json:"endedAt,omitempty"`
}

// PendingInteraction is the blocking interaction a run is waiting for. A run
// has at most one: approval precedes execution and questions are serialized.
type PendingInteraction struct {
	Kind        string     `json:"kind"`
	ID          string     `json:"id"`
	NodeID      string     `json:"nodeId"`
	Attempt     int        `json:"attempt,omitempty"`
	Title       string     `json:"title,omitempty"`
	Message     string     `json:"message,omitempty"`
	Options     []string   `json:"options,omitempty"`
	Action      string     `json:"action,omitempty"`
	RequestedAt time.Time  `json:"requestedAt,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`
}

// InjectionRecord is one clarification accepted by a run and its delivery
// disposition. The history is bounded; it is diagnostic state, not a journal.
type InjectionRecord struct {
	ID          string    `json:"id"`
	NodeID      string    `json:"nodeId"`
	Text        string    `json:"text"`
	ReceivedAt  time.Time `json:"receivedAt"`
	Disposition string    `json:"disposition,omitempty"`
}

// maxInjectionRecords bounds the per-run clarification history.
const maxInjectionRecords = 100

// Run is the durable record of one workflow execution.
type Run struct {
	ID               string              `json:"id"`
	Project          string              `json:"project"`
	WorkflowID       string              `json:"workflowId,omitempty"`
	WorkflowPath     string              `json:"workflowPath,omitempty"`
	WorkflowSnapshot string              `json:"workflowSnapshot,omitempty"`
	Status           RunStatus           `json:"status"`
	Objective        string              `json:"objective"`
	Change           string              `json:"change,omitempty"`
	ChangeBaseline   []string            `json:"changeBaseline,omitempty"`
	Nodes            []NodeRun           `json:"nodes"`
	Checkpoints      map[string]string   `json:"checkpoints,omitempty"`
	Pending          *PendingInteraction `json:"pending,omitempty"`
	Injections       []InjectionRecord   `json:"injections,omitempty"`
	SourceBranch     string              `json:"sourceBranch,omitempty"`
	BaseCommit       string              `json:"baseCommit,omitempty"`
	Branch           string              `json:"branch,omitempty"`
	Worktree         string              `json:"worktree,omitempty"`
	CurrentNode      string              `json:"currentNode,omitempty"`
	Error            string              `json:"error,omitempty"`
	StartedAt        time.Time           `json:"startedAt"`
	FinishedAt       *time.Time          `json:"finishedAt,omitempty"`
}

// appendInjection records one clarification, dropping the oldest entries when
// the bounded history is full.
func (r *Run) appendInjection(record InjectionRecord) {
	r.Injections = append(r.Injections, record)
	if len(r.Injections) > maxInjectionRecords {
		r.Injections = append([]InjectionRecord(nil), r.Injections[len(r.Injections)-maxInjectionRecords:]...)
	}
}

// Node returns the record for a node identifier.
func (r *Run) Node(id string) *NodeRun {
	return findNode(r.Nodes, id, func(node NodeRun) string { return node.NodeID })
}

// RunContext is the closed set of values a prompt may reference.
type RunContext struct {
	Objective string
	Change    string
	RunID     string
	Workdir   string
}

// Values returns the substitution table for prompt rendering.
func (c RunContext) Values() map[string]string {
	return map[string]string{
		"objective": c.Objective,
		"change":    c.Change,
		"run_id":    c.RunID,
		"workdir":   c.Workdir,
	}
}
