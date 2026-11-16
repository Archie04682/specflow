package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/workspace"
)

const interruptedWorkflow = `
id: interrupted
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: explore}
  - {id: propose, kind: agent, prompt: propose}
  - {id: apply, kind: agent, prompt: apply}
`

// stageInterruptedRun creates a real run (worktree, retained workflow, node
// records) and then rewrites its record to the state a crash would leave, so a
// second engine can observe it exactly as after a restart.
func stageInterruptedRun(t *testing.T, status RunStatus, prepare func(*Run)) (string, *Run) {
	t.Helper()

	project := newRepoProject(t)
	writeProjectWorkflow(t, project, interruptedWorkflow)

	creator := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{}, allCapabilities()))
	t.Cleanup(creator.Close)
	run, err := creator.Create(context.Background(), CreateRequest{
		Project:   project,
		Workflow:  "test",
		Objective: "recover the interrupted run",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	run.Status = status
	run.CurrentNode = ""
	run.Pending = nil
	run.Error = ""
	run.FinishedAt = nil
	for index := range run.Nodes {
		run.Nodes[index].Status = NodePending
		run.Nodes[index].Attempts = 0
		run.Nodes[index].Error = ""
		run.Nodes[index].FailureClass = ""
		run.Nodes[index].EndedAt = nil
	}
	if prepare != nil {
		prepare(run)
	}

	staged, err := workspace.Open(context.Background(), project)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	if err := NewStore(staged).Save(run); err != nil {
		t.Fatalf("save interrupted run: %v", err)
	}
	return project, run
}

func waitingApprovalRun(t *testing.T) (string, *Run) {
	t.Helper()
	return stageInterruptedRun(t, RunWaitingApproval, func(run *Run) {
		run.Nodes[0].Status = NodeCompleted
		run.Nodes[1].Status = NodeWaitingApproval
		run.Nodes[1].Attempts = 1
		run.CurrentNode = "propose"
		run.Pending = &PendingInteraction{
			Kind: "approval", ID: "approval-1", NodeID: "propose", Attempt: 1,
		}
	})
}

func waitingAnswerRun(t *testing.T) (string, *Run) {
	t.Helper()
	return stageInterruptedRun(t, RunWaitingAnswer, func(run *Run) {
		run.Nodes[0].Status = NodeCompleted
		run.Nodes[1].Status = NodeRunning
		run.Nodes[1].Attempts = 1
		run.CurrentNode = "propose"
		run.Pending = &PendingInteraction{
			Kind: "question", ID: "q-1", NodeID: "propose", Attempt: 1,
		}
	})
}

func runningRun(t *testing.T) (string, *Run) {
	t.Helper()
	return stageInterruptedRun(t, RunRunning, func(run *Run) {
		run.Nodes[0].Status = NodeCompleted
		run.Nodes[1].Status = NodeRunning
		run.Nodes[1].Attempts = 1
		run.CurrentNode = "propose"
	})
}

func nodeByID(t *testing.T, run *Run, id string) *NodeRun {
	t.Helper()

	node := run.Node(id)
	if node == nil {
		t.Fatalf("run has no node %q", id)
	}
	return node
}

func reconcileEventTypes(t *testing.T, engine *Engine, project, runID string) []string {
	t.Helper()

	events := replayRunEvents(t, engine, project, runID)
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func TestRepairWaitingApproval(t *testing.T) {
	project, staged := waitingApprovalRun(t)
	engine := newTestEngine(t)

	repaired, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if repaired.Status != RunFailed {
		t.Fatalf("status = %s, want failed", repaired.Status)
	}
	if repaired.Pending != nil {
		t.Fatalf("pending survived the repair: %+v", repaired.Pending)
	}
	if !strings.Contains(repaired.Error, "server restarted") ||
		!strings.Contains(repaired.Error, "waiting for approval") {
		t.Fatalf("run error = %q, want the restart reason", repaired.Error)
	}

	propose := nodeByID(t, repaired, "propose")
	if propose.Status != NodeFailed {
		t.Fatalf("propose status = %s, want failed", propose.Status)
	}
	if propose.FailureClass != string(harness.FailureHarnessExit) {
		t.Fatalf("failure class = %q, want %q", propose.FailureClass, harness.FailureHarnessExit)
	}
	if !strings.Contains(propose.Error, "waiting for approval") {
		t.Fatalf("propose error = %q, want the restart reason", propose.Error)
	}
	if applied := nodeByID(t, repaired, "apply"); applied.Status != NodeSkipped {
		t.Fatalf("apply status = %s, want skipped", applied.Status)
	}

	want := []string{"node.failed", "node.skipped", "run.failed"}
	if got := reconcileEventTypes(t, engine, project, staged.ID); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}

	// A repaired run is an ordinary failed run: retry works.
	if err := engine.Retry(context.Background(), project, staged.ID, "propose", RetryForward); err != nil {
		t.Fatalf("retry after repair: %v", err)
	}
	if done := waitTerminal(t, engine, project, staged.ID); done.Status != RunCompleted {
		t.Fatalf("retried run status = %s (%s), want completed", done.Status, done.Error)
	}
}

func TestRepairWaitingAnswer(t *testing.T) {
	project, staged := waitingAnswerRun(t)
	engine := newTestEngine(t)

	repaired, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if repaired.Status != RunFailed || repaired.Pending != nil {
		t.Fatalf("repaired run = %s pending=%+v, want failed with no pending interaction", repaired.Status, repaired.Pending)
	}
	propose := nodeByID(t, repaired, "propose")
	if propose.Status != NodeFailed || !strings.Contains(propose.Error, "waiting for an answer") {
		t.Fatalf("propose = %s %q, want failed while waiting for an answer", propose.Status, propose.Error)
	}

	// A repaired run is an ordinary failed run: resume works.
	if err := engine.Resume(context.Background(), project, staged.ID); err != nil {
		t.Fatalf("resume after repair: %v", err)
	}
	if done := waitTerminal(t, engine, project, staged.ID); done.Status != RunCompleted {
		t.Fatalf("resumed run status = %s (%s), want completed", done.Status, done.Error)
	}
}

func TestRepairRunningNode(t *testing.T) {
	project, staged := runningRun(t)
	engine := newTestEngine(t)

	repaired, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if repaired.Status != RunFailed || repaired.Pending != nil {
		t.Fatalf("repaired run = %s pending=%+v, want failed", repaired.Status, repaired.Pending)
	}
	propose := nodeByID(t, repaired, "propose")
	if propose.Status != NodeFailed || !strings.Contains(propose.Error, "was running") {
		t.Fatalf("propose = %s %q, want failed while running", propose.Status, propose.Error)
	}

	if err := engine.Resume(context.Background(), project, staged.ID); err != nil {
		t.Fatalf("resume after repair: %v", err)
	}
	if done := waitTerminal(t, engine, project, staged.ID); done.Status != RunCompleted {
		t.Fatalf("resumed run status = %s (%s), want completed", done.Status, done.Error)
	}
}

func TestRepairBetweenNodesKeepsCompletedWork(t *testing.T) {
	project, staged := stageInterruptedRun(t, RunRunning, func(run *Run) {
		run.Nodes[0].Status = NodeCompleted
		run.CurrentNode = "explore"
	})
	engine := newTestEngine(t)

	repaired, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if repaired.Status != RunFailed {
		t.Fatalf("status = %s, want failed", repaired.Status)
	}
	if !strings.Contains(repaired.Error, "while the run was running") {
		t.Fatalf("run error = %q, want the run-level restart reason", repaired.Error)
	}
	if got := nodeByID(t, repaired, "explore").Status; got != NodeCompleted {
		t.Fatalf("completed node became %s, want it untouched", got)
	}
	if got := nodeByID(t, repaired, "propose").Status; got != NodeSkipped {
		t.Fatalf("propose status = %s, want skipped", got)
	}
	if got := nodeByID(t, repaired, "apply").Status; got != NodeSkipped {
		t.Fatalf("apply status = %s, want skipped", got)
	}
	if got := len(eventsNamed(t, engine, project, staged.ID, "node.failed")); got != 0 {
		t.Fatalf("node.failed events = %d, want none: no node was executing", got)
	}
	want := []string{"node.skipped", "node.skipped", "run.failed"}
	if got := reconcileEventTypes(t, engine, project, staged.ID); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRetryRepairsOnTheMutationPath(t *testing.T) {
	project, staged := waitingApprovalRun(t)
	engine := newTestEngine(t)

	if err := engine.Retry(context.Background(), project, staged.ID, "propose", RetryForward); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if done := waitTerminal(t, engine, project, staged.ID); done.Status != RunCompleted {
		t.Fatalf("retried run status = %s (%s), want completed", done.Status, done.Error)
	}
}

func TestResumeRepairsOnTheMutationPath(t *testing.T) {
	project, staged := waitingAnswerRun(t)
	engine := newTestEngine(t)

	if err := engine.Resume(context.Background(), project, staged.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if done := waitTerminal(t, engine, project, staged.ID); done.Status != RunCompleted {
		t.Fatalf("resumed run status = %s (%s), want completed", done.Status, done.Error)
	}
}

func TestRepairHappensOnce(t *testing.T) {
	project, staged := waitingApprovalRun(t)
	engine := newTestEngine(t)

	first, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	second, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("second get: %v", err)
	}

	if first.Status != second.Status || first.Error != second.Error {
		t.Fatalf("a repeat read changed the record: %+v vs %+v", first, second)
	}
	if got := len(eventsNamed(t, engine, project, staged.ID, "run.failed")); got != 1 {
		t.Fatalf("run.failed events = %d, want exactly one", got)
	}
	if got := len(eventsNamed(t, engine, project, staged.ID, "node.skipped")); got != 1 {
		t.Fatalf("node.skipped events = %d, want exactly one", got)
	}
}

func TestRepairUnderConcurrentReads(t *testing.T) {
	project, staged := waitingApprovalRun(t)
	engine := newTestEngine(t)

	const readers = 8
	var wg sync.WaitGroup
	results := make(chan error, readers)
	for index := 0; index < readers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := engine.Get(context.Background(), project, staged.ID)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent get: %v", err)
		}
	}
	if got := len(eventsNamed(t, engine, project, staged.ID, "run.failed")); got != 1 {
		t.Fatalf("run.failed events = %d, want exactly one", got)
	}
	if got := len(eventsNamed(t, engine, project, staged.ID, "node.skipped")); got != 1 {
		t.Fatalf("node.skipped events = %d, want the skip pass once", got)
	}
}

