package openspec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	changesDirName = "changes"
	archiveName    = "archive"
	tasksFileName  = "tasks.md"
)

var uncheckedTask = regexp.MustCompile(`^\s*[-*]\s*\[\s*\]\s+(.+?)\s*$`)

// ListChanges returns the active change names a project has, excluding the
// archive. A project with no openspec directory has no changes.
func ListChanges(projectRoot string) ([]string, error) {
	directory := filepath.Join(projectRoot, "openspec", changesDirName)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", directory, err)
	}

	var changes []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == archiveName {
			continue
		}
		changes = append(changes, entry.Name())
	}
	return changes, nil
}

// ChangeDir returns the directory of one change.
func ChangeDir(projectRoot, change string) string {
	return filepath.Join(projectRoot, "openspec", changesDirName, change)
}

// UncheckedTasks returns the task lines that are still open in a change's
// tasks.md. Only the conventional checkbox form is recognised.
func UncheckedTasks(projectRoot, change string) ([]string, error) {
	if change == "" {
		return nil, errors.New("no change is resolved")
	}

	path := filepath.Join(ChangeDir(projectRoot, change), tasksFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var unchecked []string
	for _, line := range strings.Split(string(data), "\n") {
		if match := uncheckedTask.FindStringSubmatch(line); match != nil {
			unchecked = append(unchecked, match[1])
		}
	}
	return unchecked, nil
}
