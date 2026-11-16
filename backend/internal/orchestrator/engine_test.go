package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/workspace"
)

const linearWorkflow = `
id: linear
defaults:
  harness: fake
nodes:
  - id: a
    kind: agent
    prompt: first
  - id: b
    kind: agent
    prompt: second
  - id: c
    kind: agent
    prompt: third
edges:
  - {from: a, to: b}
  - {from: b, to: c}
`

func TestRunCompletesInOrder(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "do the thing")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	for _, id := range []string{"a", "b", "c"} {
		if record := done.Node(id); record == nil || record.Status != NodeCompleted {
			t.Fatalf("node %s = %+v", id, record)
		}
	}
	if got := strings.Join(state.order, ","); got != "a,b,c" {
		t.Fatalf("execution order = %s", got)
	}
}

func TestFailingNodeSkipsTheRest(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"b": {Failures: 1, Retryable: false, FailMessage: "boom"},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "fail at b")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunFailed {
		t.Fatalf("status = %s", done.Status)
	}
	if record := done.Node("a"); record.Status != NodeCompleted {
		t.Fatalf("a = %s", record.Status)
	}
	if record := done.Node("b"); record.Status != NodeFailed {
		t.Fatalf("b = %s", record.Status)
	}
	if record := done.Node("c"); record.Status != NodeSkipped {
		t.Fatalf("c = %s", record.Status)
	}
	if state.count("c") != 0 {
		t.Fatal("a skipped node was executed")
	}
}

func TestNodeOutcomeComesFromTheAdapterResult(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: one
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"a": {Events: 3, Failures: 1, Retryable: false},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "events are not completion")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunFailed {
		t.Fatalf("a stream that ended before settling was treated as completion")
	}
	if _, ok := hasEvent(engine, project, run.ID, "node.event"); !ok {
		t.Fatal("expected the scripted telemetry to be recorded")
	}
}

func TestCommandNodeOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		status   RunStatus
		exitCode int
		contains string
	}{
		{
			name:     "exit zero",
			workflow: "id: c\nnodes:\n  - {id: gate, kind: command, command: \"true\"}\n",
			status:   RunCompleted,
			exitCode: 0,
		},
		{
			name:     "non-zero exit",
			workflow: "id: c\nnodes:\n  - {id: gate, kind: command, command: \"exit 3\"}\n",
			status:   RunFailed,
			exitCode: 3,
			contains: "exited with 3",
		},
		{
			name:     "timeout",
			workflow: "id: c\nnodes:\n  - {id: gate, kind: command, command: \"sleep 2\", timeout: 100ms}\n",
			status:   RunFailed,
			exitCode: -1,
			contains: "timed out",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := newRepoProject(t)
			writeProjectWorkflow(t, project, test.workflow)
			engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
			defer engine.Close()

			run := startRun(t, engine, project, "gate")
			done := waitTerminal(t, engine, project, run.ID)

			if done.Status != test.status {
				t.Fatalf("status = %s (%s)", done.Status, done.Error)
			}
			record := done.Node("gate")
			if record.ExitCode != test.exitCode {
				t.Fatalf("exit code = %d, want %d", record.ExitCode, test.exitCode)
			}
			if test.contains != "" && !strings.Contains(record.Error, test.contains) {
				t.Fatalf("error %q does not contain %q", record.Error, test.contains)
			}
		})
	}
}

// replayRunEvents returns every event the run's journal holds. The run is
// terminal and evicted, so this reads the journal back from disk.
func replayRunEvents(t *testing.T, engine *Engine, projectRoot, runID string) []Event {
	t.Helper()

	events, err := engine.Replay(context.Background(), projectRoot, runID, 0)
	if err != nil {
		t.Fatalf("replay run %s: %v", runID, err)
	}
	return events
}

// runEventsOfType filters a replayed journal by event type.
func runEventsOfType(events []Event, eventType string) []Event {
	var matched []Event
	for _, event := range events {
		if event.Type == eventType {
			matched = append(matched, event)
		}
	}
	return matched
}

// runEventIndex returns the position of the first event of a type, or -1.
func runEventIndex(events []Event, eventType string) int {
	for index, event := range events {
		if event.Type == eventType {
			return index
		}
	}
	return -1
}

