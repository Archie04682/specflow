package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/specflow/backend/internal/workspace"
)

const folderWorkflow = `id: test
name: Folder test
version: 1
nodes:
  - id: write
    title: Write
    kind: command
    command: "echo made > made.txt"
`

func newFolderProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".specflow", "config.yaml"), "vcs: folder\n")
	writeTestFile(t, filepath.Join(dir, "notes.txt"), "notes\n")
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestFolderRunAppliesItsChanges(t *testing.T) {
	ctx := context.Background()
	project := newFolderProject(t)
	writeProjectWorkflow(t, project, folderWorkflow)
	engine := newTestEngine(t)

	run := startRun(t, engine, project, "objective")
	completed := waitTerminal(t, engine, project, run.ID)
	if completed.Status != RunCompleted {
		t.Fatalf("status = %s: %s", completed.Status, completed.Error)
	}
	if completed.VCS != workspace.ModeFolder || completed.Apply != RunApplyPending {
		t.Fatalf("record = vcs %q apply %q", completed.VCS, completed.Apply)
	}

	// The project is untouched until the operator applies the run.
	if _, err := os.Stat(filepath.Join(project, "made.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the project changed before apply: %v", err)
	}

	report, err := engine.Apply(ctx, project, run.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if strings.Join(report.Written, ",") != "made.txt" {
		t.Fatalf("written = %v", report.Written)
	}
	if _, err := os.Stat(filepath.Join(project, "made.txt")); err != nil {
		t.Fatalf("apply did not write the file: %v", err)
	}

	applied, err := engine.Get(ctx, project, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Apply != RunApplyApplied {
		t.Fatalf("apply state = %q", applied.Apply)
	}
	if _, err := os.Stat(filepath.Join(project, ".specflow", "worktrees", run.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the copy survived apply: %v", err)
	}
}

func TestFolderRunHoldsTheProjectUntilAppliedOrDiscarded(t *testing.T) {
	ctx := context.Background()
	project := newFolderProject(t)
	writeProjectWorkflow(t, project, folderWorkflow)
	engine := newTestEngine(t)

	if _, err := engine.Create(ctx, CreateRequest{Project: project, Workflow: "test", Objective: "one"}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_, err := engine.Create(ctx, CreateRequest{Project: project, Workflow: "test", Objective: "two"})
	if err == nil || !strings.Contains(err.Error(), "still holds this project") {
		t.Fatalf("second run was accepted: %v", err)
	}
}

func TestFolderRunDiscardsWithoutTouchingTheProject(t *testing.T) {
	ctx := context.Background()
	project := newFolderProject(t)
	writeProjectWorkflow(t, project, folderWorkflow)
	engine := newTestEngine(t)

	run := startRun(t, engine, project, "objective")
	waitTerminal(t, engine, project, run.ID)
	if err := engine.Discard(ctx, project, run.ID); err != nil {
		t.Fatalf("discard: %v", err)
	}

	discarded, err := engine.Get(ctx, project, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if discarded.Apply != RunApplyDiscarded {
		t.Fatalf("apply state = %q", discarded.Apply)
	}
	if _, err := os.Stat(filepath.Join(project, "made.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discard wrote into the project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".specflow", "worktrees", run.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the copy survived discard: %v", err)
	}
}

func TestFolderRunRefusesAProjectDirectoryNode(t *testing.T) {
	project := newFolderProject(t)
	writeProjectWorkflow(t, project, `id: test
name: Mixed
version: 1
nodes:
  - id: outside
    title: Outside
    kind: command
    workdir: project
    command: "true"
`)
	engine := newTestEngine(t)

	_, err := engine.Create(context.Background(), CreateRequest{Project: project, Workflow: "test", Objective: "objective"})
	if err == nil || !strings.Contains(err.Error(), "folder run cannot isolate") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyRefusesAProjectChangedWhilePending(t *testing.T) {
	ctx := context.Background()
	project := newFolderProject(t)
	writeProjectWorkflow(t, project, folderWorkflow)
	engine := newTestEngine(t)

	run := startRun(t, engine, project, "objective")
	waitTerminal(t, engine, project, run.ID)
	writeTestFile(t, filepath.Join(project, "notes.txt"), "edited\n")

	if _, err := engine.Apply(ctx, project, run.ID); !errors.Is(err, workspace.ErrProjectChanged) {
		t.Fatalf("err = %v, want ErrProjectChanged", err)
	}
}

func TestFolderRunCleanRetryRestoresTheBaseCopy(t *testing.T) {
	ctx := context.Background()
	project := newFolderProject(t)
	marker := filepath.Join(t.TempDir(), "marker")
	writeProjectWorkflow(t, project, fmt.Sprintf(`id: test
name: Flaky
version: 1
nodes:
  - id: flaky
    kind: command
    command: "if [ -f %s ]; then echo ok > ok.txt; else echo partial > partial.txt; touch %s; exit 1; fi"
`, marker, marker))
	engine := newTestEngine(t)

	run := startRun(t, engine, project, "objective")
	failed := waitTerminal(t, engine, project, run.ID)
	if failed.Status != RunFailed {
		t.Fatalf("status = %s, want failed: %s", failed.Status, failed.Error)
	}

	// The failed attempt left a file the retry must not see again.
	copyPath := filepath.Join(project, ".specflow", "worktrees", run.ID)
	if _, err := os.Stat(filepath.Join(copyPath, "partial.txt")); err != nil {
		t.Fatalf("the failed attempt left no partial file: %v", err)
	}

	if err := engine.Retry(ctx, project, run.ID, "flaky", RetryClean); err != nil {
		t.Fatalf("clean retry: %v", err)
	}
	completed := waitFor(t, engine, project, run.ID, func(run *Run) bool {
		return run.Status == RunCompleted
	})
	if completed.Status != RunCompleted {
		t.Fatalf("status = %s", completed.Status)
	}
	if _, err := os.Stat(filepath.Join(copyPath, "partial.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the partial file survived the restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(copyPath, "ok.txt")); err != nil {
		t.Fatalf("the retried node did not run: %v", err)
	}
}
