package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"example.com/specflow/backend/internal/orchestrator"
)

func apiSourceDraft() orchestrator.WorkflowDraft {
	return orchestrator.WorkflowDraft{
		ID:      "editor",
		Name:    "Editor",
		Version: 1,
		Defaults: orchestrator.WorkflowDefaultsDraft{
			Harness: "mock",
		},
		Nodes: []orchestrator.WorkflowNodeDraft{
			{ID: "only", Title: "Only", Kind: orchestrator.NodeKindAgent, Prompt: "work"},
		},
		Edges: []orchestrator.WorkflowEdgeDraft{},
	}
}

func apiHashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func decodeWorkflowSource(t *testing.T, body []byte) orchestrator.WorkflowSource {
	t.Helper()

	var source orchestrator.WorkflowSource
	if err := json.Unmarshal(body, &source); err != nil {
		t.Fatalf("decode workflow source: %v (%s)", err, body)
	}
	return source
}

func saveBody(draft orchestrator.WorkflowDraft, baseHash string) map[string]any {
	return map[string]any{"draft": draft, "baseHash": baseHash}
}

func saveWorkflowRequest(t *testing.T, serverURL, project, reference string, draft orchestrator.WorkflowDraft, baseHash string) (*http.Response, []byte) {
	t.Helper()
	return request(t, http.MethodPut, projectURL(serverURL, "/api/workflows/"+reference, project), saveBody(draft, baseHash))
}

// rawRequest sends a body verbatim so malformed JSON can be tested.
func rawRequest(t *testing.T, method, target, body string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, target, strings.NewReader(body))
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

func TestWorkflowSourceEndpoint(t *testing.T) {
	server, project := newRunServer(t)
	workflowPath := filepath.Join(project, ".specflow", "workflows", "test.yaml")
	sourceBytes, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/test", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.StatusCode, body)
	}

	source := decodeWorkflowSource(t, body)
	if source.Reference != "test" {
		t.Fatalf("reference = %q", source.Reference)
	}
	if source.Path != workflowPath {
		t.Fatalf("path = %q, want %q", source.Path, workflowPath)
	}
	if source.Source != string(sourceBytes) {
		t.Fatalf("source = %q, want the file bytes", source.Source)
	}
	if source.Hash != apiHashBytes(sourceBytes) {
		t.Fatalf("hash = %q, want %q", source.Hash, apiHashBytes(sourceBytes))
	}
	if source.IsTemplate {
		t.Fatal("the project copy was reported as a template")
	}
	if source.Draft.ID != "demo" || len(source.Draft.Nodes) != 1 || source.Draft.Nodes[0].ID != "a" {
		t.Fatalf("draft = %+v", source.Draft)
	}
	if !strings.Contains(string(body), `"edges":[]`) {
		t.Fatalf("draft edges are not an empty array: %s", body)
	}
}

func TestWorkflowSourceEndpointForATemplate(t *testing.T) {
	server, _ := newRunServer(t)
	project := newRepo(t)
	writeFile(t, filepath.Join(project, "workflows", "template.yaml"), `
id: template
name: Template
defaults: {harness: mock}
nodes:
  - {id: only, kind: agent}
`)

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/template", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.StatusCode, body)
	}

	source := decodeWorkflowSource(t, body)
	if !source.IsTemplate {
		t.Fatal("a workflows/ file was not reported as a template")
	}
	if want := filepath.Join(project, "workflows", "template.yaml"); source.Path != want {
		t.Fatalf("path = %q, want %q", source.Path, want)
	}
}

func TestWorkflowSourceEndpointErrors(t *testing.T) {
	server, project := newRunServer(t)

	t.Run("missing project", func(t *testing.T) {
		response, body := request(t, http.MethodGet, server.URL+"/api/workflows/test", nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
		}
	})

	t.Run("invalid reference", func(t *testing.T) {
		response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/.hidden", project), nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
		}
	})

	t.Run("unknown workflow", func(t *testing.T) {
		response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/missing", project), nil)
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

	t.Run("missing project", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/test", missing), nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
		}
	})
}

func TestWorkflowSourceEndpointReportsAParseFailure(t *testing.T) {
	server, _ := newRunServer(t)
	project := newRepo(t)
	writeFile(t, filepath.Join(project, ".specflow", "workflows", "broken.yaml"), "nodes: [\n  - {\n")

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/broken", project), nil)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
	}
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(payload["error"], "parse workflow") {
		t.Fatalf("error = %q, want the parse error", payload["error"])
	}
}

