package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/openspec"
	"example.com/specflow/backend/internal/session"
	"example.com/specflow/backend/internal/workspace"
)

// errApprovalRejected stops a run because the user said no. It is not a
// failure: the node never ran.
var errApprovalRejected = errors.New("approval rejected")

// ErrInteractionNotPending reports a request for an interaction the run is not
// waiting for.
var ErrInteractionNotPending = errors.New("interaction is not pending")

// ErrInteractionResolved reports a request for an interaction that already
// stopped waiting.
var ErrInteractionResolved = errors.New("interaction already resolved")

// ErrApprovalTimeout reports an approval gate that reached its deadline.
var ErrApprovalTimeout = errors.New("approval timed out")

// ErrNoRunningNode reports a clarification with no execution to receive it.
var ErrNoRunningNode = errors.New("run has no running node")

// ErrInjectionNotAllowed reports a node whose configuration disables injection.
var ErrInjectionNotAllowed = errors.New("node does not allow injection")

// ErrInjectionUnsupported reports a harness that cannot accept clarifications.
var ErrInjectionUnsupported = errors.New("harness does not support injection")

// ErrInjectionQueueFull reports a full clarification queue.
var ErrInjectionQueueFull = errors.New("injection queue is full")

// CreateRequest describes a run to create.
type CreateRequest struct {
	Project   string `json:"project"`
	Workflow  string `json:"workflow"`
	Objective string `json:"objective"`
}

// Engine executes workflows and keeps their state.
type Engine struct {
	registry *harness.Registry
	hub      *Hub
	now      func() time.Time
	// createWorktree is a seam for tests that need creation to fail after the
	// snapshot is written.
	createWorktree func(context.Context, *workspace.Project, string) (*workspace.Worktree, error)

	mu   sync.Mutex
	live map[string]*liveRun
	// reconcileMu serializes repair of runs that have no live execution, so
	// concurrent observations cannot repair one run twice.
	reconcileMu sync.Mutex
	base        context.Context
	baseClose   context.CancelFunc
}

type liveRun struct {
	mu       sync.Mutex
	run      *Run
	plan     *WorkflowPlan
	project  *workspace.Project
	worktree *workspace.Worktree
	session  *session.Session
	cancel   context.CancelFunc
	done     chan struct{}
	running  bool

	approval     *approvalRequest
	lastApproval *approvalClosure
}

// New returns an engine backed by a harness registry.
func New(registry *harness.Registry) *Engine {
	base, cancel := context.WithCancel(context.Background())
	return &Engine{
		registry: registry,
		hub:      NewHub(),
		now:      time.Now,
		createWorktree: func(ctx context.Context, project *workspace.Project, runID string) (*workspace.Worktree, error) {
			return project.CreateWorktree(ctx, runID)
		},
		live:      make(map[string]*liveRun),
		base:      base,
		baseClose: cancel,
	}
}

// Close stops every running execution.
func (e *Engine) Close() {
	e.baseClose()

	e.mu.Lock()
	runIDs := make([]string, 0, len(e.live))
	for _, live := range e.live {
		runIDs = append(runIDs, live.run.ID)
	}
	e.mu.Unlock()

	for _, runID := range runIDs {
		e.hub.Close(runID)
	}
	e.hub.CloseAll()
}

// Create validates the project and workflow, creates the run's workspace, and
// records the run. It does not start it.
func (e *Engine) Create(ctx context.Context, request CreateRequest) (*Run, error) {
	if strings.TrimSpace(request.Project) == "" {
		return nil, errors.New("project is required")
	}

	project, err := workspace.Open(ctx, request.Project)
	if err != nil {
		return nil, err
	}
	// Refuse before touching anything: a dirty project has no meaningful base.
	if err := project.CheckIsolatable(ctx); err != nil {
		return nil, err
	}
	if err := project.EnsureLayout(); err != nil {
		return nil, err
	}

	plan, workflowPath, source, err := compileWorkflow(project, request.Workflow)
	if err != nil {
		return nil, err
	}

	baseline, err := openspec.ListChanges(project.Root)
	if err != nil {
		return nil, err
	}

	runID := newRunID()
	snapshotPath := project.WorkflowSnapshotFile(runID)
	if err := writeFileAtomic(snapshotPath, source); err != nil {
		cleanupIncompleteRun(ctx, project, runID)
		return nil, err
	}
	worktree, err := e.createWorktree(ctx, project, runID)
	if err != nil {
		cleanupIncompleteRun(ctx, project, runID)
		return nil, err
	}

	run := &Run{
		ID:               runID,
		Project:          project.Root,
		WorkflowID:       plan.ID,
		WorkflowPath:     workflowPath,
		WorkflowSnapshot: snapshotPath,
		Status:           RunPending,
		Objective:        request.Objective,
		ChangeBaseline:   baseline,
		Nodes:            newNodeRuns(plan),
		Checkpoints:      make(map[string]string),
		SourceBranch:     worktree.Source,
		BaseCommit:       worktree.Base,
		Branch:           worktree.Branch,
		Worktree:         worktree.Path,
		StartedAt:        e.now(),
	}

	if err := NewStore(project).Save(run); err != nil {
		cleanupIncompleteRun(ctx, project, runID)
		return nil, err
	}

	live := &liveRun{
		run:      run,
		plan:     plan,
		project:  project,
		worktree: worktree,
		done:     make(chan struct{}),
	}
	e.mu.Lock()
	e.live[liveKey(project.Root, runID)] = live
	e.mu.Unlock()

	return cloneRun(run), nil
}

// compileWorkflow resolves, reads, and compiles the workflow a run references.
// The source bytes are returned so the same bytes can be retained.
func compileWorkflow(project *workspace.Project, reference string) (*WorkflowPlan, string, []byte, error) {
	path, err := ResolveWorkflowPath(project.Root, reference)
	if err != nil {
		return nil, "", nil, err
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, "", nil, fmt.Errorf("read workflow %s: %w", path, err)
	}
	roots, err := project.SkillRoots()
	if err != nil {
		return nil, "", nil, err
	}
	plan, err := CompileWorkflow(source, path, NormalizeContext{SkillRoots: roots})
	if err != nil {
		return nil, "", nil, err
	}
	return plan, path, source, nil
}

// cleanupIncompleteRun removes the state created for a run whose record was
// never persisted. It is best-effort: the caller already holds a failure. A
// worktree is unregistered before its branch is deleted.
func cleanupIncompleteRun(ctx context.Context, project *workspace.Project, runID string) {
	_ = project.RemoveWorktree(ctx, project.WorktreeDir(runID))
	_ = os.RemoveAll(project.WorktreeDir(runID))
	_ = os.RemoveAll(project.RunDir(runID))
	_, _ = project.Git(ctx, "branch", "-D", project.RunBranch(runID))
}

