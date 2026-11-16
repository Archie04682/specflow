package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"example.com/specflow/backend/internal/workspace"
)

// Store persists run records under a project's .specflow directory.
type Store struct {
	project *workspace.Project
}

// NewStore returns a store backed by the project's metadata directory.
func NewStore(project *workspace.Project) *Store {
	return &Store{project: project}
}

// Save writes the whole record, replacing the previous one atomically: a crash
// during the write leaves the previous record rather than a half-written file.
func (s *Store) Save(run *Run) error {
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run %s: %w", run.ID, err)
	}
	return writeFileAtomic(s.project.RunFile(run.ID), data)
}

// writeFileAtomic writes data through a temporary file and a rename, so a
// crash cannot leave a partially written file at path.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}

	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("commit %s: %w", path, err)
	}
	return nil
}

// Load reads a run record.
func (s *Store) Load(runID string) (*Run, error) {
	path := s.project.RunFile(runID)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}
	if err != nil {
		return nil, fmt.Errorf("read run %s: %w", runID, err)
	}

	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("parse run %s: %w", runID, err)
	}
	return &run, nil
}

// List reads every run record the project has.
func (s *Store) List() ([]*Run, error) {
	directory := filepath.Join(s.project.SpecflowDir(), "runs")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	runs := make([]*Run, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		run, err := s.Load(entry.Name())
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	return runs, nil
}