func TestWorkflowSourceSaveCreatesAndReadsBack(t *testing.T) {
	server, project := newRunServer(t)
	draft := apiSourceDraft()

	response, body := saveWorkflowRequest(t, server.URL, project, "created", draft, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.StatusCode, body)
	}

	var saved struct {
		Path string `json:"path"`
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	wantPath := filepath.Join(project, ".specflow", "workflows", "created.yaml")
	if saved.Path != wantPath {
		t.Fatalf("path = %q, want %q", saved.Path, wantPath)
	}
	written, err := os.ReadFile(saved.Path)
	if err != nil {
		t.Fatalf("read saved workflow: %v", err)
	}
	if saved.Hash != apiHashBytes(written) {
		t.Fatalf("hash = %q, want %q", saved.Hash, apiHashBytes(written))
	}
	if !strings.HasPrefix(string(written), "# Managed by the SpecFlow workflow editor.") {
		t.Fatalf("saved file has no managed header:\n%s", written)
	}

	// The editor can reopen what it saved.
	response, body = request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/created", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("reopen: %d %s", response.StatusCode, body)
	}
	source := decodeWorkflowSource(t, body)
	if source.Hash != saved.Hash {
		t.Fatalf("reopened hash = %q, want %q", source.Hash, saved.Hash)
	}
	if !reflect.DeepEqual(source.Draft, draft) {
		t.Fatalf("reopened draft:\n got: %+v\nwant: %+v", source.Draft, draft)
	}
}

func TestWorkflowSourceSaveUpdatesAnExistingWorkflow(t *testing.T) {
	server, project := newRunServer(t)

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/test", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("open: %d %s", response.StatusCode, body)
	}
	source := decodeWorkflowSource(t, body)

	updated := source.Draft
	updated.Name = "Renamed"
	response, body = saveWorkflowRequest(t, server.URL, project, "test", updated, source.Hash)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("save: %d %s", response.StatusCode, body)
	}
	var saved struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if saved.Hash == source.Hash {
		t.Fatal("the update returned the previous hash")
	}

	response, body = request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/test", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("reopen: %d %s", response.StatusCode, body)
	}
	reopened := decodeWorkflowSource(t, body)
	if reopened.Hash != saved.Hash || reopened.Draft.Name != "Renamed" {
		t.Fatalf("reopened = hash %q name %q", reopened.Hash, reopened.Draft.Name)
	}
}

func TestWorkflowSourceSaveCopiesATemplate(t *testing.T) {
	server, _ := newRunServer(t)
	project := newRepo(t)
	templatePath := filepath.Join(project, "workflows", "template.yaml")
	template := `
id: template
name: Template
defaults: {harness: mock}
nodes:
  - {id: only, kind: agent}
`
	writeFile(t, templatePath, template)

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/template", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("open: %d %s", response.StatusCode, body)
	}
	source := decodeWorkflowSource(t, body)
	if !source.IsTemplate {
		t.Fatal("template was not flagged")
	}

	response, body = saveWorkflowRequest(t, server.URL, project, "template", source.Draft, source.Hash)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("save: %d %s", response.StatusCode, body)
	}
	var saved struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if want := filepath.Join(project, ".specflow", "workflows", "template.yaml"); saved.Path != want {
		t.Fatalf("path = %q, want the project copy %q", saved.Path, want)
	}
	if written, err := os.ReadFile(templatePath); err != nil || string(written) != template {
		t.Fatalf("template was modified: %v\n%s", err, written)
	}
}