// Start launches the run's execution in the background.
func (e *Engine) Start(ctx context.Context, projectRoot, runID string) error {
	live, err := e.load(ctx, projectRoot, runID)
	if err != nil {
		return err
	}
	if _, err := e.planFor(live, false); err != nil {
		return err
	}
	return e.launch(live, 0)
}

// Get returns a snapshot of a run.
func (e *Engine) Get(ctx context.Context, projectRoot, runID string) (*Run, error) {
	live, err := e.inspect(ctx, projectRoot, runID)
	if err != nil {
		return nil, err
	}
	return live.snapshot(), nil
}

// List returns a project's runs, newest first.
func (e *Engine) List(ctx context.Context, projectRoot string) ([]*Run, error) {
	project, err := workspace.Open(ctx, projectRoot)
	if err != nil {
		return nil, err
	}

	runs, err := NewStore(project).List()
	if err != nil {
		return nil, err
	}

	for index, run := range runs {
		e.mu.Lock()
		live := e.live[liveKey(project.Root, run.ID)]
		e.mu.Unlock()
		if live != nil {
			runs[index] = live.snapshot()
			continue
		}
		// The tree must not advertise a waiting run that no process owns.
		if !interruptedRunStatus(run.Status) {
			continue
		}
		repaired, loadErr := e.loadRun(ctx, projectRoot, run.ID, false)
		if loadErr != nil {
			log.Printf("specflow: run %s: reconcile interrupted execution: %v", run.ID, loadErr)
			continue
		}
		runs[index] = repaired.snapshot()
	}
	return runs, nil
}

// Subscribe returns a run's retained events and, for a live run, its live
// event channel. Replay is complete before Events, so a caller writes Replay
// first. The caller must call Stream.Cancel when it disconnects.
func (e *Engine) Subscribe(ctx context.Context, projectRoot, runID string, after uint64) (*Stream, error) {
	live, err := e.inspect(ctx, projectRoot, runID)
	if err != nil {
		return nil, err
	}
	// Terminal replay opens the journal on demand. Concurrent subscribers can
	// race each other's close, so retry when a log disappears before it can be
	// read.
	for attempt := 0; attempt < 3; attempt++ {
		created, err := e.hub.Open(runID, live.project.EventsFile(runID))
		if err != nil {
			return nil, err
		}
		// Re-read the status after Open: the run may have completed while the
		// journal was being reopened. If it did, the new log has no execution
		// left to seal it and must be sealed here.
		terminal := live.snapshot().Status.Terminal()
		if terminal && (!e.isLive(live.project.Root, runID) || created) {
			e.hub.Seal(runID)
		}

		replay, events, sealed, cancel, err := e.hub.Subscribe(runID, after)
		if err != nil {
			if created {
				e.hub.Close(runID)
			}
			return nil, err
		}
		lastSeq := e.hub.LastSeq(runID)
		if sealed && len(replay) == 0 && lastSeq == 0 {
			cancel()
			continue
		}

		if !sealed {
			baseCancel := cancel
			cancel = once(func() {
				baseCancel()
				// Hold the run lock across CloseIfIdle so a concurrent Start
				// cannot begin using the log after it is judged idle.
				live.mu.Lock()
				defer live.mu.Unlock()
				if live.run.Status == RunPending && !live.running {
					e.hub.CloseIfIdle(runID)
				}
			})
		}
		stream := &Stream{
			Replay:  replay,
			Events:  events,
			Cancel:  cancel,
			Sealed:  sealed,
			LastSeq: lastSeq,
		}
		return stream, nil
	}

	if info, statErr := os.Stat(live.project.EventsFile(runID)); statErr == nil && info.Size() > 0 {
		return nil, fmt.Errorf("run %s: event stream disappeared while opening", runID)
	}
	return &Stream{Sealed: true, Cancel: func() {}}, nil
}

// Replay returns a run's retained events without creating a live
// subscription. It is used by callers that need a one-shot history.
func (e *Engine) Replay(ctx context.Context, projectRoot, runID string, after uint64) ([]Event, error) {
	live, err := e.inspect(ctx, projectRoot, runID)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		created, err := e.hub.Open(runID, live.project.EventsFile(runID))
		if err != nil {
			return nil, err
		}
		terminal := live.snapshot().Status.Terminal()
		if terminal && (!e.isLive(live.project.Root, runID) || created) {
			e.hub.Seal(runID)
		}
		lastSeq := e.hub.LastSeq(runID)
		replay, _, sealed, cancel, err := e.hub.Subscribe(runID, after)
		if err != nil {
			return nil, err
		}
		cancel()
		if !sealed {
			live.mu.Lock()
			if live.run.Status == RunPending && !live.running {
				e.hub.CloseIfIdle(runID)
			}
			live.mu.Unlock()
		}
		if terminal && len(replay) == 0 && lastSeq == 0 {
			continue
		}
		return replay, nil
	}
	if info, statErr := os.Stat(live.project.EventsFile(runID)); statErr == nil && info.Size() > 0 {
		return nil, fmt.Errorf("run %s: event stream disappeared while reading", runID)
	}
	return nil, nil
}

