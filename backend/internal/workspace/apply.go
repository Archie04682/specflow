package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrProjectChanged reports a project edited while a folder run waited to be
// applied. Applying would overwrite those edits, so it is refused instead.
var ErrProjectChanged = errors.New("the project changed since the run started")

// ApplyReport names what applying a folder run wrote and deleted.
type ApplyReport struct {
	Written []string `json:"written"`
	Deleted []string `json:"deleted"`
}

// ApplyRun writes a completed folder run's changes into the project: files the
// run added or changed are copied back, and files it deleted are removed. It
// refuses when a project file no longer matches the run's base manifest, so an
// edit made while the run was pending is never overwritten. Nothing is staged
// or committed.
func (p *Project) ApplyRun(ctx context.Context, runID string) (ApplyReport, error) {
	workspacePath := p.WorktreeDir(runID)
	if info, err := os.Stat(workspacePath); err != nil || !info.IsDir() {
		return ApplyReport{}, fmt.Errorf("apply run %s: the run has no workspace copy", runID)
	}

	base, err := ReadManifest(p.BaseManifestFile(runID))
	if err != nil {
		return ApplyReport{}, err
	}
	current, err := Manifest(p.Root)
	if err != nil {
		return ApplyReport{}, err
	}
	if changed := ChangedPaths(base, current); len(changed) > 0 {
		return ApplyReport{}, fmt.Errorf("%w: %s", ErrProjectChanged, strings.Join(changed, ", "))
	}

	runFiles, err := Manifest(workspacePath)
	if err != nil {
		return ApplyReport{}, err
	}

	report := ApplyReport{Written: []string{}, Deleted: []string{}}
	for relative, sum := range runFiles {
		if base[relative] == sum {
			continue
		}
		source := filepath.Join(workspacePath, filepath.FromSlash(relative))
		info, err := os.Stat(source)
		if err != nil {
			return report, fmt.Errorf("apply %s: %w", relative, err)
		}
		target := filepath.Join(p.Root, filepath.FromSlash(relative))
		if err := copyFile(source, target, info.Mode().Perm()); err != nil {
			return report, fmt.Errorf("apply %s: %w", relative, err)
		}
		report.Written = append(report.Written, relative)
	}

	for relative := range base {
		if _, ok := runFiles[relative]; ok {
			continue
		}
		target := filepath.Join(p.Root, filepath.FromSlash(relative))
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return report, fmt.Errorf("apply delete %s: %w", relative, err)
		}
		report.Deleted = append(report.Deleted, relative)
	}

	sort.Strings(report.Written)
	sort.Strings(report.Deleted)
	return report, nil
}