// eventNumber reads a numeric event-data field, which replay decodes as float.
func eventNumber(t *testing.T, event Event, key string) float64 {
	t.Helper()

	value, ok := event.Data[key].(float64)
	if !ok {
		t.Fatalf("event %s data %q = %v, want a number", event.Type, key, event.Data[key])
	}
	return value
}

func TestCommandNodePublishesLifecycle(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: command
nodes:
  - {id: gate, kind: command, command: "echo gate-ok"}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "gate")
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	record := done.Node("gate")
	if record.Command != "echo gate-ok" {
		t.Fatalf("node command = %q, want the rendered command", record.Command)
	}
	if !strings.Contains(record.Output, "gate-ok") {
		t.Fatalf("node output = %q, want the captured output", record.Output)
	}

	events := replayRunEvents(t, engine, project, run.ID)
	started := runEventsOfType(events, "command.started")
	if len(started) != 1 {
		t.Fatalf("command.started events = %d, want exactly one", len(started))
	}
	if started[0].NodeID != "gate" {
		t.Fatalf("command.started node = %q, want gate", started[0].NodeID)
	}
	if got := started[0].Data["command"]; got != "echo gate-ok" {
		t.Fatalf("command.started command = %v, want the rendered command", got)
	}
	if got := started[0].Data["workdir"]; got != done.Worktree {
		t.Fatalf("command.started workdir = %v, want %q", got, done.Worktree)
	}

	finished := runEventsOfType(events, "command.finished")
	if len(finished) != 1 {
		t.Fatalf("command.finished events = %d, want exactly one", len(finished))
	}
	if finished[0].NodeID != "gate" {
		t.Fatalf("command.finished node = %q, want gate", finished[0].NodeID)
	}
	if got := eventNumber(t, finished[0], "exitCode"); got != 0 {
		t.Fatalf("command.finished exit code = %v, want 0", got)
	}
	output, _ := finished[0].Data["output"].(string)
	if !strings.Contains(output, "gate-ok") {
		t.Fatalf("command.finished output = %q, want the captured output", output)
	}
	if got := eventNumber(t, finished[0], "durationMs"); got < 0 {
		t.Fatalf("command.finished duration = %v, want a non-negative duration", got)
	}

	startedAt := runEventIndex(events, "command.started")
	finishedAt := runEventIndex(events, "command.finished")
	if startedAt < 0 || finishedAt < 0 || startedAt > finishedAt {
		t.Fatalf("command.started at %d, command.finished at %d, want start before finish", startedAt, finishedAt)
	}
}

func TestCommandNodeFailurePublishesFinishFirst(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: command
nodes:
  - {id: gate, kind: command, command: "echo boom; exit 3"}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "gate")
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunFailed {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	record := done.Node("gate")
	if record.Command != "echo boom; exit 3" {
		t.Fatalf("node command = %q, want the rendered command", record.Command)
	}
	if record.ExitCode != 3 {
		t.Fatalf("node exit code = %d, want 3", record.ExitCode)
	}

	events := replayRunEvents(t, engine, project, run.ID)
	finished := runEventsOfType(events, "command.finished")
	if len(finished) != 1 {
		t.Fatalf("command.finished events = %d, want exactly one", len(finished))
	}
	if got := eventNumber(t, finished[0], "exitCode"); got != 3 {
		t.Fatalf("command.finished exit code = %v, want 3", got)
	}
	output, _ := finished[0].Data["output"].(string)
	if !strings.Contains(output, "boom") {
		t.Fatalf("command.finished output = %q, want the captured output", output)
	}

	failedAt := runEventIndex(events, "node.failed")
	if failedAt < 0 {
		t.Fatal("the failing node was not reported")
	}
	if finishedAt := runEventIndex(events, "command.finished"); finishedAt > failedAt {
		t.Fatalf("command.finished at %d, node.failed at %d, want the finish first", finishedAt, failedAt)
	}
}