// Cancel stops the running node and ends the run as cancelled.
func (e *Engine) Cancel(ctx context.Context, projectRoot, runID string) error {
	live, err := e.inspect(ctx, projectRoot, runID)
	if err != nil {
		return err
	}

	live.mu.Lock()
	cancel := live.cancel
	done := live.done
	running := live.running
	live.mu.Unlock()

	if !running {
		return fmt.Errorf("run %s is not running", runID)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
	return nil
}

// Approve resolves a pending approval gate. A decision identifies either the
// exact request or the node and attempt, so a late decision cannot approve a
// later attempt.
func (e *Engine) Approve(ctx context.Context, projectRoot, runID string, decision ApprovalDecision) error {
	live, err := e.inspect(ctx, projectRoot, runID)
	if err != nil {
		return err
	}

	live.mu.Lock()
	request := live.approval
	last := live.lastApproval
	terminal := live.run.Status.Terminal()

	if request == nil || !approvalMatches(request, decision) {
		live.mu.Unlock()
		if matchesClosure(last, decision) {
			return fmt.Errorf("%w: approval for node %q (%s)", ErrInteractionResolved, decision.NodeID, last.outcome)
		}
		if terminal {
			return fmt.Errorf("%w: approval for node %q", ErrInteractionResolved, decision.NodeID)
		}
		return fmt.Errorf("%w: approval for node %q", ErrInteractionNotPending, decision.NodeID)
	}
	if request.resolved {
		live.mu.Unlock()
		return fmt.Errorf("%w: approval %q", ErrInteractionResolved, request.id)
	}

	request.resolved = true
	request.decision <- decision.Approved
	live.mu.Unlock()
	return nil
}

// approvalMatches reports whether a decision names this exact request.
func approvalMatches(request *approvalRequest, decision ApprovalDecision) bool {
	if decision.RequestID != "" {
		return decision.RequestID == request.id
	}
	if decision.NodeID != "" && decision.NodeID != request.nodeID {
		return false
	}
	if decision.Attempt != 0 && decision.Attempt != request.attempt {
		return false
	}
	return decision.NodeID != "" || decision.RequestID != ""
}

// matchesClosure reports whether a decision names the last resolved gate.
func matchesClosure(last *approvalClosure, decision ApprovalDecision) bool {
	if last == nil {
		return false
	}
	if decision.RequestID != "" {
		return decision.RequestID == last.id
	}
	if decision.NodeID != "" && decision.NodeID != last.nodeID {
		return false
	}
	if decision.Attempt != 0 && decision.Attempt != last.attempt {
		return false
	}
	return decision.NodeID != "" || decision.Attempt != 0
}

// Answer resolves a question the active execution asked.
func (e *Engine) Answer(ctx context.Context, projectRoot, runID, questionID string, answer harness.Answer) error {
	live, err := e.inspect(ctx, projectRoot, runID)
	if err != nil {
		return err
	}

	live.mu.Lock()
	active := live.session
	status := live.run.Status
	live.mu.Unlock()
	if active == nil {
		if status.Terminal() {
			return fmt.Errorf("%w: question %q", ErrInteractionResolved, questionID)
		}
		return fmt.Errorf("%w: run %s has no active question", ErrInteractionNotPending, runID)
	}

	switch active.Answer(questionID, answer) {
	case session.AnswerDelivered:
		return nil
	case session.AnswerResolved:
		return fmt.Errorf("%w: question %q", ErrInteractionResolved, questionID)
	default:
		return fmt.Errorf("%w: question %q", ErrInteractionNotPending, questionID)
	}
}

// Inject delivers a clarification into the running node. It never waits for
// the adapter: the message is queued or the request is refused.
func (e *Engine) Inject(ctx context.Context, projectRoot, runID, text string) (string, error) {
	live, err := e.inspect(ctx, projectRoot, runID)
	if err != nil {
		return "", err
	}

	live.mu.Lock()
	active := live.session
	nodeID := live.run.CurrentNode
	plan := live.plan
	terminal := live.run.Status.Terminal()
	live.mu.Unlock()

	reject := func(reason string, cause error) (string, error) {
		if !terminal {
			e.emit(live, "injection.rejected", nodeID, map[string]any{
				"reason": reason,
				"error":  cause.Error(),
			})
		}
		return "", fmt.Errorf("%w: %s", cause, reason)
	}

	if active == nil {
		return reject("no_running_node", ErrNoRunningNode)
	}
	if plan == nil {
		return reject("no_running_node", ErrNoRunningNode)
	}
	node := plan.Node(nodeID)
	if node == nil {
		return reject("no_running_node", ErrNoRunningNode)
	}
	if !node.AllowInject {
		return reject("node_disallows", ErrInjectionNotAllowed)
	}

	adapter, err := e.registry.Lookup(node.Harness)
	if err != nil {
		return reject("harness_unavailable", err)
	}
	if !adapter.Capabilities().Injection {
		return reject("harness_unsupported", ErrInjectionUnsupported)
	}

	id, err := active.Inject(ctx, text)
	if err != nil {
		if errors.Is(err, session.ErrInjectionQueueFull) {
			return reject("queue_full", ErrInjectionQueueFull)
		}
		return "", err
	}
	return id, nil
}

// Discard removes a run's worktree, keeping its record and branch.
func (e *Engine) Discard(ctx context.Context, projectRoot, runID string) error {
	live, err := e.inspect(ctx, projectRoot, runID)
	if err != nil {
		return err
	}

	live.mu.Lock()
	running := live.running
	path := live.worktree.Path
	live.mu.Unlock()
	if running {
		return fmt.Errorf("run %s is still running", runID)
	}

	return live.project.RemoveWorktree(ctx, path)
}

func (e *Engine) launch(live *liveRun, startIndex int) error {
	live.mu.Lock()
	if live.plan == nil {
		live.mu.Unlock()
		return errors.New("run has no compiled workflow")
	}
	if live.running {
		live.mu.Unlock()
		return errors.New("run is already running")
	}
	ctx, cancel := context.WithCancel(e.base)
	live.running = true
	live.cancel = cancel
	live.done = make(chan struct{})
	done := live.done
	live.mu.Unlock()

	// After a restart the hub has no log for this run yet.
	if _, err := e.hub.Open(live.run.ID, live.project.EventsFile(live.run.ID)); err != nil {
		cancel()
		live.mu.Lock()
		live.running = false
		live.cancel = nil
		live.mu.Unlock()
		close(done)
		return err
	}
	// A terminal replay may have sealed the retained log. A resume or retry
	// starts a new epoch and must be able to append again.
	e.hub.Begin(live.run.ID)

	go func() {
		defer func() {
			cancel()

			live.mu.Lock()
			terminal := live.run.Status.Terminal()
			live.mu.Unlock()
			if terminal {
				e.hub.Close(live.run.ID)
				e.mu.Lock()
				delete(e.live, liveKey(live.project.Root, live.run.ID))
				e.mu.Unlock()
			}

			live.mu.Lock()
			live.running = false
			live.cancel = nil
			live.mu.Unlock()
			close(done)
		}()
		e.runFrom(ctx, live, startIndex)
	}()
	return nil
}

func (e *Engine) runFrom(ctx context.Context, live *liveRun, startIndex int) {
	live.mu.Lock()
	plan := live.plan
	live.mu.Unlock()
	if plan == nil {
		e.failRun(live, "", errors.New("run has no compiled workflow"))
		return
	}
	order := plan.Order

	e.setRun(live, func(run *Run) {
		run.Status = RunRunning
		run.Error = ""
		run.FinishedAt = nil
		run.CurrentNode = ""
	})
	e.save(live)
	e.emit(live, "run.started", "", map[string]any{
		"workflowId": live.run.WorkflowID,
		"objective":  live.run.Objective,
	})

	for index := startIndex; index < len(order); index++ {
		if ctx.Err() != nil {
			e.finishCancelled(live)
			return
		}

		nodeID := order[index]
		node := plan.Node(nodeID)
		if node == nil {
			e.failRun(live, nodeID, fmt.Errorf("node %q disappeared from the workflow", nodeID))
			return
		}
		if e.nodeStatus(live, nodeID) == NodeCompleted {
			continue
		}

		if err := e.executeNode(ctx, live, node); err != nil {
			switch {
			case errors.Is(err, errApprovalRejected):
				e.finishCancelled(live)
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				e.finishCancelled(live)
			default:
				// Record skipped nodes before the terminal status: a reader
				// that sees "failed" must never find a node still "pending".
				e.skipRemaining(live, order[index+1:])
				e.failRun(live, nodeID, err)
			}
			return
		}
	}

	e.finishCompleted(live)
}

func (e *Engine) executeNode(ctx context.Context, live *liveRun, node *PlanNode) error {
	workdir, err := ResolveWorkdir(node, live.worktree.Path, live.project.Root)
	if err != nil {
		e.markFailed(live, node, err, "config")
		return err
	}

	if node.Kind == NodeKindAgent {
		if err := e.checkAgent(live, node); err != nil {
			e.markFailed(live, node, err, "preflight")
			return err
		}
	}

	return e.runAttempts(ctx, live, node, workdir)
}

// beginAttempt publishes the transition that makes an attempt observable as
// running. The first attempt records the node's start time; a retry only
// reopens the record and clears the previous failure.
func (e *Engine) beginAttempt(live *liveRun, node *PlanNode, attempt int) {
	e.updateNode(live, node.ID, func(record *NodeRun) {
		record.Status = NodeRunning
		if attempt == 1 {
			record.StartedAt = e.now()
		} else {
			record.EndedAt = nil
		}
		record.Error = ""
		record.FailureClass = ""
		record.ExitCode = 0
	})
	e.setRun(live, func(run *Run) {
		run.Status = RunRunning
		run.CurrentNode = node.ID
	})
	e.save(live)
}

// beginAttemptCycle records the attempt number, gates it when required, and
// publishes the start. Approval is asked again for every attempt.
func (e *Engine) beginAttemptCycle(ctx context.Context, live *liveRun, node *PlanNode, attempt int) error {
	e.updateNode(live, node.ID, func(record *NodeRun) { record.Attempts = attempt })
	if node.RequireApproval {
		if err := e.awaitApproval(ctx, live, node, attempt); err != nil {
			return err
		}
	}
	e.beginAttempt(live, node, attempt)
	e.emit(live, "node.started", node.ID, map[string]any{
		"kind":    string(node.Kind),
		"title":   node.Title,
		"attempt": attempt,
	})
	return nil
}

// previousAttempts reads the attempt count a resumed or retried node carries,
// so a new execution continues the numbering instead of restarting at one.
func (e *Engine) previousAttempts(live *liveRun, nodeID string) int {
	live.mu.Lock()
	defer live.mu.Unlock()
	if record := live.run.Node(nodeID); record != nil {
		return record.Attempts
	}
	return 0
}

// runAttempts owns the retry decision: an adapter failure is retried only when
// it was classified retryable and attempts remain; a postcondition failure
// always stops for the user.
func (e *Engine) runAttempts(ctx context.Context, live *liveRun, node *PlanNode, workdir string) error {
	runContext := RunContext{
		Objective: live.snapshot().Objective,
		Change:    live.snapshot().Change,
		RunID:     live.run.ID,
		Workdir:   workdir,
	}

	failureText := ""
	for attempt := e.previousAttempts(live, node.ID) + 1; ; attempt++ {
		if err := e.beginAttemptCycle(ctx, live, node, attempt); err != nil {
			return err
		}

		checkpoint, err := e.attemptNode(ctx, live, node, workdir, &runContext, failureText)
		if err == nil {
			e.completeNode(live, node, checkpoint, runContext.Change)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		class, retryable := classifyFailure(err)
		if retryable && attempt <= node.Retry {
			failureText = truncate(err.Error(), 2000)
			continue
		}

		e.markFailed(live, node, err, class)
		return err
	}
}

// attemptNode executes one attempt and, when it succeeds, records the
// checkpoint and checks the postcondition. A resolved change is written back
// so a retry and later nodes see it.
func (e *Engine) attemptNode(ctx context.Context, live *liveRun, node *PlanNode, workdir string, runContext *RunContext, failureText string) (string, error) {
	outcome, err := e.executeOnce(ctx, live, node, workdir, *runContext, failureText)
	e.updateNode(live, node.ID, func(record *NodeRun) {
		record.Text = outcome.Text
		record.Output = outcome.Output
		record.Command = outcome.Command
		record.ExitCode = outcome.ExitCode
		record.Usage = outcome.Usage
	})
	if err != nil {
		return "", err
	}

	checkpoint, err := live.worktree.Checkpoint(ctx, node.ID)
	if err != nil {
		return "", fmt.Errorf("checkpoint node %s: %w", node.ID, err)
	}
	e.updateNode(live, node.ID, func(record *NodeRun) { record.Checkpoint = checkpoint })
	e.recordCheckpoint(live, node.ID, checkpoint)

	change, err := EvaluatePostcondition(node.Postcondition, PostconditionEnv{
		NodeID:   node.ID,
		Root:     live.worktree.Path,
		Workdir:  workdir,
		Change:   runContext.Change,
		ExitCode: outcome.ExitCode,
		Baseline: live.snapshot().ChangeBaseline,
		Context:  *runContext,
	})
	if err != nil {
		return "", err
	}
	if change != "" {
		e.setChange(live, change)
		runContext.Change = change
	}
	return checkpoint, nil
}

// completeNode publishes a successful node.
func (e *Engine) completeNode(live *liveRun, node *PlanNode, checkpoint, change string) {
	ended := e.now()
	e.updateNode(live, node.ID, func(record *NodeRun) {
		record.Status = NodeCompleted
		record.EndedAt = &ended
		record.Error = ""
	})
	postcondition := "none"
	if node.Postcondition != nil {
		postcondition = "met"
	}
	e.save(live)
	e.emit(live, "node.completed", node.ID, map[string]any{
		"checkpoint":    checkpoint,
		"change":        change,
		"postcondition": postcondition,
	})
}

type nodeOutcome struct {
	Text     string
	Output   string
	Command  string
	ExitCode int
	Usage    harness.Usage
}

func (e *Engine) executeOnce(ctx context.Context, live *liveRun, node *PlanNode, workdir string, runContext RunContext, failureText string) (nodeOutcome, error) {
	switch node.Kind {
	case NodeKindAgent:
		return e.runAgent(ctx, live, node, workdir, runContext, failureText)
	case NodeKindCommand:
		command, err := RenderPrompt(node.Command, runContext)
		if err != nil {
			return nodeOutcome{}, err
		}
		outcome, err := e.runCommand(ctx, live, node.ID, command, workdir, node.Timeout.Duration())
		// The command ran even when the outcome is a failure; the node record
		// keeps it so a reloaded run still shows what executed.
		outcome.Command = command
		return outcome, err
	default:
		return nodeOutcome{}, fmt.Errorf("node %q has unknown kind %q", node.ID, node.Kind)
	}
}

func (e *Engine) runAgent(ctx context.Context, live *liveRun, node *PlanNode, workdir string, runContext RunContext, failureText string) (nodeOutcome, error) {
	adapter, err := e.registry.Lookup(node.Harness)
	if err != nil {
		return nodeOutcome{}, err
	}

	promptText := node.Prompt
	if failureText != "" {
		promptText = strings.TrimSpace(promptText) + "\n\nPrevious attempt failed:\n" + failureText
	}
	prompt, err := RenderPrompt(promptText, runContext)
	if err != nil {
		return nodeOutcome{}, err
	}

	request := harness.Request{
		NodeID:  node.ID,
		Prompt:  prompt,
		Skill:   node.Skill,
		Skills:  node.Skills,
		Model:   modelValue(node.Model),
		Tools:   harness.ToolPolicy{Allow: node.Tools},
		Workdir: workdir,
	}

	// The compiled timeout bounds active execution (startup, the prompt
	// handshake, and the model's work). The clock pauses while the run waits on
	// the operator, so answering slowly never spends the node's budget. The
	// adapter still classifies an expired clock as a timeout.
	clock := newExecutionClock(ctx, node.Timeout.Duration())
	defer clock.Stop()

	active := session.New(node.ID, &sessionSink{engine: e, live: live, nodeID: node.ID, clock: clock}, node.AskTimeout.Duration())
	live.mu.Lock()
	live.session = active
	live.mu.Unlock()
	defer func() {
		live.mu.Lock()
		live.session = nil
		live.mu.Unlock()
		active.Close()
	}()

	result, runErr := adapter.Run(clock, request, active)
	if runErr != nil {
		return nodeOutcome{ExitCode: 1}, runErr
	}
	if result.Status != harness.StatusCompleted {
		return nodeOutcome{}, result.Err()
	}

	return nodeOutcome{Text: result.Text, Usage: result.Usage, ExitCode: 0}, nil
}

func modelValue(model *harness.Model) harness.Model {
	if model == nil {
		return harness.Model{}
	}
	return *model
}

// runCommand runs one command node and publishes its lifecycle. The started
// event names the rendered command and its working directory before the
// process exists; the finished event carries the exit code, the bounded
// output, and the duration on every path, including a timeout and a
// cancellation.
func (e *Engine) runCommand(ctx context.Context, live *liveRun, nodeID, command, workdir string, timeout time.Duration) (nodeOutcome, error) {
	started := time.Now()
	e.emit(live, "command.started", nodeID, map[string]any{
		"command": command,
		"workdir": workdir,
	})

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	process := exec.CommandContext(commandCtx, "sh", "-c", command)
	process.Dir = workdir
	output := &boundedBuffer{limit: 64 * 1024}
	process.Stdout = output
	process.Stderr = output

	runErr := process.Run()

	// Every path publishes exactly one finish, so a reader never sees a
	// running command after its process is gone.
	finished := func(exitCode int) {
		e.emit(live, "command.finished", nodeID, map[string]any{
			"exitCode":   exitCode,
			"output":     output.String(),
			"durationMs": int(time.Since(started).Milliseconds()),
		})
	}

	if ctx.Err() != nil {
		finished(-1)
		return nodeOutcome{ExitCode: -1, Output: output.String()}, ctx.Err()
	}
	if commandCtx.Err() == context.DeadlineExceeded {
		finished(-1)
		return nodeOutcome{ExitCode: -1, Output: output.String()}, fmt.Errorf("command timed out after %s", timeout)
	}

	exitCode := 0
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			finished(-1)
			return nodeOutcome{ExitCode: -1, Output: output.String()}, runErr
		}
	}
	finished(exitCode)

	if exitCode != 0 {
		return nodeOutcome{ExitCode: exitCode, Output: output.String()},
			fmt.Errorf("command %q exited with %d: %s", command, exitCode, truncate(output.String(), 2000))
	}
	return nodeOutcome{ExitCode: 0, Output: output.String()}, nil
}

type boundedBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

func (e *Engine) checkAgent(live *liveRun, node *PlanNode) error {
	adapter, err := e.registry.Lookup(node.Harness)
	if err != nil {
		return err
	}
	return Preflight(node, adapter.Capabilities())
}

// awaitApproval holds one attempt until the user decides or, when a deadline is
// configured, the deadline passes. The request carries the attempt so a
// decision cannot be reused by a retry. Without a configured approval timeout
// the gate waits for the operator rather than failing the run.
func (e *Engine) awaitApproval(ctx context.Context, live *liveRun, node *PlanNode, attempt int) error {
	timeout := node.ApprovalTimeout.Duration()
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	request := &approvalRequest{
		id:       newInteractionID("approval"),
		nodeID:   node.ID,
		attempt:  attempt,
		action:   approvalAction(node),
		deadline: deadline,
		decision: make(chan bool, 1),
	}

	pending := &PendingInteraction{
		Kind:        "approval",
		ID:          request.id,
		NodeID:      node.ID,
		Attempt:     attempt,
		Title:       node.Title,
		Action:      request.action,
		RequestedAt: e.now(),
	}
	if !deadline.IsZero() {
		pending.Deadline = &deadline
	}

	live.mu.Lock()
	live.approval = request
	live.run.Status = RunWaitingApproval
	live.run.CurrentNode = node.ID
	live.run.Pending = pending
	if record := live.run.Node(node.ID); record != nil {
		record.Status = NodeWaitingApproval
	}
	live.mu.Unlock()

	e.save(live)
	data := map[string]any{
		"id":      request.id,
		"node":    node.ID,
		"title":   node.Title,
		"kind":    string(node.Kind),
		"action":  request.action,
		"attempt": attempt,
	}
	if !deadline.IsZero() {
		data["deadline"] = deadline
	}
	e.emit(live, "approval.required", node.ID, data)

	var timer *time.Timer
	var timerC <-chan time.Time
	if !deadline.IsZero() {
		timer = time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		timerC = timer.C
	}

	select {
	case approved := <-request.decision:
		return e.finishApproval(live, node, request, approved)
	case <-timerC:
		if approved, ok := claimApprovalDecision(live, request); ok {
			return e.finishApproval(live, node, request, approved)
		}
		return e.timeoutApproval(live, node, request)
	case <-ctx.Done():
		if approved, ok := claimApprovalDecision(live, request); ok {
			return e.finishApproval(live, node, request, approved)
		}
		e.cancelApproval(live, request)
		return ctx.Err()
	}
}