func TestListRepairsInterruptedRun(t *testing.T) {
	project, staged := waitingApprovalRun(t)
	engine := newTestEngine(t)

	runs, err := engine.List(context.Background(), project)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want one", len(runs))
	}
	if runs[0].Status != RunFailed || runs[0].Pending != nil {
		t.Fatalf("listed run = %s pending=%+v, want failed", runs[0].Status, runs[0].Pending)
	}

	loaded, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.Status != runs[0].Status {
		t.Fatalf("list and get disagree: %s vs %s", runs[0].Status, loaded.Status)
	}
}

func TestApproveAfterRepairIsResolved(t *testing.T) {
	project, staged := waitingApprovalRun(t)
	engine := newTestEngine(t)

	if _, err := engine.Get(context.Background(), project, staged.ID); err != nil {
		t.Fatalf("get: %v", err)
	}

	err := engine.Approve(context.Background(), project, staged.ID, ApprovalDecision{
		NodeID: "propose", Attempt: 1, Approved: true,
	})
	if !errors.Is(err, ErrInteractionResolved) {
		t.Fatalf("approve error = %v, want ErrInteractionResolved", err)
	}
	if errors.Is(err, ErrInteractionNotPending) {
		t.Fatalf("approve error = %v, want it not to be ErrInteractionNotPending", err)
	}
}