func TestCommandNodeTimeoutPublishesFinish(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: command
nodes:
  - {id: gate, kind: command, command: "sleep 2", timeout: 100ms}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "gate")
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunFailed {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if record := done.Node("gate"); record.Command != "sleep 2" {
		t.Fatalf("node command = %q, want the rendered command", record.Command)
	}

	events := replayRunEvents(t, engine, project, run.ID)
	finished := runEventsOfType(events, "command.finished")
	if len(finished) != 1 {
		t.Fatalf("command.finished events = %d, want exactly one after a timeout", len(finished))
	}
	if got := eventNumber(t, finished[0], "exitCode"); got != -1 {
		t.Fatalf("command.finished exit code = %v, want -1 for a terminated command", got)
	}

	failedAt := runEventIndex(events, "node.failed")
	if failedAt < 0 {
		t.Fatal("the timed-out node was not reported as failed")
	}
	if finishedAt := runEventIndex(events, "command.finished"); finishedAt > failedAt {
		t.Fatalf("command.finished at %d, node.failed at %d, want the finish first", finishedAt, failedAt)
	}
}

func TestCommandNodeCancellationPublishesFinish(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: command
nodes:
  - {id: gate, kind: command, command: "while true; do :; done", timeout: 1m}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "gate")
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("gate")
		return record != nil && record.Status == NodeRunning
	})
	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCancelled {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	events := replayRunEvents(t, engine, project, run.ID)
	if len(runEventsOfType(events, "command.started")) != 1 {
		t.Fatal("a cancelled command did not publish its start")
	}
	finished := runEventsOfType(events, "command.finished")
	if len(finished) != 1 {
		t.Fatalf("command.finished events = %d, want exactly one after cancellation", len(finished))
	}
	if got := eventNumber(t, finished[0], "exitCode"); got != -1 {
		t.Fatalf("command.finished exit code = %v, want -1 for a cancelled command", got)
	}
	if finishedAt, cancelledAt := runEventIndex(events, "command.finished"), runEventIndex(events, "run.cancelled"); cancelledAt < 0 || finishedAt > cancelledAt {
		t.Fatalf("command.finished at %d, run.cancelled at %d, want the finish first", finishedAt, cancelledAt)
	}
}

func TestCheckpointIsRecordedAfterANode(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: checkpoint
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: write}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"a": {Writes: map[string]string{"made.txt": "work"}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "make a file")
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if done.Checkpoints["a"] == "" {
		t.Fatal("no checkpoint recorded for the node")
	}
	if done.Node("a").Checkpoint != done.Checkpoints["a"] {
		t.Fatal("node and run disagree about the checkpoint")
	}
}

func TestPostconditionFailureStopsTheRun(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: postcondition
defaults: {harness: fake}
nodes:
  - id: a
    kind: agent
    prompt: pretend to work
    postcondition: {type: file_exists, path: marker.txt, description: "marker must exist"}
  - {id: b, kind: agent, prompt: next}
edges:
  - {from: a, to: b}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "prove it")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunFailed {
		t.Fatalf("a completed adapter with an unmet postcondition did not fail the run")
	}
	if record := done.Node("a"); record.FailureClass != "postcondition" {
		t.Fatalf("failure class = %q", record.FailureClass)
	}
	if record := done.Node("b"); record.Status != NodeSkipped {
		t.Fatalf("b = %s", record.Status)
	}
	failure, ok := hasEvent(engine, project, run.ID, "node.postcondition_failed")
	if !ok {
		t.Fatal("postcondition failure was not reported separately")
	}
	if retryable, ok := failure.Data["retryable"].(bool); !ok || retryable {
		t.Fatalf("postcondition retryable = %v", failure.Data["retryable"])
	}
}

