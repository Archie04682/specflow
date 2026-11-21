package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const graphWorkflow = `
id: pinned
name: Pinned
version: 1
defaults:
  harness: fake
  interactive: true
  allow_inject: true
nodes:
  - {id: first, title: First, kind: agent, prompt: one}
  - {id: second, title: Second, kind: agent, prompt: two, interactive: false, require_approval: true}
edges:
  - {from: first, to: second}
`

const editedWorkflow = `
id: edited
name: Edited
defaults: {harness: fake}
nodes:
  - {id: rewritten, title: Rewritten, kind: agent, prompt: changed}
`

func createGraphRun(t *testing.T, engine *Engine, project string) *Run {
	t.Helper()

	run, err := engine.Create(context.Background(), CreateRequest{
		Project:   project,
		Workflow:  "test",
		Objective: "read the graph",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

func writeRunRecord(t *testing.T, projectRoot string, run *Run) {
	t.Helper()

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(projectRoot, ".specflow", "runs", run.ID, "run.json"), string(data))
}

func TestReadRunGraphFollowsThePinnedSnapshot(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, graphWorkflow)

	engine := newTestEngine(t)
	run := createGraphRun(t, engine, project)

	// The project file changes after creation; the retained copy must win.
	writeProjectWorkflow(t, project, editedWorkflow)

	graph, err := ReadRunGraph(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("read graph: %v", err)
	}

	if graph.RunID != run.ID {
		t.Fatalf("runId = %q, want %q", graph.RunID, run.ID)
	}
	if graph.WorkflowID != "pinned" {
		t.Fatalf("workflowId = %q, want pinned", graph.WorkflowID)
	}
	if graph.Name != "Pinned" {
		t.Fatalf("name = %q, want Pinned", graph.Name)
	}

	wantNodes := []GraphNode{
		{ID: "first", Title: "First", Kind: NodeKindAgent, Interactive: true, AllowInject: true},
		{ID: "second", Title: "Second", Kind: NodeKindAgent, Interactive: false, AllowInject: true, RequireApproval: true},
	}
	if !reflect.DeepEqual(graph.Nodes, wantNodes) {
		t.Fatalf("nodes = %+v, want %+v", graph.Nodes, wantNodes)
	}
	if want := []GraphEdge{{From: "first", To: "second"}}; !reflect.DeepEqual(graph.Edges, want) {
		t.Fatalf("edges = %+v, want %+v", graph.Edges, want)
	}
}

func TestReadRunGraphFallsBackToTheRecordedPath(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, graphWorkflow)

	engine := newTestEngine(t)
	run := createGraphRun(t, engine, project)

	// Make the record look legacy: it has a path but no retained snapshot.
	legacy := *run
	legacy.WorkflowSnapshot = ""
	writeRunRecord(t, project, &legacy)
	writeProjectWorkflow(t, project, editedWorkflow)

	graph, err := ReadRunGraph(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("read graph: %v", err)
	}
	if want := []GraphNode{{ID: "rewritten", Title: "Rewritten", Kind: NodeKindAgent}}; !reflect.DeepEqual(graph.Nodes, want) {
		t.Fatalf("nodes = %+v, want the recorded path's %+v", graph.Nodes, want)
	}
	if graph.Name != "Edited" {
		t.Fatalf("name = %q, want Edited", graph.Name)
	}
	// The run record still names the workflow it was created with.
	if graph.WorkflowID != "pinned" {
		t.Fatalf("workflowId = %q, want the recorded pinned", graph.WorkflowID)
	}

	// Without a recorded id the parsed definition supplies it.
	legacy.WorkflowID = ""
	writeRunRecord(t, project, &legacy)
	graph, err = ReadRunGraph(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("read graph: %v", err)
	}
	if graph.WorkflowID != "edited" {
		t.Fatalf("workflowId = %q, want the parsed id", graph.WorkflowID)
	}
}

func TestReadRunGraphUnknownRun(t *testing.T) {
	project := newRepoProject(t)

	_, err := ReadRunGraph(context.Background(), project, "missing")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("error = %v, want ErrRunNotFound", err)
	}
}

func TestReadRunGraphMissingDefinition(t *testing.T) {
	project := newRepoProject(t)
	writeRunRecord(t, project, &Run{
		ID:           "run-missing",
		WorkflowPath: filepath.Join(project, "workflows", "gone.yaml"),
		Status:       RunPending,
	})

	_, err := ReadRunGraph(context.Background(), project, "run-missing")
	if err == nil {
		t.Fatal("a missing recorded definition was accepted")
	}
	if errors.Is(err, ErrRunNotFound) {
		t.Fatalf("a missing definition was reported as an unknown run: %v", err)
	}
}

