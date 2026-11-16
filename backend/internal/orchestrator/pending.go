package orchestrator

import "time"

// ApprovalDecision is a user's response to one approval request.
//
// RequestID, when set, identifies the exact request; NodeID and Attempt are
// accepted for clients that track the run record instead. Attempt zero means
// "the current request for this node".
type ApprovalDecision struct {
	RequestID string
	NodeID    string
	Attempt   int
	Approved  bool
}

// approvalRequest is one attempt's approval gate. resolved is guarded by the
// run mutex and makes the gate resolve exactly once.
type approvalRequest struct {
	id       string
	nodeID   string
	attempt  int
	action   string
	deadline time.Time
	resolved bool
	decision chan bool
}

// approvalClosure records the last resolved gate so a duplicate decision can
// be reported as already resolved rather than as an unknown interaction.
type approvalClosure struct {
	id      string
	nodeID  string
	attempt int
	outcome string
}