func TestWorkflowSourceSaveConflicts(t *testing.T) {
	server, project := newRunServer(t)

	t.Run("stale hash", func(t *testing.T) {
		response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/test", project), nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("open: %d %s", response.StatusCode, body)
		}
		stale := decodeWorkflowSource(t, body)

		// The file changes between the open and the save.
		changed := []byte("id: changed\nname: Changed\n")
		writeFile(t, stale.Path, string(changed))

		response, body = saveWorkflowRequest(t, server.URL, project, "test", stale.Draft, stale.Hash)
		if response.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409: %s", response.StatusCode, body)
		}
		var payload struct {
			Error string `json:"error"`
			Hash  string `json:"hash"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode conflict: %v", err)
		}
		if payload.Error != "workflow changed on disk" {
			t.Fatalf("error = %q", payload.Error)
		}
		if want := apiHashBytes(changed); payload.Hash != want {
			t.Fatalf("hash = %q, want %q", payload.Hash, want)
		}
		if after, err := os.ReadFile(stale.Path); err != nil || string(after) != string(changed) {
			t.Fatalf("a refused save changed the file: %v\n%s", err, after)
		}
	})

	t.Run("empty base hash over a file", func(t *testing.T) {
		response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/test", project), nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("open: %d %s", response.StatusCode, body)
		}
		current := decodeWorkflowSource(t, body)

		response, body = saveWorkflowRequest(t, server.URL, project, "test", current.Draft, "")
		if response.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409: %s", response.StatusCode, body)
		}
		var payload struct {
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode conflict: %v", err)
		}
		if payload.Hash != current.Hash {
			t.Fatalf("hash = %q, want the current %q", payload.Hash, current.Hash)
		}
	})
}

func TestWorkflowSourceSaveRejectsInvalidDrafts(t *testing.T) {
	server, project := newRunServer(t)

	tests := []struct {
		name  string
		draft func() orchestrator.WorkflowDraft
	}{
		{
			name: "no nodes",
			draft: func() orchestrator.WorkflowDraft {
				draft := apiSourceDraft()
				draft.Nodes = nil
				return draft
			},
		},
		{
			name: "cycle",
			draft: func() orchestrator.WorkflowDraft {
				draft := apiSourceDraft()
				draft.Nodes = []orchestrator.WorkflowNodeDraft{
					{ID: "a", Kind: orchestrator.NodeKindAgent},
					{ID: "b", Kind: orchestrator.NodeKindAgent},
				}
				draft.Edges = []orchestrator.WorkflowEdgeDraft{{From: "a", To: "b"}, {From: "b", To: "a"}}
				return draft
			},
		},
		{
			name: "agent without a harness",
			draft: func() orchestrator.WorkflowDraft {
				draft := apiSourceDraft()
				draft.Defaults.Harness = ""
				return draft
			},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reference := "invalid-" + string(rune('a'+index))
			response, body := saveWorkflowRequest(t, server.URL, project, reference, test.draft(), "")
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
			}
			var payload struct {
				Error    string   `json:"error"`
				Problems []string `json:"problems"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode validation response: %v", err)
			}
			if payload.Error == "" {
				t.Fatalf("body has no error field: %s", body)
			}
			if len(payload.Problems) == 0 {
				t.Fatalf("body has no problems: %s", body)
			}
			if _, err := os.Stat(filepath.Join(project, ".specflow", "workflows", reference+".yaml")); !os.IsNotExist(err) {
				t.Fatalf("an invalid draft was written: %v", err)
			}
		})
	}
}

