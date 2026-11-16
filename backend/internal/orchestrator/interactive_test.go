package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/mock"
)

func eventsNamed(t *testing.T, engine *Engine, project, runID, eventType string) []Event {
	t.Helper()
	return runEventsOfType(replayRunEvents(t, engine, project, runID), eventType)
}

func TestApprovalTimeoutFailsTheNodeAndRun(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval-timeout
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true, approval_timeout: 50ms}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunFailed {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	record := done.Node("merge")
	if record.Status != NodeFailed || record.FailureClass != "timeout" {
		t.Fatalf("merge = %+v", record)
	}
	if !strings.Contains(done.Error, "timed out") {
		t.Fatalf("error = %q", done.Error)
	}
	if _, ok := hasEvent(engine, project, run.ID, "approval.timed_out"); !ok {
		t.Fatal("no approval timeout event")
	}
	if done.Pending != nil {
		t.Fatalf("a terminal run kept a pending interaction: %+v", done.Pending)
	}
}

func TestAutomaticRetryRequiresANewApproval(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval-retry
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true, retry: 1, approval_timeout: 5s}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"merge": {Failures: 1, Retryable: true},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("merge")
		return r.Status == RunWaitingApproval && record != nil && record.Attempts == 1
	})
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: true}); err != nil {
		t.Fatalf("first approval: %v", err)
	}

	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("merge")
		return r.Status == RunWaitingApproval && record != nil && record.Attempts == 2
	})
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 2, Approved: true}); err != nil {
		t.Fatalf("second approval: %v", err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if state.count("merge") != 2 {
		t.Fatalf("executions = %d", state.count("merge"))
	}
	requests := eventsNamed(t, engine, project, run.ID, "approval.required")
	if len(requests) != 2 {
		t.Fatalf("approval requests = %d, want 2", len(requests))
	}
	for index, request := range requests {
		if got := fmt.Sprint(request.Data["attempt"]); got != fmt.Sprint(index+1) {
			t.Fatalf("request %d attempt = %s", index, got)
		}
	}
}

func TestUserRetryRequiresANewApproval(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval-manual-retry
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true, approval_timeout: 5s}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"merge": {Failures: 1, Retryable: false},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: true}); err != nil {
		t.Fatal(err)
	}
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}

	if err := engine.Retry(context.Background(), project, run.ID, "merge", RetryForward); err != nil {
		t.Fatalf("retry: %v", err)
	}
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("merge")
		return r.Status == RunWaitingApproval && record != nil && record.Attempts == 2
	})
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 2, Approved: true}); err != nil {
		t.Fatalf("second approval: %v", err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if state.count("merge") != 2 {
		t.Fatalf("executions = %d", state.count("merge"))
	}
}

func TestRejectedRetryKeepsTheFailure(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval-reject-retry
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true, retry: 1, approval_timeout: 5s}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"merge": {Failures: 1, Retryable: true},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("merge")
		return r.Status == RunWaitingApproval && record != nil && record.Attempts == 1
	})
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("merge")
		return r.Status == RunWaitingApproval && record != nil && record.Attempts == 2
	})
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 2, Approved: false}); err != nil {
		t.Fatal(err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCancelled {
		t.Fatalf("status = %s", done.Status)
	}
	if record := done.Node("merge"); record.Status != NodeFailed {
		t.Fatalf("merge = %s, want failed", record.Status)
	}
}

func TestApprovalDecisionMustMatchTheRequest(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval-match
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true, approval_timeout: 5s}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	pending := waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })

	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "other", Approved: true}); !errors.Is(err, ErrInteractionNotPending) {
		t.Fatalf("wrong node error = %v", err)
	}
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 2, Approved: true}); !errors.Is(err, ErrInteractionNotPending) {
		t.Fatalf("wrong attempt error = %v", err)
	}
	if pending.Pending == nil || pending.Pending.ID == "" {
		t.Fatalf("pending interaction = %+v", pending.Pending)
	}
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{RequestID: pending.Pending.ID, Approved: true}); err != nil {
		t.Fatalf("matching approval: %v", err)
	}
	waitTerminal(t, engine, project, run.ID)

	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: true}); !errors.Is(err, ErrInteractionResolved) {
		t.Fatalf("duplicate approval error = %v", err)
	}
}

