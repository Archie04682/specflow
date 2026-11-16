package orchestrator

import (
	"os"
	"testing"
	"time"

	"example.com/specflow/backend/internal/workspace"
)

func testStore(t *testing.T) (*Store, *workspace.Project) {
	t.Helper()

	project := &workspace.Project{Root: t.TempDir()}
	if err := project.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	return NewStore(project), project
}

func TestStoreSavesAndLoadsARun(t *testing.T) {
	store, project := testStore(t)
	started := time.Now().Truncate(time.Second)

	run := &Run{
		ID:             "run-7",
		Project:        "/tmp/project",
		WorkflowID:     "standard-feature",
		Status:         RunRunning,
		Objective:      "add export",
		Change:         "add-export",
		ChangeBaseline: []string{"old-change"},
		Nodes: []NodeRun{
			{NodeID: "explore", Kind: NodeKindAgent, Status: NodeCompleted, Attempts: 1},
		},
		Checkpoints:  map[string]string{"explore": "abc123"},
		SourceBranch: "main",
		BaseCommit:   "def456",
		Branch:       "specflow/run-run-7",
		Worktree:     "/tmp/project/.specflow/worktrees/run-7",
		StartedAt:    started,
	}
	if err := store.Save(run); err != nil {
		t.Fatalf("save: %v", err)
	}

	// A fresh store is the restart: nothing is carried in memory.
	loaded, err := NewStore(project).Load("run-7")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Status != RunRunning || loaded.Change != "add-export" {
		t.Fatalf("loaded run lost its state: %+v", loaded)
	}
	if loaded.Checkpoints["explore"] != "abc123" {
		t.Fatalf("checkpoint lost: %v", loaded.Checkpoints)
	}
	if loaded.Nodes[0].NodeID != "explore" || loaded.Nodes[0].Status != NodeCompleted {
		t.Fatalf("node result lost: %+v", loaded.Nodes[0])
	}
}

func TestStoreKeepsPreviousRecordWhenTheTemporaryFileIsTruncated(t *testing.T) {
	store, project := testStore(t)

	run := &Run{ID: "run-8", Status: RunRunning, StartedAt: time.Now()}
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}

	temporary := project.RunFile("run-8") + ".tmp"
	if err := os.WriteFile(temporary, []byte(`{"id":"run-8","status":"runn`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load("run-8")
	if err != nil {
		t.Fatalf("previously written record is unreadable: %v", err)
	}
	if loaded.Status != RunRunning {
		t.Fatalf("status = %q", loaded.Status)
	}
}

func TestStoreRoundTripsTheWorkflowSnapshot(t *testing.T) {
	store, project := testStore(t)

	run := &Run{
		ID:               "run-snapshot",
		Status:           RunRunning,
		WorkflowPath:     "/project/workflows/flow.yaml",
		WorkflowSnapshot: project.WorkflowSnapshotFile("run-snapshot"),
		StartedAt:        time.Now(),
	}
	if err := store.Save(run); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := NewStore(project).Load("run-snapshot")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.WorkflowSnapshot != run.WorkflowSnapshot || loaded.WorkflowPath != run.WorkflowPath {
		t.Fatalf("loaded paths = %q, %q", loaded.WorkflowPath, loaded.WorkflowSnapshot)
	}
}

func TestStoreLoadsALegacyRecordWithoutASnapshot(t *testing.T) {
	store, project := testStore(t)

	path := project.RunFile("run-legacy")
	if err := os.MkdirAll(project.RunDir("run-legacy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"id":"run-legacy","status":"running","workflowPath":"/project/workflows/flow.yaml"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load("run-legacy")
	if err != nil {
		t.Fatalf("load legacy record: %v", err)
	}
	if loaded.WorkflowSnapshot != "" {
		t.Fatalf("legacy record gained a snapshot path: %q", loaded.WorkflowSnapshot)
	}
	if loaded.Pending != nil || loaded.Injections != nil {
		t.Fatalf("legacy record gained interactive state: %+v", loaded)
	}
}

func TestStoreListsRunsNewestFirst(t *testing.T) {
	store, _ := testStore(t)

	older := &Run{ID: "run-old", Status: RunCompleted, StartedAt: time.Now().Add(-time.Hour)}
	newer := &Run{ID: "run-new", Status: RunRunning, StartedAt: time.Now()}
	for _, run := range []*Run{older, newer} {
		if err := store.Save(run); err != nil {
			t.Fatal(err)
		}
	}

	runs, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("listed %d runs", len(runs))
	}
	if runs[0].ID != "run-new" {
		t.Fatalf("ordering = %v", []string{runs[0].ID, runs[1].ID})
	}
}

func TestStoreRoundTripsPendingAndInjections(t *testing.T) {
	store, project := testStore(t)
	deadline := time.Now().Add(5 * time.Minute).Truncate(time.Second)

	run := &Run{
		ID:        "run-interactive",
		Status:    RunWaitingAnswer,
		StartedAt: time.Now(),
		Pending: &PendingInteraction{
			Kind: "question", ID: "q1", NodeID: "explore", Attempt: 1,
			Message: "which one?", Options: []string{"a", "b"}, Deadline: &deadline,
		},
		Injections: []InjectionRecord{{ID: "i1", NodeID: "apply", Text: "csv"}},
	}
	if err := store.Save(run); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := NewStore(project).Load("run-interactive")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Pending == nil || loaded.Pending.ID != "q1" || loaded.Pending.Deadline == nil {
		t.Fatalf("pending lost: %+v", loaded.Pending)
	}
	if len(loaded.Injections) != 1 || loaded.Injections[0].Text != "csv" {
		t.Fatalf("injections lost: %+v", loaded.Injections)
	}
}
