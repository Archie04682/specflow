package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/mock"
	"example.com/specflow/backend/internal/orchestrator"
)

// flakyAdapter fails the first execution of node "b" and completes afterwards.
type flakyAdapter struct {
	mu     sync.Mutex
	failed map[string]bool
}

func (a *flakyAdapter) Descriptor() harness.Descriptor {
	return harness.Descriptor{ID: "flaky", Name: "Flaky", Version: "0.1.0", Protocol: harness.ProtocolVersion}
}

func (a *flakyAdapter) Capabilities() harness.Capabilities {
	return mock.New(mock.Script{}).Capabilities()
}

func (a *flakyAdapter) Probe(context.Context) (harness.ProbeResult, error) {
	return harness.ProbeResult{Capabilities: a.Capabilities()}, nil
}

func (a *flakyAdapter) Run(_ context.Context, request harness.Request, _ harness.Host) (harness.Result, error) {
	a.mu.Lock()
	fail := request.NodeID == "b" && !a.failed[request.NodeID]
	if fail {
		a.failed[request.NodeID] = true
	}
	a.mu.Unlock()

	if fail {
		result := harness.Result{
			Status:  harness.StatusFailed,
			Failure: &harness.Failure{Class: harness.FailureHarnessExit, Message: "transient", Retryable: false},
		}
		return result, result.Err()
	}
	return harness.Result{Status: harness.StatusCompleted, Text: "done"}, nil
}

func TestResumeUsesTheRecordedWorkflowOverHTTP(t *testing.T) {
	project := newRepo(t)

	sourcePath := filepath.Join(project, ".specflow", "workflows", "test.yaml")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `
id: flaky
defaults: {harness: flaky}
nodes:
  - {id: a, kind: agent, prompt: first}
  - {id: b, kind: agent, prompt: second}
edges:
  - {from: a, to: b}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := harness.NewRegistry()
	adapter := &flakyAdapter{failed: make(map[string]bool)}
	if err := registry.Register(func() harness.Adapter { return adapter }); err != nil {
		t.Fatal(err)
	}
	engine := orchestrator.New(registry)
	t.Cleanup(engine.Close)

	server := httptest.NewServer(NewRouterWithDeps(fstestMapFS(), registry, engine))
	t.Cleanup(server.Close)

	response, body := request(t, http.MethodPost, server.URL+"/api/runs", map[string]string{
		"project":   project,
		"workflow":  "test",
		"objective": "resume from the recorded workflow",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", response.StatusCode, body)
	}
	var created orchestrator.Run
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}

	failed := waitCompleted(t, server, project, created.ID)
	if failed.Status != orchestrator.RunFailed {
		t.Fatalf("first attempt ended %s: %s", failed.Status, failed.Error)
	}
	if record := failed.Node("b"); record == nil || record.Status != orchestrator.NodeFailed {
		t.Fatalf("node b = %+v", record)
	}

	// Make the live source uncompilable. Resume must use the retained copy.
	invalid := "this is not: [valid\n"
	if err := os.WriteFile(sourcePath, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}

	response, body = request(t, http.MethodPost, runURL(server, project, created.ID, "/resume"), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("resume: %d %s", response.StatusCode, body)
	}

	done := waitCompleted(t, server, project, created.ID)
	if done.Status != orchestrator.RunCompleted {
		t.Fatalf("resume ended %s: %s", done.Status, done.Error)
	}
	if record := done.Node("b"); record == nil || record.Status != orchestrator.NodeCompleted {
		t.Fatalf("node b after resume = %+v", record)
	}

	wantSnapshot := filepath.Join(project, ".specflow", "runs", done.ID, "workflow.yaml")
	if done.WorkflowSnapshot != wantSnapshot {
		t.Fatalf("snapshot path = %q, want %q", done.WorkflowSnapshot, wantSnapshot)
	}
	if data, err := os.ReadFile(wantSnapshot); err != nil {
		t.Fatalf("read snapshot: %v", err)
	} else if string(data) != source {
		t.Fatalf("snapshot does not hold the compiled source:\n%s", data)
	}

	if data, err := os.ReadFile(sourcePath); err != nil {
		t.Fatal(err)
	} else if string(data) != invalid {
		t.Fatalf("the live source was rewritten: %q", data)
	}

	record, err := os.ReadFile(filepath.Join(project, ".specflow", "runs", done.ID, "run.json"))
	if err != nil {
		t.Fatalf("run record: %v", err)
	}
	for _, fragment := range []string{`"workflowPath"`, `"workflowSnapshot"`, `"status": "completed"`} {
		if !strings.Contains(string(record), fragment) {
			t.Fatalf("run record lacks %s:\n%s", fragment, record)
		}
	}
}