func TestApprovalPendingStateAndEvents(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval-pending
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true, approval_timeout: 5s}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	pending := waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })
	if pending.Pending == nil {
		t.Fatal("no pending interaction")
	}
	if pending.Pending.Kind != "approval" || pending.Pending.NodeID != "merge" || pending.Pending.Attempt != 1 {
		t.Fatalf("pending = %+v", pending.Pending)
	}
	if pending.Pending.Deadline == nil || pending.Pending.Action == "" {
		t.Fatalf("pending lost its deadline or action: %+v", pending.Pending)
	}

	required := eventsNamed(t, engine, project, run.ID, "approval.required")
	if len(required) != 1 {
		t.Fatalf("approval.required events = %d", len(required))
	}
	if required[0].Data["id"] != pending.Pending.ID {
		t.Fatalf("event id = %v, pending id = %s", required[0].Data["id"], pending.Pending.ID)
	}
	if required[0].Data["deadline"] == nil {
		t.Fatal("approval.required has no deadline")
	}

	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: true}); err != nil {
		t.Fatal(err)
	}
	waitTerminal(t, engine, project, run.ID)
	if _, ok := hasEvent(engine, project, run.ID, "approval.granted"); !ok {
		t.Fatal("no approval.granted event")
	}
}

func TestCancelDuringApproval(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval-cancel
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true, approval_timeout: 5s}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })

	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCancelled {
		t.Fatalf("status = %s", done.Status)
	}
	if record := done.Node("merge"); record.Status != NodeCancelled {
		t.Fatalf("merge = %s, want cancelled", record.Status)
	}
	if done.Pending != nil {
		t.Fatalf("pending survived cancellation: %+v", done.Pending)
	}
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: true}); !errors.Is(err, ErrInteractionResolved) {
		t.Fatalf("approval after cancellation = %v", err)
	}
}

func TestQuestionPendingStateAndEvents(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: question-pending
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: ask}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q1", Kind: harness.AskInput, Message: "which one?"}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "ask me")
	pending := waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingAnswer })
	if pending.Pending == nil {
		t.Fatal("no pending question")
	}
	if pending.Pending.Kind != "question" || pending.Pending.ID != "q1" || pending.Pending.NodeID != "explore" {
		t.Fatalf("pending = %+v", pending.Pending)
	}
	if pending.Pending.Deadline != nil {
		t.Fatalf("an unbounded question carried a deadline: %v", pending.Pending.Deadline)
	}

	asked := eventsNamed(t, engine, project, run.ID, "question.asked")
	if len(asked) != 1 {
		t.Fatalf("question.asked events = %d", len(asked))
	}
	if deadline, ok := asked[0].Data["deadline"]; ok {
		t.Fatalf("an unbounded question published a deadline: %v", deadline)
	}

	if err := engine.Answer(context.Background(), project, run.ID, "q1", harness.Answer{ID: "q1", Value: "the second one"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if done.Pending != nil {
		t.Fatalf("pending survived the answer: %+v", done.Pending)
	}
	answered, ok := hasEvent(engine, project, run.ID, "question.answered")
	if !ok {
		t.Fatal("no question.answered event")
	}
	if answered.Data["value"] != "the second one" {
		t.Fatalf("question.answered value = %v", answered.Data["value"])
	}
}

func TestQuestionTimeoutIsObservableAndALateAnswerIsResolved(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: question-timeout
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: ask}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q1", Timeout: 20 * time.Millisecond}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "ask me")
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunFailed {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if done.Pending != nil {
		t.Fatalf("pending survived the timeout: %+v", done.Pending)
	}
	if _, ok := hasEvent(engine, project, run.ID, "question.timed_out"); !ok {
		t.Fatal("no question.timed_out event")
	}
	if err := engine.Answer(context.Background(), project, run.ID, "q1", harness.Answer{ID: "q1", Value: "late"}); !errors.Is(err, ErrInteractionResolved) {
		t.Fatalf("late answer error = %v", err)
	}
}

func TestCancelDuringQuestion(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: question-cancel
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: ask}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q1", Timeout: 5 * time.Second}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "ask me")
	waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingAnswer })

	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCancelled {
		t.Fatalf("status = %s", done.Status)
	}
	if done.Pending != nil {
		t.Fatalf("pending survived cancellation: %+v", done.Pending)
	}
	if _, ok := hasEvent(engine, project, run.ID, "question.cancelled"); !ok {
		t.Fatal("no question.cancelled event")
	}
}