// claimApprovalDecision reads a decision that won the race with the deadline or
// cancellation. Approve marks the request resolved and sends while holding the
// run mutex, so a resolved request always has a buffered decision.
func claimApprovalDecision(live *liveRun, request *approvalRequest) (bool, bool) {
	live.mu.Lock()
	defer live.mu.Unlock()
	if live.approval != request || !request.resolved {
		return false, false
	}
	return <-request.decision, true
}

// finishApproval clears the gate and records the user's decision.
func (e *Engine) finishApproval(live *liveRun, node *PlanNode, request *approvalRequest, approved bool) error {
	outcome := "rejected"
	live.mu.Lock()
	live.approval = nil
	live.run.Pending = nil
	live.lastApproval = &approvalClosure{
		id: request.id, nodeID: request.nodeID, attempt: request.attempt, outcome: outcome,
	}
	if approved {
		outcome = "granted"
		live.lastApproval.outcome = outcome
		live.run.Status = RunRunning
	} else if record := live.run.Node(node.ID); record != nil {
		// A rejected retry keeps the failure that caused the retry.
		if request.attempt == 1 {
			record.Status = NodeSkipped
		} else {
			record.Status = NodeFailed
		}
	}
	live.mu.Unlock()

	e.save(live)
	if approved {
		e.emit(live, "approval.granted", node.ID, map[string]any{
			"id": request.id, "node": node.ID, "attempt": request.attempt,
		})
		return nil
	}
	e.emit(live, "approval.rejected", node.ID, map[string]any{
		"id": request.id, "node": node.ID, "attempt": request.attempt,
	})
	return errApprovalRejected
}

