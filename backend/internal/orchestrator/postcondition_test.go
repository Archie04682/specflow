package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileExistsPostcondition(t *testing.T) {
	workdir := t.TempDir()
	postcondition := &Postcondition{Type: "file_exists", Path: "marker.txt", Description: "marker must exist"}

	_, err := EvaluatePostcondition(postcondition, PostconditionEnv{NodeID: "explore", Workdir: workdir})
	if err == nil {
		t.Fatal("expected a missing file to fail the postcondition")
	}

	if err := os.WriteFile(filepath.Join(workdir, "marker.txt"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluatePostcondition(postcondition, PostconditionEnv{NodeID: "explore", Workdir: workdir}); err != nil {
		t.Fatalf("present file failed: %v", err)
	}
}

func TestExitZeroPostcondition(t *testing.T) {
	postcondition := &Postcondition{Type: "exit_zero"}

	if _, err := EvaluatePostcondition(postcondition, PostconditionEnv{NodeID: "verify", ExitCode: 0}); err != nil {
		t.Fatalf("exit zero failed: %v", err)
	}

	_, err := EvaluatePostcondition(postcondition, PostconditionEnv{NodeID: "verify", ExitCode: 1})
	if err == nil || !strings.Contains(err.Error(), "exited with 1") {
		t.Fatalf("err = %v", err)
	}
}

func TestNonePostconditionAlwaysHolds(t *testing.T) {
	for _, postcondition := range []*Postcondition{nil, {Type: "none"}} {
		if _, err := EvaluatePostcondition(postcondition, PostconditionEnv{}); err != nil {
			t.Fatalf("none failed: %v", err)
		}
	}
}

func TestNewChangePostcondition(t *testing.T) {
	project := t.TempDir()
	changes := filepath.Join(project, "openspec", "changes")
	for _, name := range []string{"old-change", "archive"} {
		if err := os.MkdirAll(filepath.Join(changes, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	env := PostconditionEnv{NodeID: "propose", Root: project, Baseline: []string{"old-change"}}
	postcondition := &Postcondition{Type: "new_change"}

	if _, err := EvaluatePostcondition(postcondition, env); err == nil {
		t.Fatal("expected no-new-change to fail")
	}

	if err := os.MkdirAll(filepath.Join(changes, "add-export"), 0o755); err != nil {
		t.Fatal(err)
	}
	change, err := EvaluatePostcondition(postcondition, env)
	if err != nil {
		t.Fatalf("one new change failed: %v", err)
	}
	if change != "add-export" {
		t.Fatalf("change = %q", change)
	}

	if err := os.MkdirAll(filepath.Join(changes, "another"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = EvaluatePostcondition(postcondition, env)
	if err == nil || !strings.Contains(err.Error(), "several") {
		t.Fatalf("err = %v, want an ambiguity failure", err)
	}
}

func TestTasksCompletePostcondition(t *testing.T) {
	project := t.TempDir()
	tasksPath := filepath.Join(project, "openspec", "changes", "add-export", "tasks.md")
	if err := os.MkdirAll(filepath.Dir(tasksPath), 0o755); err != nil {
		t.Fatal(err)
	}

	env := PostconditionEnv{NodeID: "apply", Root: project, Change: "add-export"}
	postcondition := &Postcondition{Type: "tasks_complete"}

	if err := os.WriteFile(tasksPath, []byte("# Tasks\n\n- [x] 1.1 Done\n* [ ] 1.2 Still open\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := EvaluatePostcondition(postcondition, env)
	if err == nil || !strings.Contains(err.Error(), "1.2 Still open") {
		t.Fatalf("err = %v, want the unfinished task named", err)
	}

	if err := os.WriteFile(tasksPath, []byte("- [x] 1.1 Done\n- [X] 1.2 Also done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluatePostcondition(postcondition, env); err != nil {
		t.Fatalf("all tasks checked failed: %v", err)
	}
}

func TestTasksCompleteRequiresAResolvedChange(t *testing.T) {
	_, err := EvaluatePostcondition(&Postcondition{Type: "tasks_complete"}, PostconditionEnv{NodeID: "apply"})
	if err == nil || !strings.Contains(err.Error(), "no change") {
		t.Fatalf("err = %v", err)
	}
}

func TestPostconditionFailureIsTyped(t *testing.T) {
	_, err := EvaluatePostcondition(&Postcondition{Type: "exit_zero"}, PostconditionEnv{NodeID: "verify", ExitCode: 2})

	var failure *PostconditionFailure
	if !errors.As(err, &failure) {
		t.Fatalf("err = %T, want *PostconditionFailure", err)
	}
	if failure.NodeID != "verify" {
		t.Fatalf("failure node = %q", failure.NodeID)
	}
}
