package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"example.com/specflow/backend/internal/workspace"
)

// routedWorkflow is a diamond whose middle is a command gate. The gate routes
// on whether the review agent left a findings file, and fix_report only exists
// on the issues branch, so a skipped branch has a node of its own to prove
// propagation and a join that still runs.
func routedWorkflow(gateCommand string) string {
	return fmt.Sprintf(`
id: routed
defaults:
  harness: fake
nodes:
  - {id: review, kind: agent, prompt: review the diff}
  - {id: gate, kind: command, command: %q, routes: {0: clean, 1: issues}}
  - {id: fix, kind: agent, prompt: fix the findings}
  - {id: fix_report, kind: agent, prompt: summarise the fixes}
  - {id: verify, kind: agent, prompt: verify the result}
edges:
  - {from: review, to: gate}
  - {from: gate, to: fix, when: issues}
  - {from: gate, to: verify, when: clean}
  - {from: fix, to: fix_report}
  - {from: fix_report, to: verify}
`, gateCommand)
}

const routingGate = "test ! -f findings.txt"

const diamondWorkflow = `
id: diamond
defaults:
  harness: fake
nodes:
  - {id: a, kind: agent, prompt: first}
  - {id: b, kind: agent, prompt: second}
  - {id: c, kind: agent, prompt: third}
  - {id: d, kind: agent, prompt: fourth}
edges:
  - {from: a, to: b}
  - {from: a, to: c}
  - {from: b, to: d}
  - {from: c, to: d}
`

func nodeCompletedEvent(t *testing.T, engine *Engine, projectRoot, runID, nodeID string) Event {
	t.Helper()

	events, err := engine.Replay(context.Background(), projectRoot, runID, 0)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	for _, event := range events {
		if event.Type == "node.completed" && event.NodeID == nodeID {
			return event
		}
	}
	t.Fatalf("no node.completed event for %q", nodeID)
	return Event{}
}

func requireNodeStatus(t *testing.T, run *Run, nodeID string, want NodeStatus) *NodeRun {
	t.Helper()

	record := run.Node(nodeID)
	if record == nil {
		t.Fatalf("run has no node %q", nodeID)
	}
	if record.Status != want {
		t.Fatalf("node %q = %s, want %s", nodeID, record.Status, want)
	}
	return record
}

// An unconditional diamond still runs every node, in the order the flattened
// topological sort produced before routing existed.
func TestUnroutedDiamondKeepsItsOrder(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, diamondWorkflow)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "run the diamond")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
	if got := strings.Join(state.order, ","); got != "a,b,c,d" {
		t.Fatalf("execution order = %s, want a,b,c,d", got)
	}
}

func TestGateRoutesTheCleanBranch(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, routedWorkflow(routingGate))
	state := newFakeState()
	engine := New(fakeRegistry(t, state, nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "review the change")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	gate := requireNodeStatus(t, done, "gate", NodeCompleted)
	if gate.Outcome != "clean" {
		t.Fatalf("gate outcome = %q, want clean", gate.Outcome)
	}
	if gate.ExitCode != 0 {
		t.Fatalf("gate exit code = %d", gate.ExitCode)
	}
	requireNodeStatus(t, done, "verify", NodeCompleted)
	requireNodeStatus(t, done, "fix", NodeSkipped)
	requireNodeStatus(t, done, "fix_report", NodeSkipped)

	if state.count("fix") != 0 || state.count("fix_report") != 0 {
		t.Fatalf("the untaken branch executed: %+v", state.order)
	}
	// Command nodes are not harness executions, so the scripted order lists
	// agent nodes only.
	if got := strings.Join(state.order, ","); got != "review,verify" {
		t.Fatalf("execution order = %s", got)
	}

	event := nodeCompletedEvent(t, engine, project, run.ID, "gate")
	if event.Data["route"] != "clean" {
		t.Fatalf("node.completed route = %v", event.Data["route"])
	}
}

