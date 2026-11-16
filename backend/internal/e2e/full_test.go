package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"example.com/specflow/backend/internal/orchestrator"
)

// TestFullStandardFeature is the product scenario: the canonical workflow,
// the real OpenSpec skills, a real model, and the operator answering and
// approving through the API. It is opt-in twice because it is slow and makes
// many model calls.
func TestFullStandardFeature(t *testing.T) {
	if os.Getenv("PI_E2E_FULL") != "1" {
		t.Skip("set PI_E2E_FULL=1 to run the full standard-feature workflow")
	}
	if _, err := exec.LookPath("openspec"); err != nil {
		t.Skipf("openspec is not on PATH: %v", err)
	}
	model, binary := requireRealPi(t)

	shim, pidfile := writePiShim(t, binary, shimPass)
	adapter := newPiAdapter(t, shim)

	project := newFixture(t)
	copyOpenSpecSkills(t, project.Root)

	source, err := os.ReadFile(filepath.Join("..", "..", "..", "workflows", "standard-feature.yaml"))
	if err != nil {
		t.Fatalf("read canonical workflow: %v", err)
	}
	workflow := string(source)
	workflow = strings.ReplaceAll(workflow, "  model: default", "  model: "+model)
	workflow = strings.ReplaceAll(workflow, "      id: default", "      id: "+model)
	workflow = strings.ReplaceAll(workflow, "      thinking: high", "      thinking: off")
	if !strings.Contains(workflow, "  model: "+model) || !strings.Contains(workflow, "      id: "+model) {
		t.Fatalf("could not pin the canonical workflow to model %q", model)
	}
	project.writeWorkflow(t, "standard", workflow)

	server, _, root := newServerOn(t, "pi", adapter, project, nil)
	baseline := runtime.NumGoroutine()
	descriptors, haveFDs := openFDCount()

	objective := `Add a Farewell() function to main.go that returns "goodbye", with a test in main_test.go. Keep Greeting unchanged.`
	run := createRun(t, server, root, "standard", objective)
	events := driveRunFor(t, server, root, run.ID, operatorRules{
		answer: map[string]string{
			"explore": objective,
			"propose": objective,
			"apply":   "Implement the change's tasks, keep the change minimal, and run the tests.",
		},
	}, 25*time.Minute)
	done := waitRun(t, server, root, run.ID, 30*time.Second)

	if done.Status != orchestrator.RunCompleted {
		var failures []string
		for _, node := range done.Nodes {
			if node.Status != orchestrator.NodeCompleted {
				failures = append(failures, fmt.Sprintf("%s=%s (%s)", node.NodeID, node.Status, node.Error))
			}
		}
		t.Fatalf("run status = %s (%s); nodes: %s", done.Status, done.Error, strings.Join(failures, "; "))
	}
	for _, id := range []string{"explore", "propose", "apply", "review", "tests", "verify", "archive", "merge"} {
		node := done.Node(id)
		if node == nil || node.Status != orchestrator.NodeCompleted {
			t.Fatalf("node %s = %+v, want completed", id, node)
		}
		if node.Checkpoint == "" {
			t.Fatalf("node %s has no checkpoint", id)
		}
	}
	if done.Change == "" {
		t.Fatal("the run did not resolve a change")
	}

	// The merge landed on main in the project checkout.
	if body := gitShow(t, root, "main", "main.go"); !strings.Contains(strings.ToLower(body), "farewell") {
		t.Fatalf("main.go does not contain the feature:\n%s", body)
	}
	log, err := exec.Command("git", "-C", root, "log", "-1", "--format=%s").Output()
	if err != nil {
		t.Fatalf("read merge commit: %v", err)
	}
	if !strings.Contains(string(log), "Merge") {
		t.Fatalf("last commit = %q, want the merge commit", log)
	}

	// The merged tree is functional.
	command := exec.Command("go", "test", "./...")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("the merged tree does not pass its tests: %v\n%s", err, output)
	}

	// The archive node removed the active change.
	list := exec.Command("openspec", "list", "--json")
	list.Dir = root
	output, err := list.Output()
	if err != nil {
		t.Fatalf("openspec list: %v", err)
	}
	var listed struct {
		Changes []any `json:"changes"`
	}
	if err := json.Unmarshal(output, &listed); err != nil {
		t.Fatalf("decode openspec list: %v\n%s", err, output)
	}
	if len(listed.Changes) != 0 {
		t.Fatalf("active changes remain after archive: %s", output)
	}

	if _, err := os.Stat(filepath.Join(root, ".specflow", "worktrees", run.ID)); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after completion: %v", err)
	}
	assertProcessGone(t, readPID(t, pidfile))
	if haveFDs {
		assertNoFDLeak(t, descriptors)
	}
	assertNoGoroutineLeak(t, baseline)

	// The stream covered every node.
	started := make(map[string]bool)
	for _, event := range events {
		if event.Type == "node.started" {
			started[event.NodeID] = true
		}
	}
	for _, id := range []string{"explore", "propose", "apply", "review", "tests", "verify", "archive", "merge"} {
		if !started[id] {
			t.Fatalf("event stream never started node %s", id)
		}
	}
}