func TestApprovalGate(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: approval
defaults: {harness: fake}
nodes:
  - {id: merge, kind: agent, prompt: merge, require_approval: true}
`)

	t.Run("approved", func(t *testing.T) {
		state := newFakeState()
		engine := New(fakeRegistry(t, state, nil, allCapabilities()))
		defer engine.Close()

		run := startRun(t, engine, project, "merge it")
		waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })

		if _, ok := hasEvent(engine, project, run.ID, "approval.required"); !ok {
			t.Fatal("no approval request was emitted")
		}
		if state.count("merge") != 0 {
			t.Fatal("the gated node ran before approval")
		}

		if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: true}); err != nil {
			t.Fatal(err)
		}
		done := waitTerminal(t, engine, project, run.ID)
		if done.Status != RunCompleted {
			t.Fatalf("status = %s (%s)", done.Status, done.Error)
		}
		if state.count("merge") != 1 {
			t.Fatal("the approved node did not run")
		}
	})

	t.Run("rejected", func(t *testing.T) {
		state := newFakeState()
		engine := New(fakeRegistry(t, state, nil, allCapabilities()))
		defer engine.Close()

		run := startRun(t, engine, project, "merge it")
		waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingApproval })

		if err := engine.Approve(context.Background(), project, run.ID, ApprovalDecision{NodeID: "merge", Attempt: 1, Approved: false}); err != nil {
			t.Fatal(err)
		}
		done := waitTerminal(t, engine, project, run.ID)
		if done.Status != RunCancelled {
			t.Fatalf("status = %s", done.Status)
		}
		if record := done.Node("merge"); record.Status != NodeSkipped {
			t.Fatalf("merge = %s", record.Status)
		}
		if state.count("merge") != 0 {
			t.Fatal("a rejected node was executed")
		}
	})
}

func TestQuestionRouting(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: question
defaults: {harness: fake}
nodes:
  - {id: explore, kind: agent, prompt: ask}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"explore": {Ask: &harness.Ask{ID: "q1", Kind: harness.AskInput, Message: "name?"}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "ask me")
	waitFor(t, engine, project, run.ID, func(r *Run) bool { return r.Status == RunWaitingAnswer })

	if _, ok := hasEvent(engine, project, run.ID, "question.asked"); !ok {
		t.Fatal("no question event was emitted")
	}

	answer := harness.Answer{ID: "q1", Value: "Alice"}
	if err := engine.Answer(context.Background(), project, run.ID, "q1", answer); err != nil {
		t.Fatal(err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if done.Node("explore").Text != "Alice" {
		t.Fatalf("the answer did not reach the execution: %q", done.Node("explore").Text)
	}
	if _, ok := hasEvent(engine, project, run.ID, "question.answered"); !ok {
		t.Fatal("no question resolution was emitted")
	}
}

func TestInjectionRouting(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: inject
defaults: {harness: fake}
nodes:
  - {id: apply, kind: agent, prompt: implement, allow_inject: true}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"apply": {WaitForInjection: true, Sleep: 200 * time.Millisecond},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "implement")
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("apply")
		return record != nil && record.Status == NodeRunning
	})

	if _, err := engine.Inject(context.Background(), project, run.ID, "use encoding/csv"); err != nil {
		t.Fatalf("inject: %v", err)
	}

	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if _, ok := hasEvent(engine, project, run.ID, "injection.received"); !ok {
		t.Fatal("no injection event was emitted")
	}
	if _, ok := hasEvent(engine, project, run.ID, "injection.acknowledged"); !ok {
		t.Fatal("no injection acknowledgement was emitted")
	}
	if len(done.Injections) != 1 || done.Injections[0].Disposition != "delivered" {
		t.Fatalf("injection record = %+v", done.Injections)
	}

	if _, err := engine.Inject(context.Background(), project, run.ID, "too late"); err == nil {
		t.Fatal("injecting into a finished run succeeded")
	}
}

func TestInjectionRefusedWhenTheNodeDisallowsIt(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: noinject
defaults: {harness: fake}
nodes:
  - {id: apply, kind: agent, prompt: implement, allow_inject: false}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"apply": {Sleep: 200 * time.Millisecond},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "implement")
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("apply")
		return record != nil && record.Status == NodeRunning
	})

	if _, err := engine.Inject(context.Background(), project, run.ID, "nope"); err == nil {
		t.Fatal("injection was accepted for a node that disallows it")
	}
	if _, ok := hasEvent(engine, project, run.ID, "injection.rejected"); !ok {
		t.Fatal("no injection.rejected event was emitted")
	}
	waitTerminal(t, engine, project, run.ID)
}

func TestCancellationStopsTheRunningNode(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: cancel
defaults: {harness: fake}
nodes:
  - {id: long, kind: agent, prompt: work, timeout: 1m}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"long": {Sleep: 10 * time.Second},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "long work")
	waitFor(t, engine, project, run.ID, func(r *Run) bool {
		record := r.Node("long")
		return record != nil && record.Status == NodeRunning
	})

	if err := engine.Cancel(context.Background(), project, run.ID); err != nil {
		t.Fatal(err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCancelled {
		t.Fatalf("status = %s", done.Status)
	}
	if record := done.Node("long"); record.Status != NodeCancelled {
		t.Fatalf("node = %s", record.Status)
	}
}

func TestFixForwardRetry(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: retry
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work, retry: 2}
`)

	t.Run("retryable failure is retried", func(t *testing.T) {
		state := newFakeState()
		engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
			"a": {Failures: 1, Retryable: true},
		}, allCapabilities()))
		defer engine.Close()

		run := startRun(t, engine, project, "retry me")
		done := waitTerminal(t, engine, project, run.ID)
		if done.Status != RunCompleted {
			t.Fatalf("status = %s (%s)", done.Status, done.Error)
		}
		if record := done.Node("a"); record.Attempts != 2 {
			t.Fatalf("attempts = %d", record.Attempts)
		}
		if state.count("a") != 2 {
			t.Fatalf("executions = %d", state.count("a"))
		}
	})

	t.Run("non-retryable failure is not retried", func(t *testing.T) {
		state := newFakeState()
		engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
			"a": {Failures: 1, Retryable: false},
		}, allCapabilities()))
		defer engine.Close()

		run := startRun(t, engine, project, "do not retry")
		done := waitTerminal(t, engine, project, run.ID)
		if done.Status != RunFailed {
			t.Fatalf("status = %s", done.Status)
		}
		if state.count("a") != 1 {
			t.Fatalf("executions = %d", state.count("a"))
		}
	})

	t.Run("postcondition failure is not retried automatically", func(t *testing.T) {
		project := newRepoProject(t)
		writeProjectWorkflow(t, project, `
id: postretry
defaults: {harness: fake}
nodes:
  - id: a
    kind: agent
    prompt: work
    retry: 3
    postcondition: {type: file_exists, path: marker.txt}
`)
		state := newFakeState()
		engine := New(fakeRegistry(t, state, nil, allCapabilities()))
		defer engine.Close()

		run := startRun(t, engine, project, "no marker")
		done := waitTerminal(t, engine, project, run.ID)
		if done.Node("a").Attempts != 1 {
			t.Fatalf("a postcondition failure was retried automatically: %d attempts", done.Node("a").Attempts)
		}
	})
}