func TestReadRunGraphUnparseableDefinition(t *testing.T) {
	project := newRepoProject(t)
	source := filepath.Join(project, ".specflow", "runs", "run-broken", "workflow.yaml")
	writeTestFile(t, source, "nodes: [\n  - {\n")
	writeRunRecord(t, project, &Run{
		ID:               "run-broken",
		WorkflowSnapshot: source,
		Status:           RunPending,
	})

	if _, err := ReadRunGraph(context.Background(), project, "run-broken"); err == nil {
		t.Fatal("an unparseable recorded definition was accepted")
	}
}

func TestReadRunGraphSurvivesARemovedSkill(t *testing.T) {
	project := newRepoProject(t)
	// ResolveSkill only requires the directory to exist, and an empty
	// directory does not make the project dirty.
	skillDir := filepath.Join(project, ".dsh", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectWorkflow(t, project, `
id: skilled
defaults: {harness: fake}
nodes:
  - {id: work, kind: agent, skill: demo}
`)

	engine := newTestEngine(t)
	run := createGraphRun(t, engine, project)

	// Compilation is no longer possible once the skill is gone; the graph
	// must still be readable because it never resolves skills.
	if err := os.RemoveAll(skillDir); err != nil {
		t.Fatal(err)
	}

	graph, err := ReadRunGraph(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("a removed skill blocked the graph: %v", err)
	}
	if len(graph.Nodes) != 1 || graph.Nodes[0].ID != "work" {
		t.Fatalf("nodes = %+v, want the bound node", graph.Nodes)
	}
}

func TestReadRunGraphChangesNothing(t *testing.T) {
	project := newRepoProject(t)
	// A plain linear workflow so the run reaches a terminal state without
	// waiting on an approval gate.
	writeProjectWorkflow(t, project, linearWorkflow)

	engine := newTestEngine(t)
	run := startRun(t, engine, project, "run once")
	waitTerminal(t, engine, project, run.ID)

	runFile := filepath.Join(project, ".specflow", "runs", run.ID, "run.json")
	eventsFile := filepath.Join(project, ".specflow", "runs", run.ID, "events.jsonl")
	runBefore, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, err := os.ReadFile(eventsFile)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ReadRunGraph(context.Background(), project, run.ID); err != nil {
		t.Fatalf("read graph: %v", err)
	}

	runAfter, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatal(err)
	}
	eventsAfter, err := os.ReadFile(eventsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(runBefore, runAfter) {
		t.Fatal("reading the graph rewrote run.json")
	}
	if !bytes.Equal(eventsBefore, eventsAfter) {
		t.Fatal("reading the graph appended an event")
	}
}

func TestReadRunGraphOfAPendingRunWritesNothing(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, graphWorkflow)

	engine := newTestEngine(t)
	run := createGraphRun(t, engine, project)

	runFile := filepath.Join(project, ".specflow", "runs", run.ID, "run.json")
	before, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ReadRunGraph(context.Background(), project, run.ID); err != nil {
		t.Fatalf("read graph: %v", err)
	}

	after, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("reading a pending run's graph rewrote run.json")
	}
	if _, err := os.Stat(filepath.Join(project, ".specflow", "runs", run.ID, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("reading a pending run's graph produced events: %v", err)
	}
}

const routingGraphWorkflow = `
id: routed
name: Routed
defaults: {harness: fake}
nodes:
  - {id: gate, kind: command, command: "test ! -s findings.md", routes: {0: clean, 1: issues}}
  - {id: fix, kind: agent, prompt: fix it}
  - {id: verify, kind: command, command: "true"}
edges:
  - {from: gate, to: fix, when: issues}
  - {from: gate, to: verify, when: clean}
`

func TestReadRunGraphCarriesEdgeConditions(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, routingGraphWorkflow)

	engine := newTestEngine(t)
	run := createGraphRun(t, engine, project)

	graph, err := ReadRunGraph(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("read graph: %v", err)
	}

	want := []GraphEdge{
		{From: "gate", To: "fix", When: "issues"},
		{From: "gate", To: "verify", When: "clean"},
	}
	if !reflect.DeepEqual(graph.Edges, want) {
		t.Fatalf("edges = %+v, want %+v", graph.Edges, want)
	}
}
