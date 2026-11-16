package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"example.com/specflow/backend/internal/orchestrator"
)

func projectURL(serverURL, path, project string) string {
	return serverURL + path + "?" + url.Values{"project": {project}}.Encode()
}

func TestWorkflowCatalogEndpoint(t *testing.T) {
	server, project := newRunServer(t)

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.StatusCode, body)
	}

	var payload struct {
		Workflows []orchestrator.WorkflowSummary `json:"workflows"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(payload.Workflows) != 1 {
		t.Fatalf("workflows = %+v, want one entry", payload.Workflows)
	}

	entry := payload.Workflows[0]
	if entry.Reference != "test" {
		t.Fatalf("reference = %q, want test", entry.Reference)
	}
	wantPath := filepath.Join(project, ".specflow", "workflows", "test.yaml")
	if entry.Path != wantPath {
		t.Fatalf("path = %q, want %q", entry.Path, wantPath)
	}
	if entry.Name != "test" {
		t.Fatalf("name = %q, want the reference fallback", entry.Name)
	}
	if !entry.Valid || len(entry.Problems) != 0 {
		t.Fatalf("valid = %v, problems = %v", entry.Valid, entry.Problems)
	}
	if len(entry.Nodes) != 1 || entry.Nodes[0].ID != "a" || entry.Nodes[0].Kind != orchestrator.NodeKindAgent {
		t.Fatalf("nodes = %+v", entry.Nodes)
	}
	if entry.Edges == nil || len(entry.Edges) != 0 {
		t.Fatalf("edges = %+v, want an empty array", entry.Edges)
	}
}

func TestWorkflowCatalogEmptyProjectReturnsAnArray(t *testing.T) {
	server, _ := newRunServer(t)
	// A different, workflow-free repository proves an empty catalog is not an
	// error and not JSON null.
	project := newRepo(t)

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.StatusCode, body)
	}

	var payload struct {
		Workflows []orchestrator.WorkflowSummary `json:"workflows"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Workflows == nil {
		t.Fatalf("workflows is null: %s", body)
	}
	if len(payload.Workflows) != 0 {
		t.Fatalf("workflows = %+v, want empty", payload.Workflows)
	}
}

func TestWorkflowCatalogErrors(t *testing.T) {
	server, _ := newRunServer(t)

	tests := []struct {
		name   string
		target string
	}{
		{name: "missing project", target: server.URL + "/api/workflows"},
		{name: "non-git project", target: projectURL(server.URL, "/api/workflows", t.TempDir())},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, body := request(t, http.MethodGet, test.target, nil)
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
			}
			var payload map[string]string
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if payload["error"] == "" {
				t.Fatalf("body has no error field: %s", body)
			}
		})
	}
}

func TestRunGraphEndpoint(t *testing.T) {
	server, project := newRunServer(t)
	writeFile(t, filepath.Join(project, ".specflow", "workflows", "graph.yaml"), `
id: graph
name: Graph
defaults:
  harness: mock
  interactive: true
  allow_inject: true
nodes:
  - {id: later, title: Later, kind: agent, prompt: after}
  - {id: earlier, title: Earlier, kind: agent, prompt: before}
edges:
  - {from: earlier, to: later}
`)

	response, body := request(t, http.MethodPost, server.URL+"/api/runs", map[string]string{
		"project":   project,
		"workflow":  "graph",
		"objective": "draw the graph",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", response.StatusCode, body)
	}
	var run orchestrator.Run
	if err := json.Unmarshal(body, &run); err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, server, project, run.ID)

	response, body = request(t, http.MethodGet, runURL(server, project, run.ID, "/graph"), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("graph: %d %s", response.StatusCode, body)
	}

	var graph orchestrator.RunGraph
	if err := json.Unmarshal(body, &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if graph.RunID != run.ID {
		t.Fatalf("runId = %q, want %q", graph.RunID, run.ID)
	}
	if graph.WorkflowID != "graph" || graph.Name != "Graph" {
		t.Fatalf("workflow identity = %q %q", graph.WorkflowID, graph.Name)
	}

	// Nodes keep declaration order, which here differs from topological order.
	wantNodes := []orchestrator.GraphNode{
		{ID: "later", Title: "Later", Kind: orchestrator.NodeKindAgent, Interactive: true, AllowInject: true},
		{ID: "earlier", Title: "Earlier", Kind: orchestrator.NodeKindAgent, Interactive: true, AllowInject: true},
	}
	if len(graph.Nodes) != len(wantNodes) {
		t.Fatalf("nodes = %+v, want %+v", graph.Nodes, wantNodes)
	}
	for index := range wantNodes {
		if graph.Nodes[index] != wantNodes[index] {
			t.Fatalf("nodes[%d] = %+v, want %+v", index, graph.Nodes[index], wantNodes[index])
		}
	}
	if len(graph.Edges) != 1 || graph.Edges[0].From != "earlier" || graph.Edges[0].To != "later" {
		t.Fatalf("edges = %+v, want earlier->later", graph.Edges)
	}
}

func TestRunGraphErrors(t *testing.T) {
	server, project := newRunServer(t)

	t.Run("missing project", func(t *testing.T) {
		response, body := request(t, http.MethodGet, server.URL+"/api/runs/whatever/graph", nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
		}
	})

	t.Run("unknown run", func(t *testing.T) {
		response, body := request(t, http.MethodGet, runURL(server, project, "missing", "/graph"), nil)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", response.StatusCode, body)
		}
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["error"] == "" {
			t.Fatalf("body has no error field: %s", body)
		}
	})

	t.Run("missing definition", func(t *testing.T) {
		run := createRun(t, server, project)
		waitCompleted(t, server, project, run.ID)
		if err := os.Remove(run.WorkflowSnapshot); err != nil {
			t.Fatal(err)
		}

		response, body := request(t, http.MethodGet, runURL(server, project, run.ID, "/graph"), nil)
		if response.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", response.StatusCode, body)
		}
	})

	t.Run("unparseable definition", func(t *testing.T) {
		run := createRun(t, server, project)
		waitCompleted(t, server, project, run.ID)
		writeFile(t, run.WorkflowSnapshot, "nodes: [\n  - {\n")

		response, body := request(t, http.MethodGet, runURL(server, project, run.ID, "/graph"), nil)
		if response.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", response.StatusCode, body)
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