func TestInjectRejectedWhenNoNodeIsRunning(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: inject-no-node
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true, approval_timeout: 5s}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })

	if _, err := engine.Inject(context.Background(), project, run.ID, "hello"); !errors.Is(err, ErrNoRunningNode) {
		t.Fatalf("err = %v, want ErrNoRunningNode", err)
	}
	rejected := eventsNamed(t, engine, project, run.ID, "injection.rejected")
	if len(rejected) != 1 || rejected[0].Data["reason"] != "no_running_node" {
		t.Fatalf("injection.rejected = %+v", rejected)
	}

	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitTerminal(t, engine, project, run.ID)
}

func TestInjectQueueFullIsRejectedAndPublished(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: inject-full
defaults: {harness: fake}
nodes:
  - {id: apply, kind: agent, prompt: work, allow_inject: true}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"apply": {Sleep: time.Second},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "implement")
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("apply")
		return record != nil && record.Status == NodeRunning
	})

	engine.mu.Lock()
	live := engine.live[liveKey(project, run.ID)]
	engine.mu.Unlock()
	if live == nil {
		t.Fatal("the run is not live")
	}
	live.mu.Lock()
	active := live.session
	live.mu.Unlock()
	if active == nil {
		t.Fatal("the node has no active session")
	}

	// Fill the session queue directly so the test exercises the engine's
	// refusal without paying for 256 run-record writes.
	for index := 0; index < 1000; index++ {
		if _, err := active.Inject(context.Background(), "fill"); err != nil {
			break
		}
	}

	_, err := engine.Inject(context.Background(), project, run.ID, "overflow")
	if !errors.Is(err, ErrInjectionQueueFull) {
		t.Fatalf("overflow error = %v, want ErrInjectionQueueFull", err)
	}
	rejected := eventsNamed(t, engine, project, run.ID, "injection.rejected")
	if len(rejected) == 0 || rejected[len(rejected)-1].Data["reason"] != "queue_full" {
		t.Fatalf("injection.rejected = %+v", rejected)
	}

	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitTerminal(t, engine, project, run.ID)
}

func TestAllInteractiveMechanismsInOneRun(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: interactive
defaults: {harness: mock}
nodes:
  - {id: apply, kind: agent, prompt: work, require_approval: true, allow_inject: true, approval_timeout: 5s, ask_timeout: 5s}
`)
	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter {
		return mock.New(mock.Script{
			Ask:              &harness.Ask{ID: "q1", Kind: harness.AskInput, Title: "Clarify"},
			WaitForInjection: true,
		})
	}); err != nil {
		t.Fatal(err)
	}
	engine := New(registry)
	defer engine.Close()

	run := startRun(t, engine, project, "do the work")
	approval := waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })
	if approval.Pending == nil || approval.Pending.Kind != "approval" {
		t.Fatalf("approval pending = %+v", approval.Pending)
	}
	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "apply", Attempt: 1, Approved: true}); err != nil {
		t.Fatalf("approve: %v", err)
	}

	question := waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingAnswer })
	if question.Pending == nil || question.Pending.Kind != "question" {
		t.Fatalf("question pending = %+v", question.Pending)
	}
	if err := engine.Answer(context.Background(), project, run.ID, "q1", harness.Answer{ID: "q1", Value: "the first one"}); err != nil {
		t.Fatalf("answer: %v", err)
	}

	if _, err := engine.Inject(context.Background(), project, run.ID, "use encoding/csv"); err != nil {
		t.Fatalf("inject: %v", err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if done.Pending != nil {
		t.Fatalf("pending survived completion: %+v", done.Pending)
	}
	if len(done.Injections) != 1 || done.Injections[0].Disposition != "delivered" {
		t.Fatalf("injection record = %+v", done.Injections)
	}

	order := []string{
		"approval.required",
		"approval.granted",
		"question.asked",
		"question.answered",
		"injection.received",
		"injection.acknowledged",
		"run.completed",
	}
	all, err := engine.Replay(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	position := make(map[string]int, len(all))
	for index, event := range all {
		if _, seen := position[event.Type]; !seen {
			position[event.Type] = index
		}
	}
	last := -1
	for _, eventType := range order {
		index, ok := position[eventType]
		if !ok {
			t.Fatalf("missing %s in %v", eventType, eventTypesForTest(all))
		}
		if index <= last {
			t.Fatalf("%s is out of order: %v", eventType, eventTypesForTest(all))
		}
		last = index
	}
}

func eventTypesForTest(events []Event) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func TestInteractiveRunLeavesNoEngineGoroutines(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: leak
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: ask}
`)
	baseline := runtime.NumGoroutine()

	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q1", Timeout: 5 * time.Second}},
	}, allCapabilities()))
	run := startRun(t, engine, project, "ask me")
	waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingAnswer })

	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitTerminal(t, engine, project, run.ID)
	engine.Close()

	deadline := time.Now().Add(5 * time.Second)
	for {
		stacks := goroutineStacks()
		leaked := strings.Contains(stacks, "orchestrator.(*Engine).runFrom") ||
			strings.Contains(stacks, "orchestrator.(*Engine).launch.func1")
		if !leaked && runtime.NumGoroutine() <= baseline+1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines did not settle: baseline=%d now=%d\n%s", baseline, runtime.NumGoroutine(), stacks)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func goroutineStacks() string {
	buffer := make([]byte, 1<<20)
	n := runtime.Stack(buffer, true)
	return string(buffer[:n])
}