func TestGateRoutesTheIssuesBranch(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, routedWorkflow(routingGate))
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"review": {Writes: map[string]string{"findings.txt": "one defect\n"}},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "review the change")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	gate := requireNodeStatus(t, done, "gate", NodeCompleted)
	if gate.Outcome != "issues" || gate.ExitCode != 1 {
		t.Fatalf("gate = outcome %q, exit %d", gate.Outcome, gate.ExitCode)
	}
	requireNodeStatus(t, done, "fix", NodeCompleted)
	requireNodeStatus(t, done, "fix_report", NodeCompleted)
	requireNodeStatus(t, done, "verify", NodeCompleted)

	// The join runs once, after the branch that reached it.
	if state.count("verify") != 1 {
		t.Fatalf("verify ran %d times", state.count("verify"))
	}
	if got := strings.Join(state.order, ","); got != "review,fix,fix_report,verify" {
		t.Fatalf("execution order = %s", got)
	}
}

func TestUndeclaredExitCodeStillFailsTheGate(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, routedWorkflow("exit 2"))
	engine := newTestEngine(t)

	run := startRun(t, engine, project, "break the gate")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunFailed {
		t.Fatalf("status = %s", done.Status)
	}
	gate := requireNodeStatus(t, done, "gate", NodeFailed)
	if gate.ExitCode != 2 {
		t.Fatalf("gate exit code = %d", gate.ExitCode)
	}
	if gate.Outcome != "" {
		t.Fatalf("an undeclared exit code routed: %q", gate.Outcome)
	}
	for _, id := range []string{"fix", "fix_report", "verify"} {
		requireNodeStatus(t, done, id, NodeSkipped)
	}
	if !strings.Contains(done.Error, "exited with 2") {
		t.Fatalf("run error = %q", done.Error)
	}
}