func TestWorkflowSourceSaveRejectsMalformedRequests(t *testing.T) {
	server, project := newRunServer(t)

	tests := []struct {
		name string
		body string
	}{
		{name: "not JSON", body: "{not json"},
		{name: "empty body", body: ""},
		{name: "draft is not an object", body: `{"draft":"nope","baseHash":""}`},
		{name: "base hash is not a string", body: `{"draft":{},"baseHash":7}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, body := rawRequest(t, http.MethodPut, projectURL(server.URL, "/api/workflows/malformed", project), test.body)
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
			}
		})
	}
}

func TestWorkflowSourceSaveErrors(t *testing.T) {
	server, project := newRunServer(t)

	t.Run("missing project", func(t *testing.T) {
		response, body := request(t, http.MethodPut, server.URL+"/api/workflows/anything", saveBody(apiSourceDraft(), ""))
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
		}
	})

	t.Run("invalid reference", func(t *testing.T) {
		response, body := saveWorkflowRequest(t, server.URL, project, ".hidden", apiSourceDraft(), "")
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
		}
	})

	t.Run("unusable project", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		response, body := saveWorkflowRequest(t, server.URL, missing, "anything", apiSourceDraft(), "")
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.StatusCode, body)
		}
	})

	t.Run("unexpected IO", func(t *testing.T) {
		broken := newRepo(t)
		writeFile(t, filepath.Join(broken, ".specflow", "workflows"), "not a directory")

		response, body := saveWorkflowRequest(t, server.URL, broken, "anything", apiSourceDraft(), "")
		if response.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", response.StatusCode, body)
		}
	})
}

func TestWorkflowSourceSaveDoesNotTouchAnExistingRun(t *testing.T) {
	server, project := newRunServer(t)

	run := createRun(t, server, project)
	done := waitCompleted(t, server, project, run.ID)

	runFile := filepath.Join(project, ".specflow", "runs", done.ID, "run.json")
	snapshotFile := filepath.Join(project, ".specflow", "runs", done.ID, "workflow.yaml")
	runBefore, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatalf("read run record: %v", err)
	}
	snapshotBefore, err := os.ReadFile(snapshotFile)
	if err != nil {
		t.Fatalf("read run snapshot: %v", err)
	}

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/test", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("open: %d %s", response.StatusCode, body)
	}
	source := decodeWorkflowSource(t, body)

	edited := source.Draft
	edited.Name = "Edited after the run"
	response, body = saveWorkflowRequest(t, server.URL, project, "test", edited, source.Hash)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("save: %d %s", response.StatusCode, body)
	}

	runAfter, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatalf("read run record after save: %v", err)
	}
	snapshotAfter, err := os.ReadFile(snapshotFile)
	if err != nil {
		t.Fatalf("read run snapshot after save: %v", err)
	}
	if string(runBefore) != string(runAfter) {
		t.Fatal("saving a workflow changed the run record")
	}
	if string(snapshotBefore) != string(snapshotAfter) {
		t.Fatal("saving a workflow changed the run's pinned definition")
	}

	// The run is still readable and terminal.
	response, body = request(t, http.MethodGet, runURL(server, project, done.ID, ""), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("get run: %d %s", response.StatusCode, body)
	}
	var reloaded orchestrator.Run
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if reloaded.Status != orchestrator.RunCompleted {
		t.Fatalf("run status = %s, want completed", reloaded.Status)
	}
}

// Routing crosses the wire as a map with string keys, so the editor's save and
// reopen are tested end to end rather than only through Go values.
func TestWorkflowSourceSaveRoundTripsRouting(t *testing.T) {
	server, project := newRunServer(t)
	draft := orchestrator.WorkflowDraft{
		ID:       "routed",
		Name:     "Routed",
		Version:  1,
		Defaults: orchestrator.WorkflowDefaultsDraft{Harness: "mock"},
		Nodes: []orchestrator.WorkflowNodeDraft{
			{
				ID:      "gate",
				Title:   "Review gate",
				Kind:    orchestrator.NodeKindCommand,
				Command: "test ! -s review.md",
				Routes:  map[int]string{0: "clean", 1: "issues"},
			},
			{ID: "fix", Title: "Fix", Kind: orchestrator.NodeKindAgent, Prompt: "fix"},
			{ID: "verify", Title: "Verify", Kind: orchestrator.NodeKindAgent, Prompt: "verify"},
		},
		Edges: []orchestrator.WorkflowEdgeDraft{
			{From: "gate", To: "fix", When: "issues"},
			{From: "gate", To: "verify", When: "clean"},
		},
	}

	response, body := saveWorkflowRequest(t, server.URL, project, "routed", draft, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("save: %d %s", response.StatusCode, body)
	}

	response, body = request(t, http.MethodGet, projectURL(server.URL, "/api/workflows/routed", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("reopen: %d %s", response.StatusCode, body)
	}
	source := decodeWorkflowSource(t, body)

	gate := source.Draft.Nodes[0]
	if gate.Routes[0] != "clean" || gate.Routes[1] != "issues" {
		t.Fatalf("routes did not survive the wire: %+v", gate.Routes)
	}
	if source.Draft.Edges[0].When != "issues" || source.Draft.Edges[1].When != "clean" {
		t.Fatalf("conditions did not survive the wire: %+v", source.Draft.Edges)
	}
	if !reflect.DeepEqual(source.Draft, draft) {
		t.Fatalf("reopened draft:\n got: %+v\nwant: %+v", source.Draft, draft)
	}

	written, err := os.ReadFile(source.Path)
	if err != nil {
		t.Fatalf("read saved workflow: %v", err)
	}
	for _, fragment := range []string{"routes:", "when: issues", "when: clean"} {
		if !strings.Contains(string(written), fragment) {
			t.Fatalf("saved workflow has no %q:\n%s", fragment, written)
		}
	}
}
