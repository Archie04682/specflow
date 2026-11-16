package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/mock"
	"example.com/specflow/backend/internal/orchestrator"
)

func fstestMapFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><div id=\"root\"></div>")},
	}
}

func newRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	git := func(args ...string) {
		command := exec.Command("git", args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "initial")

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func newRunServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	project := newRepo(t)
	workflowPath := filepath.Join(project, ".specflow", "workflows", "test.yaml")
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		t.Fatal(err)
	}
	workflow := `
id: demo
defaults: {harness: mock}
nodes:
  - {id: a, kind: agent, prompt: work}
`
	if err := os.WriteFile(workflowPath, []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter { return mock.New(mock.Script{}) }); err != nil {
		t.Fatal(err)
	}

	engine := orchestrator.New(registry)
	t.Cleanup(engine.Close)

	static := fstestMapFS()
	server := httptest.NewServer(NewRouterWithDeps(static, registry, engine))
	t.Cleanup(server.Close)

	return server, project
}

func request(t *testing.T, method, target string, body any) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, data
}

func runURL(server *httptest.Server, project, runID, suffix string) string {
	query := url.Values{"project": {project}}.Encode()
	return server.URL + "/api/runs/" + runID + suffix + "?" + query
}

func createRun(t *testing.T, server *httptest.Server, project string) orchestrator.Run {
	t.Helper()

	response, body := request(t, http.MethodPost, server.URL+"/api/runs", map[string]string{
		"project":   project,
		"workflow":  "test",
		"objective": "do it",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", response.StatusCode, body)
	}

	var run orchestrator.Run
	if err := json.Unmarshal(body, &run); err != nil {
		t.Fatal(err)
	}
	if run.ID == "" {
		t.Fatal("created run has no id")
	}
	return run
}

func waitCompleted(t *testing.T, server *httptest.Server, project, runID string) orchestrator.Run {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, body := request(t, http.MethodGet, runURL(server, project, runID, ""), nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("get: %d %s", response.StatusCode, body)
		}
		var run orchestrator.Run
		if err := json.Unmarshal(body, &run); err != nil {
			t.Fatal(err)
		}
		if run.Status.Terminal() {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("run did not finish")
	return orchestrator.Run{}
}

func TestRunLifecycleOverHTTP(t *testing.T) {
	server, project := newRunServer(t)

	created := createRun(t, server, project)
	done := waitCompleted(t, server, project, created.ID)
	if done.Status != orchestrator.RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	response, body := request(t, http.MethodGet, server.URL+"/api/runs?"+url.Values{"project": {project}}.Encode(), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", response.StatusCode, body)
	}
	var runs []orchestrator.Run
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != created.ID {
		t.Fatalf("list = %+v", runs)
	}

	response, body = request(t, http.MethodGet, runURL(server, project, created.ID, ""), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("get: %d", response.StatusCode)
	}
	var loaded orchestrator.Run
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Node("a") == nil || loaded.Node("a").Status != orchestrator.NodeCompleted {
		t.Fatalf("node = %+v", loaded.Nodes)
	}
}

func TestUnknownRunIsNotFound(t *testing.T) {
	server, project := newRunServer(t)

	response, _ := request(t, http.MethodGet, runURL(server, project, "missing", ""), nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestEventsStreamReplaysAndResumes(t *testing.T) {
	server, project := newRunServer(t)
	created := createRun(t, server, project)
	waitCompleted(t, server, project, created.ID)

	response, body := request(t, http.MethodGet, runURL(server, project, created.ID, "/events"), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("events: %d %s", response.StatusCode, body)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("content type = %q", contentType)
	}
	if !strings.Contains(string(body), "event: run.started") {
		t.Fatalf("stream lacks the first event:\n%s", body)
	}
	if !strings.Contains(string(body), "event: run.completed") {
		t.Fatalf("stream lacks the terminal event:\n%s", body)
	}

	// Resume after the first event: the replay must not repeat it.
	req, err := http.NewRequest(http.MethodGet, runURL(server, project, created.ID, "/events"), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Last-Event-ID", "1")
	resumed, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resumedBody, _ := io.ReadAll(resumed.Body)
	resumed.Body.Close()
	if strings.Contains(string(resumedBody), "id: 1\n") {
		t.Fatalf("replay repeated an event at or before the cursor:\n%s", resumedBody)
	}
	if !strings.Contains(string(resumedBody), "event: run.completed") {
		t.Fatalf("replay lost later events:\n%s", resumedBody)
	}
}

func TestRunEndpointErrors(t *testing.T) {
	server, project := newRunServer(t)
	created := createRun(t, server, project)
	waitCompleted(t, server, project, created.ID)

	tests := []struct {
		name   string
		method string
		target string
		body   any
		want   int
	}{
		{
			name:   "create without a project",
			method: http.MethodPost,
			target: server.URL + "/api/runs",
			body:   map[string]string{"workflow": "test"},
			want:   http.StatusBadRequest,
		},
		{
			name:   "answer without a question id",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/answers"),
			body:   map[string]string{"value": "yes"},
			want:   http.StatusBadRequest,
		},
		{
			name:   "inject without text",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/injections"),
			body:   map[string]string{"text": "   "},
			want:   http.StatusBadRequest,
		},
		{
			name:   "inject oversized text",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/injections"),
			body:   map[string]string{"text": strings.Repeat("x", 64*1024+1)},
			want:   http.StatusBadRequest,
		},
		{
			name:   "approve without a node id",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/approve"),
			body:   map[string]any{"decision": "approve"},
			want:   http.StatusBadRequest,
		},
		{
			name:   "approve without a decision",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/approve"),
			body:   map[string]any{"nodeId": "a"},
			want:   http.StatusBadRequest,
		},
		{
			name:   "approve with an unknown decision",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/approve"),
			body:   map[string]any{"nodeId": "a", "decision": "maybe"},
			want:   http.StatusBadRequest,
		},
		{
			name:   "answer an unknown run",
			method: http.MethodPost,
			target: runURL(server, project, "missing", "/answers"),
			body:   map[string]string{"questionId": "q1", "value": "yes"},
			want:   http.StatusNotFound,
		},
		{
			name:   "list without a project",
			method: http.MethodGet,
			target: server.URL + "/api/runs",
			want:   http.StatusBadRequest,
		},
		{
			name:   "answer with no question outstanding",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/answers"),
			body:   map[string]string{"questionId": "q1", "value": "yes"},
			want:   http.StatusGone,
		},
		{
			name:   "injection with no running node",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/injections"),
			body:   map[string]string{"text": "hello"},
			want:   http.StatusConflict,
		},
		{
			name:   "cancel a finished run",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/cancel"),
			want:   http.StatusConflict,
		},
		{
			name:   "approve a run that is not waiting",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/approve"),
			body:   map[string]any{"nodeId": "a", "approved": true},
			want:   http.StatusGone,
		},
		{
			name:   "retry a completed run",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/retry"),
			body:   map[string]string{"nodeId": "a", "mode": "forward"},
			want:   http.StatusConflict,
		},
		{
			name:   "resume a completed run",
			method: http.MethodPost,
			target: runURL(server, project, created.ID, "/resume"),
			want:   http.StatusConflict,
		},
		{
			name:   "discard",
			method: http.MethodDelete,
			target: runURL(server, project, created.ID, ""),
			want:   http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, body := request(t, test.method, test.target, test.body)
			if response.StatusCode != test.want {
				t.Fatalf("status = %d, want %d: %s", response.StatusCode, test.want, body)
			}
		})
	}
}