func TestTimedOutGateDoesNotRoute(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: slow-gate
defaults:
  harness: fake
nodes:
  - {id: gate, kind: command, command: "sleep 5", timeout: 100ms, routes: {0: clean, 1: issues}}
  - {id: clean_path, kind: agent, prompt: clean}
  - {id: issues_path, kind: agent, prompt: issues}
edges:
  - {from: gate, to: clean_path, when: clean}
  - {from: gate, to: issues_path, when: issues}
`)
	engine := newTestEngine(t)

	run := startRun(t, engine, project, "time out the gate")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunFailed {
		t.Fatalf("status = %s", done.Status)
	}
	gate := requireNodeStatus(t, done, "gate", NodeFailed)
	if gate.Outcome != "" {
		t.Fatalf("a timed-out gate routed: %q", gate.Outcome)
	}
	if !strings.Contains(done.Error, "timed out") {
		t.Fatalf("run error = %q", done.Error)
	}
}

func TestFailedNodeSkipsTheRemainderOfARoutedRun(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, routedWorkflow(routingGate))
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"review": {Writes: map[string]string{"findings.txt": "one defect\n"}},
		"fix":    {Failures: 1, Retryable: false, FailMessage: "cannot fix"},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "fail while fixing")
	done := waitTerminal(t, engine, project, run.ID)

	if done.Status != RunFailed {
		t.Fatalf("status = %s", done.Status)
	}
	requireNodeStatus(t, done, "gate", NodeCompleted)
	requireNodeStatus(t, done, "fix", NodeFailed)
	requireNodeStatus(t, done, "fix_report", NodeSkipped)
	requireNodeStatus(t, done, "verify", NodeSkipped)
	if !strings.Contains(done.Error, "cannot fix") {
		t.Fatalf("run error = %q", done.Error)
	}
}

func TestRoutingGateReportsRouteInTheRunRecord(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, routedWorkflow(routingGate))
	engine := newTestEngine(t)

	run := startRun(t, engine, project, "record the route")
	done := waitTerminal(t, engine, project, run.ID)

	gate := requireNodeStatus(t, done, "gate", NodeCompleted)
	if gate.Outcome != "clean" {
		t.Fatalf("persisted outcome = %q", gate.Outcome)
	}

	// The record is the only durable memory of the branch, so it must survive
	// a reload from disk.
	opened, err := workspace.Open(context.Background(), project)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	reloaded, err := NewStore(opened).Load(run.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Node("gate").Outcome != "clean" {
		t.Fatalf("reloaded outcome = %q", reloaded.Node("gate").Outcome)
	}
}

func TestResumeKeepsTheUntakenBranchSkipped(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, routedWorkflow(routingGate))
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"verify": {Failures: 1, Retryable: false, FailMessage: "verify broke"},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "resume a clean run")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}
	requireNodeStatus(t, failed, "fix", NodeSkipped)
	requireNodeStatus(t, failed, "fix_report", NodeSkipped)
	requireNodeStatus(t, failed, "verify", NodeFailed)

	if err := engine.Resume(context.Background(), project, run.ID); err != nil {
		t.Fatal(err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	// The completed gate replays its recorded route, so nothing on the untaken
	// branch runs on the resume.
	requireNodeStatus(t, done, "fix", NodeSkipped)
	requireNodeStatus(t, done, "fix_report", NodeSkipped)
	if state.count("fix") != 0 || state.count("fix_report") != 0 {
		t.Fatalf("an off-path node executed: %+v", state.order)
	}
	if state.count("verify") != 2 {
		t.Fatalf("verify ran %d times, want the failed attempt plus the resume", state.count("verify"))
	}
	if state.count("review") != 1 {
		t.Fatalf("completed node review ran %d times", state.count("review"))
	}
}

func TestResumeRerunsANodeSkippedByAFailure(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, routedWorkflow(routingGate))
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
		"review": {Failures: 1, Retryable: false, FailMessage: "review broke"},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "resume after the review")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}
	requireNodeStatus(t, failed, "review", NodeFailed)
	requireNodeStatus(t, failed, "gate", NodeSkipped)
	requireNodeStatus(t, failed, "fix", NodeSkipped)

	if err := engine.Resume(context.Background(), project, run.ID); err != nil {
		t.Fatal(err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	// The gate was skipped because the run failed, not because no edge reached
	// it, so the resume runs it and routes the clean branch.
	gate := requireNodeStatus(t, done, "gate", NodeCompleted)
	if gate.Outcome != "clean" {
		t.Fatalf("gate outcome = %q", gate.Outcome)
	}
	requireNodeStatus(t, done, "verify", NodeCompleted)
	requireNodeStatus(t, done, "fix", NodeSkipped)
	if state.count("review") != 2 {
		t.Fatalf("review ran %d times", state.count("review"))
	}
}

func TestResumeFailsWhenACompletedGateCarriesNoUsableRoute(t *testing.T) {
	tests := []struct {
		name    string
		outcome string
		want    string
	}{
		{
			name:    "no recorded route",
			outcome: "",
			want:    `node "gate" completed without a recorded route`,
		},
		{
			name:    "a route it does not declare",
			outcome: "major",
			want:    `node "gate" completed under route "major", which it does not declare`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := newRepoProject(t)
			writeProjectWorkflow(t, project, routedWorkflow(routingGate))
			state := newFakeState()
			engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{
				"review": {Writes: map[string]string{"findings.txt": "one defect\n"}},
				"fix":    {Failures: 1, Retryable: false, FailMessage: "cannot fix"},
			}, allCapabilities()))
			defer engine.Close()

			run := startRun(t, engine, project, "corrupt the route")
			failed := waitTerminal(t, engine, project, run.ID)
			if failed.Status != RunFailed {
				t.Fatalf("status = %s", failed.Status)
			}
			executionsBefore := len(state.order)

			// A completed routing node must carry a label it actually declares;
			// otherwise the taken path cannot be reconstructed, and the resume
			// refuses rather than guessing a branch.
			corrupted := *failed
			corrupted.Nodes = append([]NodeRun(nil), failed.Nodes...)
			for index := range corrupted.Nodes {
				if corrupted.Nodes[index].NodeID == "gate" {
					corrupted.Nodes[index].Outcome = test.outcome
				}
			}
			writeRunRecord(t, project, &corrupted)

			if err := engine.Resume(context.Background(), project, run.ID); err != nil {
				t.Fatalf("resume should start and then refuse: %v", err)
			}
			refused := waitTerminal(t, engine, project, run.ID)
			if refused.Status != RunFailed {
				t.Fatalf("status = %s, want failed", refused.Status)
			}
			if !strings.Contains(refused.Error, test.want) {
				t.Fatalf("run error = %q, want it to contain %q", refused.Error, test.want)
			}
			if len(state.order) != executionsBefore {
				t.Fatalf("a node executed despite the corrupt record: %+v", state.order)
			}
		})
	}
}
