package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/specflow/backend/internal/workspace"
)

func writeSkillConfig(t *testing.T, projectRoot string) {
	t.Helper()

	writeTestFile(t, filepath.Join(projectRoot, ".specflow", "config.yaml"), "skills:\n  - .specflow/skills\n")
}

func writeSkill(t *testing.T, projectRoot, name string) {
	t.Helper()

	writeTestFile(t, filepath.Join(projectRoot, ".specflow", "skills", name, "SKILL.md"), "# "+name+"\n")
}

func TestCreateRetainsTheCompiledWorkflow(t *testing.T) {
	project := newRepoProject(t)
	source := "id: snapshot\ndefaults: {harness: fake}\nnodes:\n  - {id: a, kind: agent, prompt: work}\n"
	writeProjectWorkflow(t, project, source)

	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	run, err := engine.Create(context.Background(), CreateRequest{Project: project, Workflow: "test", Objective: "retain"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	wantSource := filepath.Join(project, ".specflow", "workflows", "test.yaml")
	if run.WorkflowPath != wantSource {
		t.Fatalf("workflow path = %q", run.WorkflowPath)
	}
	wantSnapshot := filepath.Join(project, ".specflow", "runs", run.ID, "workflow.yaml")
	if run.WorkflowSnapshot != wantSnapshot {
		t.Fatalf("snapshot path = %q", run.WorkflowSnapshot)
	}
	if run.WorkflowID != "snapshot" {
		t.Fatalf("workflow id = %q", run.WorkflowID)
	}

	data, err := os.ReadFile(run.WorkflowSnapshot)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(data) != source {
		t.Fatalf("snapshot differs from the source:\n%s", data)
	}
}

func TestCreateFailsBeforeWorktreeWhenASkillIsMissing(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: missing
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work, skill: required}
`)
	writeSkillConfig(t, project)

	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()

	_, err := engine.Create(context.Background(), CreateRequest{Project: project, Workflow: "test", Objective: "missing"})
	if err == nil {
		t.Fatal("expected a missing skill to fail creation")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("error does not name the skill: %v", err)
	}

	runs, err := os.ReadDir(filepath.Join(project, ".specflow", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("a failed compile left run metadata: %v", runs)
	}
	worktrees, err := os.ReadDir(filepath.Join(project, ".specflow", "worktrees"))
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 0 {
		t.Fatalf("a failed compile left a worktree: %v", worktrees)
	}

	handle, err := workspace.Open(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	branches, err := handle.Git(context.Background(), "branch", "--list", "specflow/run-*")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(branches) != "" {
		t.Fatalf("a failed compile left a branch: %s", branches)
	}
}

func TestCreateRemovesTheRunDirectoryWhenTheWorktreeFails(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: latefailure
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work}
`)

	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	defer engine.Close()
	engine.createWorktree = func(context.Context, *workspace.Project, string) (*workspace.Worktree, error) {
		return nil, errors.New("forced worktree failure")
	}

	if _, err := engine.Create(context.Background(), CreateRequest{Project: project, Workflow: "test", Objective: "late"}); err == nil {
		t.Fatal("expected worktree creation to fail")
	}

	runs, err := os.ReadDir(filepath.Join(project, ".specflow", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("a failed creation left run metadata: %v", runs)
	}
	handle, err := workspace.Open(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	branches, err := handle.Git(context.Background(), "branch", "--list", "specflow/run-*")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(branches) != "" {
		t.Fatalf("a failed creation left a branch: %s", branches)
	}
}

func TestResumeUsesTheRecordedWorkflow(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, sourcePath string)
	}{
		{
			name: "edited source",
			mutate: func(t *testing.T, sourcePath string) {
				if err := os.WriteFile(sourcePath, []byte("this is not: [valid\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "deleted source",
			mutate: func(t *testing.T, sourcePath string) {
				if err := os.Remove(sourcePath); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := newRepoProject(t)
			writeProjectWorkflow(t, project, linearWorkflow)
			state := newFakeState()
			behaviors := map[string]*fakeBehavior{"b": {Failures: 1, Retryable: false}}
			engine := New(fakeRegistry(t, state, behaviors, allCapabilities()))
			defer engine.Close()

			run := startRun(t, engine, project, "resume after edit")
			failed := waitTerminal(t, engine, project, run.ID)
			if failed.Status != RunFailed {
				t.Fatalf("status = %s", failed.Status)
			}

			test.mutate(t, filepath.Join(project, ".specflow", "workflows", "test.yaml"))

			if err := engine.Resume(context.Background(), project, run.ID); err != nil {
				t.Fatalf("resume: %v", err)
			}
			done := waitTerminal(t, engine, project, run.ID)
			if done.Status != RunCompleted {
				t.Fatalf("status = %s (%s)", done.Status, done.Error)
			}
			if state.count("a") != 1 || state.count("b") != 2 || state.count("c") != 1 {
				t.Fatalf("executions: a=%d b=%d c=%d", state.count("a"), state.count("b"), state.count("c"))
			}
		})
	}
}

func TestResumeFallsBackToTheSourceForALegacyRecord(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	state := newFakeState()
	behaviors := map[string]*fakeBehavior{"b": {Failures: 1, Retryable: false}}
	engine := New(fakeRegistry(t, state, behaviors, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "legacy")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}

	handle, err := workspace.Open(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(handle)
	stored, err := store.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := stored.WorkflowSnapshot
	if snapshotPath == "" {
		t.Fatal("the run has no snapshot")
	}
	stored.WorkflowSnapshot = ""
	if err := store.Save(stored); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatal(err)
	}

	restarted := New(fakeRegistry(t, state, behaviors, allCapabilities()))
	defer restarted.Close()

	if err := restarted.Resume(context.Background(), project, run.ID); err != nil {
		t.Fatalf("resume legacy run: %v", err)
	}
	done := waitTerminal(t, restarted, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}
}

func TestResumeRejectsAWorkflowThatDoesNotMatchTheRecord(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, linearWorkflow)
	state := newFakeState()
	engine := New(fakeRegistry(t, state, map[string]*fakeBehavior{"b": {Failures: 1, Retryable: false}}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "mismatch")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}

	tests := []struct {
		name     string
		workflow string
	}{
		{
			name: "different node set",
			workflow: "id: other\ndefaults: {harness: fake}\n" +
				"nodes:\n  - {id: x, kind: agent, prompt: other}\n",
		},
		{
			name: "reordered nodes",
			workflow: "id: linear\ndefaults: {harness: fake}\n" +
				"nodes:\n" +
				"  - {id: b, kind: agent, prompt: second}\n" +
				"  - {id: a, kind: agent, prompt: first}\n" +
				"  - {id: c, kind: agent, prompt: third}\n" +
				"edges:\n" +
				"  - {from: a, to: b}\n" +
				"  - {from: b, to: c}\n",
		},
		{
			name: "changed kind",
			workflow: "id: linear\ndefaults: {harness: fake}\n" +
				"nodes:\n" +
				"  - {id: a, kind: agent, prompt: first}\n" +
				"  - {id: b, kind: command, command: \"true\"}\n" +
				"  - {id: c, kind: agent, prompt: third}\n" +
				"edges:\n" +
				"  - {from: a, to: b}\n" +
				"  - {from: b, to: c}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(run.WorkflowSnapshot, []byte(test.workflow), 0o644); err != nil {
				t.Fatal(err)
			}

			err := engine.Resume(context.Background(), project, run.ID)
			if err == nil {
				t.Fatal("resuming a mismatched workflow succeeded")
			}
			if !strings.Contains(err.Error(), "recorded") {
				t.Fatalf("error does not explain the mismatch: %v", err)
			}
			if state.count("c") != 0 {
				t.Fatal("a mismatched workflow executed a node")
			}
		})
	}
}