// timeoutApproval fails the node and the run when no decision arrives. It
// checks for a decision under the same lock that clears the gate, so an
// approval that won the race is never reported to the client and then ignored.
func (e *Engine) timeoutApproval(live *liveRun, node *PlanNode, request *approvalRequest) error {
	live.mu.Lock()
	if live.approval == request && request.resolved {
		approved := <-request.decision
		live.mu.Unlock()
		return e.finishApproval(live, node, request, approved)
	}
	live.approval = nil
	live.run.Pending = nil
	live.lastApproval = &approvalClosure{
		id: request.id, nodeID: request.nodeID, attempt: request.attempt, outcome: "timed_out",
	}
	live.mu.Unlock()

	err := fmt.Errorf("%w: node %q attempt %d", ErrApprovalTimeout, node.ID, request.attempt)
	e.markFailed(live, node, err, "timeout")
	e.emit(live, "approval.timed_out", node.ID, map[string]any{
		"id": request.id, "node": node.ID, "attempt": request.attempt, "deadline": request.deadline,
	})
	return err
}

// cancelApproval releases a gate interrupted by run cancellation. The run's
// cancellation event follows; no approval-specific cancellation event exists.
func (e *Engine) cancelApproval(live *liveRun, request *approvalRequest) {
	live.mu.Lock()
	live.approval = nil
	live.run.Pending = nil
	live.lastApproval = &approvalClosure{
		id: request.id, nodeID: request.nodeID, attempt: request.attempt, outcome: "cancelled",
	}
	live.mu.Unlock()
	e.save(live)
}

func approvalAction(node *PlanNode) string {
	if node.Kind == NodeKindCommand {
		return node.Command
	}
	if node.Skill != nil {
		return "run skill " + node.Skill.Name
	}
	return node.Prompt
}

func (e *Engine) finishCompleted(live *liveRun) {
	// Release the worktree before publishing the terminal state, so a reader
	// that sees "completed" never finds a workspace it should no longer have.
	_ = live.project.RemoveWorktree(context.Background(), live.worktree.Path)

	ended := e.now()
	e.setRun(live, func(run *Run) {
		run.Status = RunCompleted
		run.CurrentNode = ""
		run.Pending = nil
		run.FinishedAt = &ended
	})
	e.save(live)
	snapshot := live.snapshot()
	completed := 0
	for _, node := range snapshot.Nodes {
		if node.Status == NodeCompleted {
			completed++
		}
	}
	duration := ended.Sub(snapshot.StartedAt).Round(time.Second)
	e.emit(live, "run.completed", "", map[string]any{
		"summary": fmt.Sprintf("%d/%d nodes completed in %s", completed, len(snapshot.Nodes), duration),
	})
	e.hub.Seal(live.run.ID)
}

func (e *Engine) finishCancelled(live *liveRun) {
	ended := e.now()
	e.setRun(live, func(run *Run) {
		if run.Status.Terminal() {
			return
		}
		run.Status = RunCancelled
		run.CurrentNode = ""
		run.Pending = nil
		run.FinishedAt = &ended
	})
	e.updateRunningNode(live, NodeCancelled)
	e.save(live)
	e.emit(live, "run.cancelled", "", nil)
	e.hub.Seal(live.run.ID)
}

// failRun ends the run as failed. nodeID is empty when the failure belongs to
// the run rather than a node.
func (e *Engine) failRun(live *liveRun, nodeID string, err error) {
	ended := e.now()
	e.setRun(live, func(run *Run) {
		run.Status = RunFailed
		run.Error = err.Error()
		run.CurrentNode = ""
		run.Pending = nil
		run.FinishedAt = &ended
	})
	e.save(live)
	e.emit(live, "run.failed", nodeID, map[string]any{"error": err.Error()})
	e.hub.Seal(live.run.ID)
}

func (e *Engine) skipRemaining(live *liveRun, ids []string) {
	for _, id := range ids {
		if e.nodeStatus(live, id) == NodeCompleted {
			continue
		}
		e.updateNode(live, id, func(record *NodeRun) { record.Status = NodeSkipped })
		e.emit(live, "node.skipped", id, nil)
	}
	e.save(live)
}

func (e *Engine) markFailed(live *liveRun, node *PlanNode, err error, class string) {
	e.markNodeFailed(live, node.ID, err, class)
}

// markNodeFailed records one node's failure and publishes it. The interrupted-
// run repair knows only a node ID, so the failure tail lives here.
func (e *Engine) markNodeFailed(live *liveRun, nodeID string, err error, class string) {
	ended := e.now()
	status := NodeFailed
	if errors.Is(err, context.Canceled) {
		status = NodeCancelled
	}

	e.updateNode(live, nodeID, func(record *NodeRun) {
		record.Status = status
		record.Error = truncate(err.Error(), 8000)
		record.FailureClass = class
		record.EndedAt = &ended
	})
	e.save(live)

	eventType := "node.failed"
	var failure *PostconditionFailure
	if errors.As(err, &failure) {
		eventType = "node.postcondition_failed"
	}
	_, retryable := classifyFailure(err)
	e.emit(live, eventType, nodeID, map[string]any{
		"error":     err.Error(),
		"class":     class,
		"retryable": retryable,
	})
}