func TestFailureClassRetryPolicy(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: classretry
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work, retry: 1}
`)

	cases := []struct {
		name         string
		class        harness.FailureClass
		retryable    bool
		want         RunStatus
		wantAttempts int
	}{
		{"provider", harness.FailureProvider, true, RunCompleted, 2},
		{"auth", harness.FailureAuth, false, RunFailed, 1},
		{"config", harness.FailureConfig, false, RunFailed, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := newFakeState()
			engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
				"a": {
					Failures:    1,
					Retryable:   tc.retryable,
					FailClass:   tc.class,
					FailMessage: tc.name + " failure",
				},
			}, allCapabilities()))
			defer engine.Close()

			run := startRun(t, engine, project, "retry "+tc.name)
			done := waitTerminal(t, engine, project, run.ID)
			if done.Status != tc.want {
				t.Fatalf("status = %s, want %s (%s)", done.Status, tc.want, done.Error)
			}
			if got := done.Node("a").Attempts; got != tc.wantAttempts {
				t.Fatalf("attempts = %d, want %d", got, tc.wantAttempts)
			}
		})
	}
}

func TestAgentNodeTimeout(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: agenttimeout
defaults: {harness: fake}
nodes:
  - {id: slow, kind: agent, prompt: work, timeout: 50ms}
`)

	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"slow": {Sleep: 2 * time.Second},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "time out")
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunFailed {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	record := done.Node("slow")
	if record.Status != NodeFailed {
		t.Fatalf("node = %s, want failed", record.Status)
	}
	if record.FailureClass != string(harness.FailureTimeout) {
		t.Fatalf("failure class = %q, want %q", record.FailureClass, harness.FailureTimeout)
	}
}

func TestCleanRetryRestoresTheCheckpoint(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: cleanretry
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: base}
  - {id: b, kind: agent, prompt: risky}
