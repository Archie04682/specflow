package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/orchestrator"
)

// newFolderRunServer serves a project whose workspace mode is folder.
func newFolderRunServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".specflow", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".specflow", "config.yaml"), []byte("vcs: folder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workflow := `id: test
name: Folder test
version: 1
nodes:
  - {id: write, kind: command, command: "echo made > made.txt"}
`
	if err := os.WriteFile(filepath.Join(project, ".specflow", "workflows", "test.yaml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := harness.NewRegistry()
	engine := orchestrator.New(registry)
	t.Cleanup(engine.Close)

	server := httptest.NewServer(NewRouterWithDeps(fstestMapFS(), registry, engine))
	t.Cleanup(server.Close)
	return server, project
}

func TestFolderRunAppliesOverHTTP(t *testing.T) {
	server, project := newFolderRunServer(t)

	response, body := request(t, http.MethodPost, server.URL+"/api/runs", map[string]string{
		"project":   project,
		"workflow":  "test",
		"objective": "write a file",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", response.StatusCode, body)
	}
	var created struct {
		ID    string `json:"id"`
		VCS   string `json:"vcs"`
		Apply string `json:"apply"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if created.VCS != "folder" {
		t.Fatalf("run vcs = %q", created.VCS)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		response, body = request(t, http.MethodGet, projectURL(server.URL, "/api/runs/"+created.ID, project), nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("get: %d %s", response.StatusCode, body)
		}
		var run struct {
			Status string `json:"status"`
			Apply  string `json:"apply"`
		}
		if err := json.Unmarshal(body, &run); err != nil {
			t.Fatal(err)
		}
		if run.Status == "completed" {
			if run.Apply != "pending" {
				t.Fatalf("apply = %q", run.Apply)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run did not complete: %s", body)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := os.Stat(filepath.Join(project, "made.txt")); !os.IsNotExist(err) {
		t.Fatalf("the project changed before apply: %v", err)
	}

	response, body = request(t, http.MethodPost, projectURL(server.URL, "/api/runs/"+created.ID+"/apply", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", response.StatusCode, body)
	}
	var report struct {
		Written []string `json:"written"`
		Deleted []string `json:"deleted"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Written) != 1 || report.Written[0] != "made.txt" {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(project, "made.txt")); err != nil {
		t.Fatalf("apply did not write: %v", err)
	}

	response, body = request(t, http.MethodPost, projectURL(server.URL, "/api/runs/"+created.ID+"/apply", project), nil)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("second apply: %d %s", response.StatusCode, body)
	}
}