func TestAnswerAfterRepairIsResolved(t *testing.T) {
	project, staged := waitingAnswerRun(t)
	engine := newTestEngine(t)

	if _, err := engine.Get(context.Background(), project, staged.ID); err != nil {
		t.Fatalf("get: %v", err)
	}

	err := engine.Answer(context.Background(), project, staged.ID, "q-1", harness.Answer{
		ID: "q-1", Value: "late",
	})
	if !errors.Is(err, ErrInteractionResolved) {
		t.Fatalf("answer error = %v, want ErrInteractionResolved", err)
	}
	if errors.Is(err, ErrInteractionNotPending) {
		t.Fatalf("answer error = %v, want it not to be ErrInteractionNotPending", err)
	}
}

func TestRepairSurvivesAnUnreadableJournal(t *testing.T) {
	project, staged := waitingApprovalRun(t)
	journal := filepath.Join(project, ".specflow", "runs", staged.ID, "events.jsonl")
	if err := os.RemoveAll(journal); err != nil {
		t.Fatalf("clear journal: %v", err)
	}
	if err := os.MkdirAll(journal, 0o755); err != nil {
		t.Fatalf("replace journal with a directory: %v", err)
	}

	engine := newTestEngine(t)
	repaired, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if repaired.Status != RunFailed || repaired.Pending != nil {
		t.Fatalf("repaired run = %s pending=%+v, want failed without a journal", repaired.Status, repaired.Pending)
	}
	if nodeByID(t, repaired, "propose").Status != NodeFailed {
		t.Fatal("the interrupted node was not failed without a journal")
	}
}

