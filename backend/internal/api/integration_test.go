package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/mock"
	"example.com/specflow/backend/internal/orchestrator"
)

// canonicalWorkflowProject writes the real template into a fresh project with
// the harness and the toolchain gates swapped for ones this test can run.
func canonicalWorkflowProject(t *testing.T) string {
	t.Helper()

	project := newRepo(t)

	source, err := os.ReadFile(filepath.Join("..", "..", "..", "workflows", "standard-feature.yaml"))
	if err != nil {
		t.Fatalf("read canonical workflow: %v", err)
	}
	text := string(source)
	text = strings.ReplaceAll(text, "harness: pi", "harness: mock")
	text = strings.ReplaceAll(text, `openspec validate "{{change}}" --strict && make test`, "true")
	text = strings.ReplaceAll(text, `openspec archive "{{change}}" --yes`, "true")

	workflowPath := filepath.Join(project, ".specflow", "workflows", "standard.yaml")
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workflowPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	// Skills live under .specflow so they stay out of the project's git status,
	// exactly as run metadata does.
	config := filepath.Join(project, ".specflow", "config.yaml")
	if err := os.WriteFile(config, []byte("skills:\n  - .specflow/skills\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"openspec-explore", "openspec-propose", "openspec-apply-change", "openspec-archive-change"} {
		directory := filepath.Join(project, ".specflow", "skills", name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return project
}

// canonicalServer starts the API over a project with one mock script.
func canonicalServer(t *testing.T, project string, script mock.Script) *httptest.Server {
	t.Helper()

	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter {
		return mock.New(script)
	}); err != nil {
		t.Fatal(err)
	}
	engine := orchestrator.New(registry)
	t.Cleanup(engine.Close)

	server := httptest.NewServer(NewRouterWithDeps(fstestMapFS(), registry, engine))
	t.Cleanup(server.Close)
	return server
}

// runCanonical creates and starts a run of the canonical template, approving
// every gate, and returns the terminal record.
func runCanonical(t *testing.T, server *httptest.Server, project string) (orchestrator.Run, map[string]bool) {
	t.Helper()

	response, body := request(t, "POST", server.URL+"/api/runs", map[string]string{
		"project":   project,
		"workflow":  "standard",
		"objective": "add an export endpoint",
	})
	if response.StatusCode != 201 {
		t.Fatalf("create: %d %s", response.StatusCode, body)
	}
	var created orchestrator.Run
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}

	approved := make(map[string]bool)
	var done orchestrator.Run
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, body := request(t, "GET", runURL(server, project, created.ID, ""), nil)
		var run orchestrator.Run
		if err := json.Unmarshal(body, &run); err != nil {
			t.Fatal(err)
		}

		if run.Status.Terminal() {
			done = run
			break
		}
		if run.Status == orchestrator.RunWaitingApproval && run.CurrentNode != "" && !approved[run.CurrentNode] {
			approved[run.CurrentNode] = true
			response, body := request(t, "POST", runURL(server, project, created.ID, "/approve"), map[string]any{
				"nodeId":   run.CurrentNode,
				"approved": true,
			})
			if response.StatusCode != 200 {
				t.Fatalf("approve %s: %d %s", run.CurrentNode, response.StatusCode, body)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if done.Status != orchestrator.RunCompleted {
		t.Fatalf("run ended %s: %s", done.Status, done.Error)
	}
	return done, approved
}

func TestCanonicalWorkflowEndToEnd(t *testing.T) {
	project := canonicalWorkflowProject(t)

	server := canonicalServer(t, project, mock.Script{
		WritesOn: "propose",
		Writes: map[string]string{
			"openspec/changes/demo-change/tasks.md": "# Tasks\n\n- [x] 1.1 Done\n- [x] 1.2 Also done\n",
		},
	})

	done, approved := runCanonical(t, server, project)

	if len(approved) != 2 {
		t.Fatalf("expected approvals for propose and merge, got %v", approved)
	}
	if done.Change != "demo-change" {
		t.Fatalf("resolved change = %q", done.Change)
	}

	// The canonical path now routes: with no findings file the gate takes the
	// clean edge and the fix node never runs.
	gate := done.Node("review_gate")
	if gate == nil || gate.Status != orchestrator.NodeCompleted || gate.Outcome != "clean" {
		t.Fatalf("review_gate = %+v", gate)
	}
	if fix := done.Node("fix"); fix == nil || fix.Status != orchestrator.NodeSkipped {
		t.Fatalf("fix = %+v", fix)
	}
	if tests := done.Node("tests"); tests == nil || tests.Status != orchestrator.NodeCompleted {
		t.Fatalf("tests = %+v", tests)
	}

	for _, nodeID := range []string{"explore", "propose", "apply", "tests", "verify", "merge"} {
		record := done.Node(nodeID)
		if record == nil || record.Status != orchestrator.NodeCompleted {
			t.Fatalf("node %s = %+v", nodeID, record)
		}
		if done.Checkpoints[nodeID] == "" {
			t.Fatalf("no checkpoint for %s", nodeID)
		}
	}

	// The merge landed in the project checkout, not in the worktree.
	merged := filepath.Join(project, "openspec", "changes", "demo-change", "tasks.md")
	if _, err := os.Stat(merged); err != nil {
		t.Fatalf("the merged change is not in the project: %v", err)
	}

	gitLog := runGit(t, project, "log", "--oneline")
	if !strings.Contains(gitLog, "Merge demo-change") {
		t.Fatalf("no merge commit in the project history:\n%s", gitLog)
	}
	if branch := strings.TrimSpace(runGit(t, project, "rev-parse", "--abbrev-ref", "HEAD")); branch != "main" {
		t.Fatalf("project is on %q", branch)
	}
	if !strings.Contains(runGit(t, project, "branch", "--list", done.Branch), done.Branch) {
		t.Fatal("the run branch is gone")
	}

	// The worktree is released and the record survived.
	if _, err := os.Stat(done.Worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists: %v", err)
	}
	record, err := os.ReadFile(filepath.Join(project, ".specflow", "runs", done.ID, "run.json"))
	if err != nil {
		t.Fatalf("run record: %v", err)
	}
	if !strings.Contains(string(record), `"status": "completed"`) {
		t.Fatalf("run record:\n%s", record)
	}
	events, err := os.ReadFile(filepath.Join(project, ".specflow", "runs", done.ID, "events.jsonl"))
	if err != nil {
		t.Fatalf("event log: %v", err)
	}
	if !strings.Contains(string(events), `"type":"run.completed"`) {
		t.Fatalf("event log lacks the terminal event")
	}
}

// A run whose worktree carries review findings routes into fix instead.
// The mock script writes on one node, so the findings file is left with the
// change itself; what the gate checks is the file, not who wrote it.
func TestCanonicalWorkflowRoutesToFix(t *testing.T) {
	project := canonicalWorkflowProject(t)

	server := canonicalServer(t, project, mock.Script{
		WritesOn: "propose",
		Writes: map[string]string{
			"openspec/changes/demo-change/tasks.md": "# Tasks\n\n- [x] 1.1 Done\n",
			".specflow/review.md":                   "## Missing test\nCover the new branch.\n",
		},
	})

	done, _ := runCanonical(t, server, project)

	gate := done.Node("review_gate")
	if gate == nil || gate.Status != orchestrator.NodeCompleted || gate.Outcome != "issues" {
		t.Fatalf("review_gate = %+v", gate)
	}
	if gate.ExitCode != 1 {
		t.Fatalf("review_gate exit code = %d", gate.ExitCode)
	}
	for _, nodeID := range []string{"review", "fix", "tests", "verify", "merge"} {
		record := done.Node(nodeID)
		if record == nil || record.Status != orchestrator.NodeCompleted {
			t.Fatalf("node %s = %+v", nodeID, record)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
