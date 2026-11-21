package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// baseManifestName is the run's record of the files it started from.
const baseManifestName = "base.json"

// snapshotsDirName is where a folder run's per-node snapshots live, inside its
// own metadata directory.
const snapshotsDirName = "snapshots"

// BaseSnapshotID is the snapshot of the copy as it was created, before any
// node ran. A clean retry of the first node restores it.
const BaseSnapshotID = snapshotsDirName + "/base"

// gitDirName is the repository metadata a folder run never copies: a run that
// wrote git objects back on apply could corrupt the project's repository.
const gitDirName = ".git"

// BaseManifestFile is the manifest a folder run is compared against.
func (p *Project) BaseManifestFile(runID string) string {
	return filepath.Join(p.RunDir(runID), baseManifestName)
}

// CreateFolderWorkspace copies the project into the run's workspace directory,
// excluding .specflow, and records the copy's files as the run's base.
func (p *Project) CreateFolderWorkspace(ctx context.Context, runID string) (*Worktree, error) {
	path := p.WorktreeDir(runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create workspace parent: %w", err)
	}
	// A previous attempt's copy is never reused: the run's base is the project
	// as it is now.
	if err := os.RemoveAll(path); err != nil {
		return nil, fmt.Errorf("clear workspace %s: %w", path, err)
	}

	if err := copyTree(p.Root, path, isExcludedPath); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("copy project into %s: %w", path, err)
	}

	manifest, err := Manifest(path)
	if err != nil {
		_ = os.RemoveAll(path)
		return nil, err
	}
	if err := os.MkdirAll(p.RunDir(runID), 0o755); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("create run directory: %w", err)
	}
	if err := WriteManifest(p.BaseManifestFile(runID), manifest); err != nil {
		_ = os.RemoveAll(path)
		return nil, err
	}

	// The copy as created is the base a clean retry of the first node returns
	// to; node snapshots are taken after each node instead.
	baseSnapshot := filepath.Join(p.RunDir(runID), filepath.FromSlash(BaseSnapshotID))
	if err := os.MkdirAll(filepath.Dir(baseSnapshot), 0o755); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("create base snapshot: %w", err)
	}
	if err := copyTree(path, baseSnapshot, nil); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("snapshot the empty run: %w", err)
	}

	return &Worktree{
		Path:    path,
		Mode:    ModeFolder,
		Root:    p.Root,
		RunID:   runID,
		project: p,
	}, nil
}

// WriteManifest writes a manifest atomically.
func WriteManifest(path string, manifest map[string]string) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := writeFileAtomic(path, append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// ReadManifest reads a manifest written by WriteManifest.
func ReadManifest(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	manifest := make(map[string]string)
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return manifest, nil
}

// Manifest hashes every regular file under root, keyed by its slash-separated
// relative path. Directories, symlinks, and the root .specflow directory are
// not part of a folder run's base.
func Manifest(root string) (map[string]string, error) {
	manifest := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == "." {
			return nil
		}
		if isExcludedPath(relative) {
			return nil
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		manifest[relative] = sum
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("hash %s: %w", root, err)
	}
	return manifest, nil
}

// ChangedPaths lists the paths where one manifest differs from another.
func ChangedPaths(before, after map[string]string) []string {
	changed := make([]string, 0)
	for path, sum := range after {
		if previous, ok := before[path]; !ok || previous != sum {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

// hashFile is the content hash a manifest compares.
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// isExcludedPath reports a path a folder run never copies: the project's
// metadata directory and its git metadata.
func isExcludedPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	for _, name := range []string{specflowDirName, gitDirName} {
		if relative == name || strings.HasPrefix(relative, name+"/") {
			return true
		}
	}
	return false
}

// copyTree copies regular files and directories, preserving mode bits. Symlinks
// are skipped: following one could leave the project, and copying the link
// itself would make the copy depend on a target outside it.
func copyTree(source, destination string, skip func(relative string) bool) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(destination, 0o755)
		}
		relative = filepath.ToSlash(relative)
		if skip != nil && skip(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

// copyFile copies one file and its mode.
func copyFile(source, destination string, mode fs.FileMode) error {
	from, err := os.Open(source)
	if err != nil {
		return err
	}
	defer from.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	to, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(to, from); err != nil {
		to.Close()
		return err
	}
	return to.Close()
}

// snapshot records the copy's current state and returns the identifier recorded
// against the node.
func (w *Worktree) snapshot(nodeID string) (string, error) {
	name := fmt.Sprintf("%s-%d", nodeID, time.Now().UnixNano())
	relative := filepath.Join(snapshotsDirName, name)
	target := filepath.Join(w.project.RunDir(w.RunID), relative)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create snapshot for %s: %w", nodeID, err)
	}
	if err := copyTree(w.Path, target, nil); err != nil {
		_ = os.RemoveAll(target)
		return "", fmt.Errorf("snapshot node %s: %w", nodeID, err)
	}
	return filepath.ToSlash(relative), nil
}

// restoreSnapshot replaces the copy with a recorded snapshot.
func (w *Worktree) restoreSnapshot(identifier string) error {
	source := filepath.Join(w.project.RunDir(w.RunID), filepath.FromSlash(identifier))
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("restore %s: %w", identifier, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("restore %s: not a snapshot", identifier)
	}

	entries, err := os.ReadDir(w.Path)
	if err != nil {
		return fmt.Errorf("clear workspace %s: %w", w.Path, err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(w.Path, entry.Name())); err != nil {
			return fmt.Errorf("clear workspace %s: %w", w.Path, err)
		}
	}
	if err := copyTree(source, w.Path, nil); err != nil {
		return fmt.Errorf("restore %s: %w", identifier, err)
	}
	return nil
}

// releaseFolderWorkspace removes a folder run's copy and its snapshots, keeping
// the run's record and journal.
func (w *Worktree) releaseFolderWorkspace() error {
	if err := os.RemoveAll(w.Path); err != nil {
		return fmt.Errorf("remove workspace %s: %w", w.Path, err)
	}
	if err := os.RemoveAll(filepath.Join(w.project.RunDir(w.RunID), snapshotsDirName)); err != nil {
		return fmt.Errorf("remove snapshots for %s: %w", w.RunID, err)
	}
	return nil
}

// Release removes a run's workspace. A git worktree keeps its branch; a folder
// workspace and its snapshots are gone.
func (w *Worktree) Release(ctx context.Context) error {
	if w.Mode == ModeFolder {
		return w.releaseFolderWorkspace()
	}
	return w.project.RemoveWorktree(ctx, w.Path)
}
