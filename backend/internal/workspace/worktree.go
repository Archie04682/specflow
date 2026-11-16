package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Worktree is a run's private checkout.
type Worktree struct {
	// Path is the directory the run's nodes execute in.
	Path string `json:"path"`
	// Branch is the run's own branch.
	Branch string `json:"branch"`
	// Base is the commit the branch was created from.
	Base string `json:"base"`
	// Source is the branch the project had checked out when the run started.
	Source string `json:"source"`
}

// CreateWorktree branches a fresh worktree for the run from the project's
// current commit and records the branch, base, and source branch.
func (p *Project) CreateWorktree(ctx context.Context, runID string) (*Worktree, error) {
	if err := p.CheckIsolatable(ctx); err != nil {
		return nil, err
	}

	source, err := p.CurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	base, err := p.Head(ctx)
	if err != nil {
		return nil, err
	}

	path := p.WorktreeDir(runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create worktree parent: %w", err)
	}

	branch := p.RunBranch(runID)
	if _, err := p.Git(ctx, "worktree", "add", "-b", branch, path, base); err != nil {
		return nil, fmt.Errorf("create worktree for run %s: %w", runID, err)
	}

	return &Worktree{Path: path, Branch: branch, Base: base, Source: source}, nil
}

// RemoveWorktree deletes a run's checkout but keeps its branch, so the work
// remains mergeable and inspectable.
func (p *Project) RemoveWorktree(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if _, err := p.Git(ctx, "worktree", "remove", "--force", path); err != nil {
		return fmt.Errorf("remove worktree %s: %w", path, err)
	}
	return nil
}

// Head returns the commit the worktree currently has checked out.
func (w *Worktree) Head(ctx context.Context) (string, error) {
	return runGit(ctx, w.Path, "rev-parse", "HEAD")
}

// Dirty reports whether the worktree has any change at all, tracked or not.
func (w *Worktree) Dirty(ctx context.Context) (bool, error) {
	output, err := runGit(ctx, w.Path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// Checkpoint commits everything in the worktree and returns the resulting
// commit. A worktree that changed nothing reuses its current commit instead of
// creating an empty one.
func (w *Worktree) Checkpoint(ctx context.Context, nodeID string) (string, error) {
	dirty, err := w.Dirty(ctx)
	if err != nil {
		return "", err
	}
	if !dirty {
		return w.Head(ctx)
	}

	if _, err := runGit(ctx, w.Path, "add", "-A"); err != nil {
		return "", fmt.Errorf("stage node %s: %w", nodeID, err)
	}

	// Checkpoints use a fixed identity: a machine without user.name configured
	// must not fail a run over attribution. --no-verify keeps the user's hooks
	// out of the orchestrator's bookkeeping.
	_, err = runGit(ctx, w.Path,
		"-c", "user.name=SpecFlow",
		"-c", "user.email=specflow@localhost",
		"commit", "--no-verify", "-m", "specflow: "+nodeID)
	if err != nil {
		return "", fmt.Errorf("checkpoint node %s: %w", nodeID, err)
	}

	return w.Head(ctx)
}

// Restore returns the worktree to a checkpoint, discarding tracked changes and
// untracked files created after it. Ignored files are left alone: clean is
// given -d but never -x.
func (w *Worktree) Restore(ctx context.Context, commit string) error {
	if commit == "" {
		return errors.New("restore: empty checkpoint")
	}
	if _, err := runGit(ctx, w.Path, "reset", "--hard", commit); err != nil {
		return fmt.Errorf("restore worktree to %s: %w", commit, err)
	}
	if _, err := runGit(ctx, w.Path, "clean", "-fd"); err != nil {
		return fmt.Errorf("clean worktree after restore: %w", err)
	}
	return nil
}

// CheckMergeable reports whether merging into source is safe: the project must
// be on the source branch and must have no uncommitted changes. The merge
// itself runs in the project, never in the run worktree, because the worktree
// already has the run branch checked out.
func (p *Project) CheckMergeable(ctx context.Context, source string) error {
	if source == "" {
		return errors.New("merge: run has no recorded source branch")
	}

	current, err := p.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	if current != source {
		return fmt.Errorf("merge: project is on branch %q, expected %q", current, source)
	}

	paths, err := p.Uncommitted(ctx)
	if err != nil {
		return err
	}
	if len(paths) > 0 {
		return fmt.Errorf("merge: project has uncommitted changes: %s", strings.Join(paths, ", "))
	}
	return nil
}