func TestCleanupIncompleteRunRemovesState(t *testing.T) {
	projectRoot := newRepoProject(t)
	handle, err := workspace.Open(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	runID := "cleanup-run"
	if _, err := handle.CreateWorktree(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(handle.RunDir(runID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(handle.WorkflowSnapshotFile(runID), []byte("id: cleanup\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanupIncompleteRun(context.Background(), handle, runID)

	if _, err := os.Stat(handle.RunDir(runID)); !os.IsNotExist(err) {
		t.Fatalf("run directory remains: %v", err)
	}
	if _, err := os.Stat(handle.WorktreeDir(runID)); !os.IsNotExist(err) {
		t.Fatalf("worktree directory remains: %v", err)
	}
	branches, err := handle.Git(context.Background(), "branch", "--list", handle.RunBranch(runID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(branches) != "" {
		t.Fatalf("branch remains: %s", branches)
	}
}

func TestInspectionDoesNotCompileAfterASkillIsRemoved(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: skillcheck
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work, skill: needed}
`)
	writeSkillConfig(t, project)
	writeSkill(t, project, "needed")

	state := newFakeState()
	behaviors := map[string]*fakeBehavior{"a": {Failures: 1, Retryable: false}}
	engine := New(fakeRegistry(t, state, behaviors, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "inspect")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}

	if err := os.RemoveAll(filepath.Join(project, ".specflow", "skills", "needed")); err != nil {
		t.Fatal(err)
	}

	restarted := New(fakeRegistry(t, state, behaviors, allCapabilities()))
	defer restarted.Close()

	loaded, err := restarted.Get(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("inspection failed after the skill was removed: %v", err)
	}
	if loaded.Status != RunFailed {
		t.Fatalf("inspection status = %s", loaded.Status)
	}

	err = restarted.Resume(context.Background(), project, run.ID)
	if err == nil {
		t.Fatal("resuming without the skill succeeded")
	}
	for _, fragment := range []string{"needed", filepath.Join(project, ".specflow", "skills")} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not mention %q", err, fragment)
		}
	}

	stillFailed, err := restarted.Get(context.Background(), project, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillFailed.Status != RunFailed {
		t.Fatalf("a failed resume changed the run status to %s", stillFailed.Status)
	}
	if state.count("a") != 1 {
		t.Fatalf("the node ran during a refused resume: %d", state.count("a"))
	}
}

func TestExecutionUsesThePlanValues(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: values
defaults:
  harness: fake
  model: {id: cheap, thinking: low}
nodes:
  - id: a
    kind: agent
    prompt: work
    skill: needed
    model: {id: coder, thinking: high}
`)
	writeSkillConfig(t, project)
	writeSkill(t, project, "needed")

	state := newFakeState()
	engine := New(fakeRegistry(t, state, nil, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "plan values")
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	request, ok := state.request("a")
	if !ok {
		t.Fatal("the adapter saw no request")
	}
	if request.Model.ID != "coder" || request.Model.Thinking != "high" {
		t.Fatalf("model = %+v", request.Model)
	}
	if request.Skill == nil || request.Skill.Name != "needed" || len(request.Skill.Paths) != 1 {
		t.Fatalf("skill = %+v", request.Skill)
	}
	if !filepath.IsAbs(request.Skill.Paths[0]) {
		t.Fatalf("skill path is not absolute: %q", request.Skill.Paths[0])
	}
	if len(request.Skills) != 1 || request.Skills[0].Name != "needed" {
		t.Fatalf("skills = %+v, want the plural request binding too", request.Skills)
	}
	if request.Workdir != run.Worktree {
		t.Fatalf("workdir = %q, want %q", request.Workdir, run.Worktree)
	}
}

func TestResumeKeepsRecordedInteractionDeadlines(t *testing.T) {
	project := newRepoProject(t)
	writeProjectWorkflow(t, project, `
id: pinned-timeouts
defaults: {harness: fake, ask_timeout: 5m, approval_timeout: 7m}
nodes:
  - {id: a, kind: agent, prompt: first}
  - {id: b, kind: agent, prompt: second}
edges:
  - {from: a, to: b}
`)
	engine := New(fakeRegistry(t, newFakeState(), map[string]*fakeBehavior{
		"b": {Failures: 1, Retryable: false},
	}, allCapabilities()))
	defer engine.Close()

	run := startRun(t, engine, project, "pin the interaction deadlines")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s", failed.Status)
	}

	writeProjectWorkflow(t, project, `
id: pinned-timeouts
defaults: {harness: fake, ask_timeout: 99m, approval_timeout: 98m}
nodes:
  - {id: a, kind: agent, prompt: first}
  - {id: b, kind: agent, prompt: second}
edges:
  - {from: a, to: b}
`)

	if err := engine.Resume(context.Background(), project, run.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	done := waitTerminal(t, engine, project, run.ID)
	if done.Status != RunCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Error)
	}

	live, err := engine.inspect(context.Background(), project, run.ID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	plan, err := engine.planFor(live, true)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	node := plan.Node("a")
	if node.AskTimeout.Duration() != 5*time.Minute {
		t.Fatalf("ask timeout = %s, want the recorded 5m", node.AskTimeout)
	}
	if node.ApprovalTimeout.Duration() != 7*time.Minute {
		t.Fatalf("approval timeout = %s, want the recorded 7m", node.ApprovalTimeout)
	}
}