func (e *Engine) updateRunningNode(live *liveRun, status NodeStatus) {
	live.mu.Lock()
	defer live.mu.Unlock()

	for index := range live.run.Nodes {
		if live.run.Nodes[index].Status == NodeRunning || live.run.Nodes[index].Status == NodeWaitingApproval {
			ended := e.now()
			live.run.Nodes[index].Status = status
			live.run.Nodes[index].EndedAt = &ended
		}
	}
}

type sessionSink struct {
	engine *Engine
	live   *liveRun
	nodeID string
	// clock bounds the node's active execution time; it pauses while a question
	// is pending so the operator's thinking time is not charged to the node.
	clock *executionClock
}

func (s *sessionSink) Emit(event harness.Event) error {
	data := map[string]any{"kind": string(event.Kind)}
	if event.Text != "" {
		data["text"] = event.Text
	}
	if event.Tool != nil {
		data["tool"] = event.Tool
	}
	if event.File != nil {
		data["file"] = event.File
	}
	if event.Usage != nil {
		data["usage"] = event.Usage
	}
	if event.Notice != nil {
		data["notice"] = event.Notice
	}
	if event.Injection != nil {
		data["injection"] = event.Injection
		s.engine.acknowledgeInjection(s.live, s.nodeID, event.Injection)
	}
	s.engine.emit(s.live, "node.event", s.nodeID, data)
	return nil
}

// InjectionQueued records a clarification before the adapter can read it.
func (s *sessionSink) InjectionQueued(injection harness.Injection) {
	s.engine.appendInjection(s.live, InjectionRecord{
		ID:         injection.ID,
		NodeID:     s.nodeID,
		Text:       injection.Text,
		ReceivedAt: s.engine.now(),
	})
	s.engine.save(s.live)
	s.engine.emit(s.live, "injection.received", s.nodeID, map[string]any{
		"id":   injection.ID,
		"text": injection.Text,
	})
}

// acknowledgeInjection records the adapter's delivery disposition and
// publishes the lifecycle event.
func (e *Engine) acknowledgeInjection(live *liveRun, nodeID string, ack *harness.InjectionAck) {
	disposition := ack.Disposition
	if disposition == "" {
		disposition = "unknown"
	}

	live.mu.Lock()
	for index := range live.run.Injections {
		if live.run.Injections[index].ID == ack.ID {
			live.run.Injections[index].Disposition = disposition
		}
	}
	live.mu.Unlock()
	e.save(live)
	e.emit(live, "injection.acknowledged", nodeID, map[string]any{
		"id":          ack.ID,
		"disposition": disposition,
		"error":       ack.Error,
	})
}

func (s *sessionSink) Question(ask harness.Ask, deadline time.Time) {
	if s.clock != nil {
		s.clock.Pause()
	}

	s.live.mu.Lock()
	if s.live.run.Status == RunRunning {
		s.live.run.Status = RunWaitingAnswer
	}
	attempt := 0
	if record := s.live.run.Node(s.nodeID); record != nil {
		attempt = record.Attempts
	}
	pending := &PendingInteraction{
		Kind:        "question",
		ID:          ask.ID,
		NodeID:      s.nodeID,
		Attempt:     attempt,
		Title:       ask.Title,
		Message:     ask.Message,
		Options:     append([]string(nil), ask.Options...),
		RequestedAt: s.engine.now(),
	}
	if !deadline.IsZero() {
		pending.Deadline = &deadline
	}
	s.live.run.Pending = pending
	s.live.mu.Unlock()
	s.engine.save(s.live)

	data := map[string]any{
		"id":          ask.ID,
		"kind":        string(ask.Kind),
		"title":       ask.Title,
		"message":     ask.Message,
		"options":     ask.Options,
		"default":     ask.Default,
		"placeholder": ask.Placeholder,
		"prefill":     ask.Prefill,
		"attempt":     attempt,
	}
	if !deadline.IsZero() {
		data["deadline"] = deadline
	}
	s.engine.emit(s.live, "question.asked", s.nodeID, data)
}

func (s *sessionSink) Resolved(questionID string, resolution session.Resolution, answer harness.Answer) {
	if s.clock != nil {
		s.clock.Resume()
	}

	eventType := "question.answered"
	switch resolution {
	case session.ResolutionTimedOut:
		eventType = "question.timed_out"
	case session.ResolutionCancelled:
		eventType = "question.cancelled"
	}

	s.live.mu.Lock()
	if s.live.run.Status == RunWaitingAnswer {
		s.live.run.Status = RunRunning
	}
	if pending := s.live.run.Pending; pending != nil && pending.Kind == "question" && pending.ID == questionID {
		s.live.run.Pending = nil
	}
	s.live.mu.Unlock()
	s.engine.save(s.live)
	data := map[string]any{
		"id":         questionID,
		"resolution": string(resolution),
	}
	if resolution == session.ResolutionAnswered {
		data["value"] = answer.Value
		data["confirmed"] = answer.Confirmed
	}
	s.engine.emit(s.live, eventType, s.nodeID, data)
}

func (e *Engine) isLive(projectRoot, runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.live[liveKey(projectRoot, runID)] != nil
}

func (e *Engine) load(ctx context.Context, projectRoot, runID string) (*liveRun, error) {
	return e.loadRun(ctx, projectRoot, runID, true)
}

// inspect loads a run without adding it to the live set. Inspection and
// rejected mutations must not resurrect a terminal run that was evicted.
func (e *Engine) inspect(ctx context.Context, projectRoot, runID string) (*liveRun, error) {
	return e.loadRun(ctx, projectRoot, runID, false)
}