func TestLiveRunIsNotRepaired(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, interruptedWorkflow)
	behaviors := map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q-live", Kind: harness.AskInput}},
	}
	engine := New(fakeRegistry(t, newFakeState(), behaviors, allCapabilities()))
	t.Cleanup(engine.Close)

	run := startRun(t, engine, project, "live")
	waitFor(t, engine, project, run.ID, func(run *Run) bool { return run.Status == RunWaitingAnswer })

	loaded, err := engine.Get(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.Status != RunWaitingAnswer || loaded.Pending == nil {
		t.Fatalf("live run = %s pending=%+v, want its wait untouched", loaded.Status, loaded.Pending)
	}

	// Let the live execution settle before the temp project is removed.
	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatalf("cancel live run: %v", err)
	}
	waitTerminal(t, engine, project, run.ID)
}

func TestTerminalRunIsNotRepaired(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, interruptedWorkflow)
	behaviors := map[string]*fakeBehavior{
		"explore": {Failures: 1, FailClass: harness.FailureHarnessExit, FailMessage: "boom"},
	}
	engine := New(fakeRegistry(t, newFakeState(), behaviors, allCapabilities()))
	t.Cleanup(engine.Close)

	run := startRun(t, engine, project, "fail")
	waitTerminal(t, engine, project, run.ID)
	before := reconcileEventTypes(t, engine, project, run.ID)

	observer := newTestEngine(t)
	loaded, err := observer.Get(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.Status != RunFailed {
		t.Fatalf("terminal run status = %s, want failed", loaded.Status)
	}
	after := reconcileEventTypes(t, observer, project, run.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("a terminal run gained events: %v -> %v", before, after)
	}
}

func TestPendingRunIsNotRepaired(t *testing.T) {
	project, staged := stageInterruptedRun(t, RunPending, nil)
	engine := newTestEngine(t)

	loaded, err := engine.Get(context.Background(), project, staged.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.Status != RunPending {
		t.Fatalf("pending run status = %s, want pending", loaded.Status)
	}
	if nodeByID(t, loaded, "explore").Status != NodePending {
		t.Fatal("a run that never started had its nodes changed")
	}
}
