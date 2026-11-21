package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func folderProject(t *testing.T) *Project {
	t.Helper()

	project := newProject(t)
	if err := project.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	return project
}

func TestCreateFolderWorkspaceCopiesAndRecordsBase(t *testing.T) {
	project := folderProject(t)
	if err := os.MkdirAll(filepath.Join(project.Root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project.Root, "docs", "note.md"), "hello\n")

	worktree, err := project.CreateFolderWorkspace(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if worktree.Mode != ModeFolder {
		t.Fatalf("mode = %q", worktree.Mode)
	}

	if _, err := os.Stat(filepath.Join(worktree.Path, "docs", "note.md")); err != nil {
		t.Fatalf("copy is missing a project file: %v", err)
	}
	for _, excluded := range []string{".specflow", ".git"} {
		if _, err := os.Stat(filepath.Join(worktree.Path, excluded)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("copy contains %s: %v", excluded, err)
		}
	}

	base, err := ReadManifest(project.BaseManifestFile("run-1"))
	if err != nil {
		t.Fatalf("read base: %v", err)
	}
	for _, path := range []string{"README.md", "docs/note.md"} {
		if _, ok := base[path]; !ok {
			t.Fatalf("base manifest has no %q: %+v", path, base)
		}
	}
	for _, excluded := range []string{".specflow/config.yaml", ".git/HEAD"} {
		if _, ok := base[excluded]; ok {
			t.Fatalf("base manifest records %s: %+v", excluded, base)
		}
	}
}

func TestFolderSnapshotAndRestore(t *testing.T) {
	project := folderProject(t)
	worktree, err := project.CreateFolderWorkspace(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(worktree.Path, "after.txt"), "after\n")

	snapshot, err := worktree.Checkpoint(context.Background(), "node-a")
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	writeFile(t, filepath.Join(worktree.Path, "later.txt"), "later\n")

	if err := worktree.Restore(context.Background(), snapshot); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree.Path, "after.txt")); err != nil {
		t.Fatalf("restore lost an earlier file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree.Path, "later.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore kept a later file: %v", err)
	}
}

func TestApplyRunWritesAndDeletes(t *testing.T) {
	project := folderProject(t)
	writeFile(t, filepath.Join(project.Root, "keep.txt"), "keep\n")
	worktree, err := project.CreateFolderWorkspace(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(worktree.Path, "README.md"), "changed\n")
	writeFile(t, filepath.Join(worktree.Path, "new.txt"), "new\n")
	if err := os.Remove(filepath.Join(worktree.Path, "keep.txt")); err != nil {
		t.Fatal(err)
	}

	report, err := project.ApplyRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if strings.Join(report.Written, ",") != "README.md,new.txt" {
		t.Fatalf("written = %v", report.Written)
	}
	if strings.Join(report.Deleted, ",") != "keep.txt" {
		t.Fatalf("deleted = %v", report.Deleted)
	}

	data, err := os.ReadFile(filepath.Join(project.Root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "changed\n" {
		t.Fatalf("README = %q", data)
	}
	if _, err := os.Stat(filepath.Join(project.Root, "new.txt")); err != nil {
		t.Fatalf("new file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project.Root, "keep.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted file still present: %v", err)
	}
}

func TestApplyRunRefusesAChangedProject(t *testing.T) {
	project := folderProject(t)
	worktree, err := project.CreateFolderWorkspace(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(worktree.Path, "new.txt"), "new\n")
	writeFile(t, filepath.Join(project.Root, "README.md"), "edited meanwhile\n")

	_, err = project.ApplyRun(context.Background(), "run-1")
	if !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("err = %v, want ErrProjectChanged", err)
	}
	if _, err := os.Stat(filepath.Join(project.Root, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused apply wrote a file: %v", err)
	}
}

func TestReleaseRemovesCopyAndSnapshots(t *testing.T) {
	project := folderProject(t)
	worktree, err := project.CreateFolderWorkspace(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Checkpoint(context.Background(), "node-a"); err != nil {
		t.Fatal(err)
	}

	if err := worktree.Release(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(worktree.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copy still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project.RunDir("run-1"), snapshotsDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshots still present: %v", err)
	}
}