func (e *Engine) loadRun(ctx context.Context, projectRoot, runID string, cache bool) (*liveRun, error) {
	project, err := workspace.Open(ctx, projectRoot)
	if err != nil {
		return nil, err
	}

	key := liveKey(project.Root, runID)
	e.mu.Lock()
	live := e.live[key]
	e.mu.Unlock()
	if live != nil {
		return live, nil
	}

	run, err := NewStore(project).Load(runID)
	if err != nil {
		return nil, err
	}

	// A record left running or waiting by a previous process is repaired the
	// first time this process observes it. A run that became live while we
	// reconciled is returned as-is instead of being wrapped a second time.
	if interruptedRunStatus(run.Status) {
		run, err = e.reconcile(project, runID)
		if err != nil {
			return nil, err
		}
		e.mu.Lock()
		existing := e.live[key]
		e.mu.Unlock()
		if existing != nil {
			return existing, nil
		}
	}

	live = &liveRun{
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
	if !cache {
		return live, nil
	}

	e.mu.Lock()
	if existing := e.live[key]; existing != nil {
		e.mu.Unlock()
		return existing, nil
	}
	e.live[key] = live
	e.mu.Unlock()
	return live, nil
}

// planFor returns the workflow plan for a live run. A cached plan is reused
// when refresh is false; resume and retry pass true so a definition that no
// longer compiles in the current environment fails before any node executes.
// Only execution paths call it: inspection must keep working when a skill has
// disappeared since the run was created.
func (e *Engine) planFor(live *liveRun, refresh bool) (*WorkflowPlan, error) {
	live.mu.Lock()
	if live.plan != nil && !refresh {
		plan := live.plan
		live.mu.Unlock()
		return plan, nil
	}
	run := live.run
	live.mu.Unlock()

	source := run.WorkflowSnapshot
	if source == "" {
		source = run.WorkflowPath
	}
	if source == "" {
		return nil, fmt.Errorf("run %s has no workflow definition", run.ID)
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("read recorded workflow for run %s: %w", run.ID, err)
	}
	roots, err := live.project.SkillRoots()
	if err != nil {
		return nil, err
	}

	// The source label keeps an empty workflow id stable across create and
	// resume, even though resume reads the retained copy.
	label := run.WorkflowPath
	if label == "" {
		label = source
	}
	plan, err := CompileWorkflow(data, label, NormalizeContext{SkillRoots: roots})
	if err != nil {
		return nil, err
	}
	if err := planMatchesRun(plan, run); err != nil {
		return nil, err
	}

	live.mu.Lock()
	if live.plan == nil {
		live.plan = plan
	}
	cached := live.plan
	live.mu.Unlock()
	return cached, nil
}

// planMatchesRun refuses to execute a definition that is not the graph the run
// recorded. Node identifiers and kinds are compared in declaration order; a
// legacy source whose prompts or edges changed cannot be detected without a
// recorded hash, but a different node list, order, or kind is refused.
func planMatchesRun(plan *WorkflowPlan, run *Run) error {
	if len(plan.Nodes) != len(run.Nodes) {
		return fmt.Errorf("run %s recorded %d nodes but its workflow has %d", run.ID, len(run.Nodes), len(plan.Nodes))
	}

	for index := range plan.Nodes {
		planned := plan.Nodes[index]
		recorded := run.Nodes[index]
		if planned.ID != recorded.NodeID {
			return fmt.Errorf("run %s recorded node %q at position %d but its workflow has %q", run.ID, recorded.NodeID, index, planned.ID)
		}
		if planned.Kind != recorded.Kind {
			return fmt.Errorf("run %s recorded node %q as %s but its workflow has %s", run.ID, recorded.NodeID, recorded.Kind, planned.Kind)
		}
	}
	return nil
}

func (l *liveRun) snapshot() *Run {
	l.mu.Lock()
	defer l.mu.Unlock()
	return cloneRun(l.run)
}

func (e *Engine) setRun(live *liveRun, mutate func(*Run)) {
	live.mu.Lock()
	mutate(live.run)
	live.mu.Unlock()
}

func (e *Engine) updateNode(live *liveRun, nodeID string, mutate func(*NodeRun)) {
	live.mu.Lock()
	if record := live.run.Node(nodeID); record != nil {
		mutate(record)
	}
	live.mu.Unlock()
}

// appendInjection records a clarification and its delivery state.
func (e *Engine) appendInjection(live *liveRun, record InjectionRecord) {
	live.mu.Lock()
	live.run.appendInjection(record)
	live.mu.Unlock()
}

func (e *Engine) recordCheckpoint(live *liveRun, nodeID, checkpoint string) {
	live.mu.Lock()
	if live.run.Checkpoints == nil {
		live.run.Checkpoints = make(map[string]string)
	}
	live.run.Checkpoints[nodeID] = checkpoint
	live.mu.Unlock()
}

func (e *Engine) setChange(live *liveRun, change string) {
	live.mu.Lock()
	live.run.Change = change
	live.mu.Unlock()
	e.emit(live, "change.resolved", "", map[string]any{"change": change})
}

func (e *Engine) nodeStatus(live *liveRun, nodeID string) NodeStatus {
	live.mu.Lock()
	defer live.mu.Unlock()
	if record := live.run.Node(nodeID); record != nil {
		return record.Status
	}
	return ""
}

func (e *Engine) save(live *liveRun) {
	snapshot := live.snapshot()
	_ = NewStore(live.project).Save(snapshot)
}

func (e *Engine) emit(live *liveRun, eventType, nodeID string, data map[string]any) {
	if _, err := e.hub.Emit(live.run.ID, eventType, nodeID, data); err != nil {
		log.Printf("specflow: run %s: emit %s: %v", live.run.ID, eventType, err)
	}
}

func classifyFailure(err error) (string, bool) {
	var failure *harness.Failure
	if errors.As(err, &failure) {
		return string(failure.Class), failure.Retryable
	}

	var postcondition *PostconditionFailure
	if errors.As(err, &postcondition) {
		return "postcondition", false
	}
	return "internal", false
}

func liveKey(projectRoot, runID string) string {
	return projectRoot + "\x00" + runID
}

func newNodeRuns(plan *WorkflowPlan) []NodeRun {
	nodes := make([]NodeRun, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		nodes = append(nodes, NodeRun{
			NodeID: node.ID,
			Title:  node.Title,
			Kind:   node.Kind,
			Status: NodePending,
		})
	}
	return nodes
}

func newInteractionID(prefix string) string {
	return prefix + "-" + newRunID()
}

func newRunID() string {
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer[:])
}

func cloneRun(run *Run) *Run {
	data, err := json.Marshal(run)
	if err != nil {
		return run
	}
	var cloned Run
	if err := json.Unmarshal(data, &cloned); err != nil {
		return run
	}
	return &cloned
}

// ResolveWorkflowPath finds a workflow by path, by project-relative path, or
// by identifier under the project's workflow directories.
func ResolveWorkflowPath(projectRoot, reference string) (string, error) {
	if strings.TrimSpace(reference) == "" {
		return "", errors.New("workflow is required")
	}

	candidates := []string{reference}
	if !filepath.IsAbs(reference) {
		candidates = append(candidates,
			filepath.Join(projectRoot, reference),
			filepath.Join(projectRoot, ".specflow", "workflows", reference+".yaml"),
			filepath.Join(projectRoot, "workflows", reference+".yaml"),
		)
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("workflow %q not found", reference)
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