func TestQuestionWaitDoesNotConsumeTheExecutionTimeout(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: question-pause
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: ask, timeout: 80ms}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q1", Kind: harness.AskInput}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "ask me")
	waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingAnswer })

	// Well past the node timeout: a clock that kept running would fail the node.
	time.Sleep(250 * time.Millisecond)

	if err := engine.Answer(context.Background(), project, run.ID, "q1", harness.Answer{ID: "q1", Value: "answered late"}); err != nil {
		t.Fatalf("answer: %v", err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if record := done.Node("explore"); record.FailureClass != "" {
		t.Fatalf("explore failure class = %q, want none", record.FailureClass)
	}
}

func TestConfiguredAskDeadlineIsPublished(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: question-deadline
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: ask, ask_timeout: 5s}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q1", Kind: harness.AskInput}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "ask me")
	pending := waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingAnswer })
	if pending.Pending == nil || pending.Pending.Deadline == nil {
		t.Fatalf("a configured question lost its deadline: %+v", pending.Pending)
	}

	asked := eventsNamed(t, engine, project, run.ID, "question.asked")
	if len(asked) != 1 || asked[0].Data["deadline"] == nil {
		t.Fatalf("question.asked did not publish the configured deadline: %+v", asked)
	}

	if err := engine.Answer(context.Background(), project, run.ID, "q1", harness.Answer{ID: "q1", Value: "answered"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if done := waitTerminal(t, engine, project, run.ID); done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
}

func TestApprovalWithoutConfiguredDeadlineWaits(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval-unbounded
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "merge")
	pending := waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })
	if pending.Pending == nil {
		t.Fatal("no pending approval")
	}
	if pending.Pending.Deadline != nil {
		t.Fatalf("an unbounded approval carried a deadline: %v", pending.Pending.Deadline)
	}

	required := eventsNamed(t, engine, project, run.ID, "approval.required")
	if len(required) != 1 {
		t.Fatalf("approval.required events = %d", len(required))
	}
	if deadline, ok := required[0].Data["deadline"]; ok {
		t.Fatalf("an unbounded approval published a deadline: %v", deadline)
	}

	time.Sleep(150 * time.Millisecond)

	if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: true}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if done := waitTerminal(t, engine, project, run.ID); done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
}

func TestCancelWhileWaitingForAnAnswerIsNotATimeout(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: question-cancel-paused
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: ask, timeout: 60ms}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q1", Kind: harness.AskInput}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "ask me")
	waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingAnswer })

	time.Sleep(200 * time.Millisecond)

	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCancelled {
		t.Fatalf("status = %s (%s), want cancelled", done.Status, done.Error)
	}
	if strings.Contains(done.Error, "timed out") {
		t.Fatalf("cancellation was reported as a timeout: %q", done.Error)
	}
	if record := done.Node("explore"); record.Status != NodeCancelled {
		t.Fatalf("explore = %s, want cancelled", record.Status)
	}
}