edges:
  - {from: a, to: b}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"a": {Writes: map[string]string{"base.txt": "ok"}},
		"b": {
			CheckAbsent: "junk.txt",
			Writes:      map[string]string{"junk.txt": "mess"},
			Failures:    1,
			Retryable:   false,
		},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "retry cleanly")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}

	worktree := failed.Worktree
	if _, err := os.Stat(filepath.Join(worktree, "junk.txt")); err != nil {
		t.Fatalf("the failed attempt left no file to clean up: %v", err)
	}

	if err := engine.Retry(context.Background(), project, run.ID, "b", RetryClean); err != nil {
		t.Fatal(err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if state.count("b") != 2 {
		t.Fatalf("b executions = %d", state.count("b"))
	}
}

func TestResumeSkipsCompletedNodes(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"b": {Failures: 1, Retryable: false},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "resume")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}

	if err := engine.Resume(context.Background(), project, run.ID); err != nil {
		t.Fatal(err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if state.count("a") != 1 {
		t.Fatalf("completed node a ran %d times", state.count("a"))
	}
	if state.count("b") != 2 || state.count("c") != 1 {
		t.Fatalf("b = %d, c = %d", state.count("b"), state.count("c"))
	}
}

func TestResumeKeepsTheResolvedChange(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: changeresume
defaults: {harness: fake}
nodes:
  - id: propose
    kind: agent
    prompt: propose
    postcondition: {type: new_change}
  - {id: apply, kind: agent, prompt: apply}
edges:
  - {from: propose, to: apply}
`)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"propose": {Writes: map[string]string{"openspec/changes/feat/tasks.md": "- [x] 1.1 done\n"}},
		"apply":   {Failures: 1, Retryable: false},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "resume a change")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Change != "feat" {
		t.Fatalf("change = %q", failed.Change)
	}

	if err := engine.Resume(context.Background(), project, run.ID); err != nil {
		t.Fatal(err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Change != "feat" {
		t.Fatalf("resume lost the resolved change: %q", done.Change)
	}
	if state.count("propose") != 1 {
		t.Fatalf("propose ran %d times", state.count("propose"))
	}
}

func TestWorkspaceLifetime(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: lifetime
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work}
`)

	t.Run("completed run releases its worktree", func(t *testing.T) {
		engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
		defer engine.Close()

		run := startRun(t, engine, project, "complete")
		done := waitTerminal(t, engine, project, run.ID)

		if _, err := os.Stat(done.Worktree); !os.IsNotExist(err) {
			t.Fatalf("worktree still exists: %v", err)
		}
		if !branchExists(t, project, done.Branch) {
			t.Fatal("run branch was removed")
		}
	})

	t.Run("failed run keeps its worktree until discarded", func(t *testing.T) {
		engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
			"a": {Failures: 1, Retryable: false},
		}, allCapabilities()))
		defer engine.Close()

		run := startRun(t, engine, project, "fail")
		done := waitTerminal(t, engine, project, run.ID)

		if _, err := os.Stat(done.Worktree); err != nil {
			t.Fatalf("failed run lost its worktree: %v", err)
		}

		if err := engine.Discard(context.Background(), project, run.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(done.Worktree); !os.IsNotExist(err) {
			t.Fatalf("discard left the worktree: %v", err)
		}
	})
}

func TestEventSequenceAndReplay(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "events")
	waitTerminal(t, engine, project, run.ID)

	events, err := engine.Replay(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("only %d events", len(events))
	}
	for index := 1; index < len(events); index++ {
		if events[index].Seq <= events[index-1].Seq {
			t.Fatalf("sequence went backwards at %d", index)
		}
	}

	middle := events[len(events)/2].Seq
	replay, err := engine.Replay(context.Background(), project, run.ID, middle)
	if err != nil {
		t.Fatalf("replay after cursor: %v", err)
	}
	if len(replay) != len(events)-len(events)/2-1 {
		t.Fatalf("replay length = %d", len(replay))
	}
	for _, event := range replay {
		if event.Seq <= middle {
			t.Fatalf("replay returned an event at or before the cursor: %d", event.Seq)
		}
	}

	data, err := os.ReadFile(filepath.Join(project, ".specflow", "runs", run.ID, "events.jsonl"))
	if err != nil {
		t.Fatalf("event log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(events) {
		t.Fatalf("log has %d lines, hub holds %d events", len(lines), len(events))
	}
	var first Event
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("first log line: %v", err)
	}
	if first.Type != "run.started" {
		t.Fatalf("first event = %q", first.Type)
	}
}

