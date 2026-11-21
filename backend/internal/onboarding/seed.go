package onboarding

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	specflow "example.com/specflow"
	"example.com/specflow/backend/internal/workspace"
)

// Errors a seed request distinguishes.
var (
	ErrFlowExists       = errors.New("flow already exists")
	ErrTemplateNotFound = errors.New("no such workflow template")
)

// SeedResult names the flow that was written and where.
type SeedResult struct {
	Reference string `json:"reference"`
	Path      string `json:"path"`
}

// SeedFlow writes the embedded template for a reference into the project's
// workflow directory. It never overwrites: a project copy may carry the
// operator's edits, and the editor already owns that path.
func SeedFlow(projectPath, reference string) (SeedResult, error) {
	root, err := projectDir(projectPath)
	if err != nil {
		return SeedResult{}, err
	}
	if reference == "" {
		reference = DefaultFlowReference
	}

	mode, err := projectMode(root)
	if err != nil {
		return SeedResult{}, err
	}

	// The embedded files are the whitelist: a reference that is not one of them
	// cannot name a path, so the target can never escape the workflow directory.
	template, err := fs.ReadFile(specflow.WorkflowTemplates(), templatePath(templateFor(reference, mode)))
	if err != nil {
		return SeedResult{}, fmt.Errorf("%w: %s", ErrTemplateNotFound, reference)
	}

	directory := filepath.Join(root, ".specflow", "workflows")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return SeedResult{}, fmt.Errorf("create %s: %w", directory, err)
	}

	path := filepath.Join(directory, reference+".yaml")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return SeedResult{}, fmt.Errorf("%w: %s", ErrFlowExists, path)
	}
	if err != nil {
		return SeedResult{}, fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(template); err != nil {
		file.Close()
		_ = os.Remove(path)
		return SeedResult{}, fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return SeedResult{}, fmt.Errorf("write %s: %w", path, err)
	}
	return SeedResult{Reference: reference, Path: path}, nil
}

// Templates lists the flow references this build can seed, without reading the
// disk, so a client can offer them.
func Templates() ([]string, error) {
	names, err := specflow.WorkflowTemplateNames()
	if err != nil {
		return nil, fmt.Errorf("list workflow templates: %w", err)
	}
	references := make([]string, 0, len(names))
	for _, name := range names {
		references = append(references, strings.TrimSuffix(name, ".yaml"))
	}
	sort.Strings(references)
	return references, nil
}

// templateFor maps a flow reference to the template that seeds it. A folder
// project's flow has no merge node, so it comes from the -folder variant under
// the same reference.
func templateFor(reference, mode string) string {
	if mode == workspace.ModeFolder {
		return reference + "-folder"
	}
	return reference
}

// projectMode reads a project's workspace mode. Opening a project does not
// block, so the read needs no request context.
func projectMode(root string) (string, error) {
	project, err := workspace.Open(context.Background(), root)
	if err != nil {
		return "", err
	}
	return project.Mode()
}

// templatePath is the reference's path inside the embedded template set.
func templatePath(reference string) string {
	return "workflows/" + reference + ".yaml"
}

// templateExists reports whether the executable carries a template.
func templateExists(reference string) bool {
	_, err := specflow.WorkflowTemplates().Open(templatePath(reference))
	return err == nil
}