func TestRunRecordSurvivesRestart(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "persist")
	waitTerminal(t, engine, project, run.ID)

	// A new engine has nothing in memory: this is a restart.
	restarted := New(harness.NewRegistry())
	defer restarted.Close()

	loaded, err := restarted.Get(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("load after restart: %v", err)
	}
	if loaded.Status != RunCompleted {
		t.Fatalf("status = %s", loaded.Status)
	}
	if loaded.Checkpoints["a"] == "" {
		t.Fatal("checkpoints were lost")
	}

	runs, err := restarted.List(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("list = %+v", runs)
	}
}

func branchExists(t *testing.T, projectRoot, name string) bool {
	t.Helper()

	project, err := workspace.Open(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	output, err := project.Git(context.Background(), "branch", "--list", name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(output) != ""
}

func TestTerminalCleanupEvictsLiveAndKeepsReplay(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "terminal cleanup")
	waitTerminal(t, engine, project, run.ID)

	engine.mu.Lock()
	_, cached := engine.live[liveKey(project, run.ID)]
	engine.mu.Unlock()
	if cached {
		t.Fatal("terminal run remained in the live set")
	}

	for i := 0; i < 2; i++ {
		if _, err := engine.Get(context.Background(), project, run.ID); err != nil {
			t.Fatalf("get terminal run: %v", err)
		}
	}
	if engine.hub.log(run.ID) != nil {
		t.Fatal("Get opened a journal for a terminal run")
	}
	engine.mu.Lock()
	cached = len(engine.live) != 0
	engine.mu.Unlock()
	if cached {
		t.Fatal("inspection re-added a terminal run to the live set")
	}

	stream, err := engine.Subscribe(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer stream.Cancel()
	if !stream.Sealed {
		t.Fatal("terminal subscription was not sealed")
	}
	if stream.Events != nil {
		t.Fatal("sealed subscription returned a live channel")
	}
	if len(stream.Replay) == 0 {
		t.Fatal("terminal subscription returned no replay")
	}
	if stream.Replay[len(stream.Replay)-1].Type != "run.completed" {
		t.Fatalf("last replay event = %s", stream.Replay[len(stream.Replay)-1].Type)
	}

	tail, err := engine.Replay(context.Background(), project, run.ID, stream.LastSeq)
	if err != nil {
		t.Fatalf("tail replay: %v", err)
	}
	if len(tail) != 0 {
		t.Fatalf("tail replay = %d events", len(tail))
	}
}

func TestSubscribeEndsWhenRunTerminates(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: slow
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"a": {Ask: &harness.Ask{ID: "q", Title: "Question"}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "live subscription")
	waitFor(t, engine, project, run.ID, func(run *Run) bool {
		return run.Status == RunWaitingAnswer
	})
	stream, err := engine.Subscribe(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer stream.Cancel()
	if stream.Sealed {
		t.Fatal("subscription was already sealed")
	}
	if err := engine.Answer(context.Background(), project, run.ID, "q", harness.Answer{ID: "q", Value: "ok"}); err != nil {
		t.Fatalf("answer: %v", err)
	}

	deadline := time.After(5 * time.Second)
	last := ""
	for {
		select {
		case event, open := <-stream.Events:
			if !open {
				if last != "run.completed" {
					t.Fatalf("last live event = %q", last)
				}
				return
			}
			last = event.Type
		case <-deadline:
			t.Fatal("live stream did not close after the run terminated")
		}
	}
}

func TestNodeStartedCarriesAttemptOnRetry(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: retry
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work, retry: 1}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"a": {Failures: 2, Retryable: true},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "retry payload")
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunFailed {
		t.Fatalf("status = %s", done.Status)
	}

	events, err := engine.Replay(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var starts []int
	var failed *Event
	for index := range events {
		event := events[index]
		if event.Type == "node.started" && event.NodeID == "a" {
			attempt, _ := event.Data["attempt"].(float64)
			starts = append(starts, int(attempt))
		}
		if event.Type == "node.failed" {
			failed = &event
		}
	}
	if len(starts) != 2 || starts[0] != 1 || starts[1] != 2 {
		t.Fatalf("node.started attempts = %v", starts)
	}
	if failed == nil {
		t.Fatal("node.failed event was not emitted")
	}
	if retryable, ok := failed.Data["retryable"].(bool); !ok || !retryable {
		t.Fatalf("retryable = %v", failed.Data["retryable"])
	}
}

func TestTransitionPayloadsOnSuccess(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: payloads
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work}
`)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "payloads")
	waitTerminal(t, engine, project, run.ID)
	events, err := engine.Replay(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}

	var started, nodeStarted, completed, runCompleted *Event
	for index := range events {
		event := events[index]
		switch event.Type {
		case "run.started":
			started = &event
		case "node.started":
			nodeStarted = &event
		case "node.completed":
			completed = &event
		case "run.completed":
			runCompleted = &event
		}
	}
	if started == nil || started.Data["workflowId"] != "payloads" || started.Data["objective"] != "payloads" {
		t.Fatalf("run.started = %+v", started)
	}
	if nodeStarted == nil || nodeStarted.Data["attempt"] != float64(1) {
		t.Fatalf("node.started = %+v", nodeStarted)
	}
	if completed == nil || completed.Data["postcondition"] != "none" {
		t.Fatalf("node.completed = %+v", completed)
	}
	if runCompleted == nil || runCompleted.Data["summary"] == "" {
		t.Fatalf("run.completed = %+v", runCompleted)
	}
}

func TestResumeContinuesEventSequence(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"b": {Failures: 1, Retryable: false},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "resume sequence")
	waitTerminal(t, engine, project, run.ID)

	before, err := engine.Replay(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := before[len(before)-1].Seq
	if err := engine.Resume(context.Background(), project, run.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	waitTerminal(t, engine, project, run.ID)

	after, err := engine.Replay(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) <= len(before) {
		t.Fatalf("resume added %d events", len(after)-len(before))
	}
	if after[len(before)].Seq != last+1 {
		t.Fatalf("first resumed event seq = %d, want %d", after[len(before)].Seq, last+1)
	}
}

func TestConcurrentTerminalSubscriptions(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "concurrent replay")
	waitTerminal(t, engine, project, run.ID)

	type result struct {
		stream *Stream
		err    error
	}
	results := make(chan result, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, err := engine.Subscribe(context.Background(), project, run.ID, 0)
			results <- result{stream: stream, err: err}
		}()
	}
	wg.Wait()
	close(results)

	for result := range results {
		if result.err != nil {
			t.Fatalf("subscribe: %v", result.err)
		}
		if !result.stream.Sealed {
			t.Fatal("terminal subscription was not sealed")
		}
		if len(result.stream.Replay) == 0 {
			t.Fatal("concurrent terminal subscription lost replay")
		}
		if result.stream.Replay[len(result.stream.Replay)-1].Type != "run.completed" {
			t.Fatalf("last event = %s", result.stream.Replay[len(result.stream.Replay)-1].Type)
		}
	}
}

func TestPendingSubscriptionCancelReleasesJournal(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run, err := engine.Create(context.Background(), CreateRequest{
		Project:   project,
		Workflow:  "test",
		Objective: "pending subscription",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	stream, err := engine.Subscribe(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if stream.Sealed {
		t.Fatal("pending subscription was sealed")
	}
	if engine.hub.log(run.ID) == nil {
		t.Fatal("subscription did not open the journal")
	}
	stream.Cancel()
	if engine.hub.log(run.ID) != nil {
		t.Fatal("cancel left the pending run's journal open")
	}
}

func TestResumeAfterTerminalSubscriptionStartsNewEpoch(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"b": {Failures: 1, Retryable: false},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "resume after replay")
	waitTerminal(t, engine, project, run.ID)

	stream, err := engine.Subscribe(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatalf("terminal subscribe: %v", err)
	}
	defer stream.Cancel()
	if !stream.Sealed || len(stream.Replay) == 0 {
		t.Fatalf("sealed = %v, replay = %d", stream.Sealed, len(stream.Replay))
	}

	if err := engine.Resume(context.Background(), project, run.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	waitTerminal(t, engine, project, run.ID)

	events, err := engine.Replay(context.Background(), project, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	starts := 0
	for _, event := range events {
		if event.Type == "run.started" {
			starts++
		}
	}
	if starts != 2 {
		t.Fatalf("run.started count = %d", starts)
	}
}
