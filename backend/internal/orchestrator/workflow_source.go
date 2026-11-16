package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"example.com/specflow/backend/internal/workspace"
)

var (
	// ErrWorkflowNotFound reports a reference that neither workflow directory has.
	ErrWorkflowNotFound = errors.New("workflow not found")

	// ErrInvalidWorkflowReference reports a reference that is not a bare
	// filename. A reference never reaches a path check; the pattern is the
	// whole gate.
	ErrInvalidWorkflowReference = errors.New("invalid workflow reference")

	// ErrWorkflowProject reports that the project itself could not be opened.
	// The project path is client state, so the API reports it as a bad request,
	// unlike a file failure inside an open project.
	ErrWorkflowProject = errors.New("workflow project cannot be opened")
)

// workflowReferencePattern is the only shape a reference may have: a bare
// filename, no separators, no parent segment, no leading dot.
var workflowReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// WorkflowConflictError reports a save whose base hash does not match the file
// it would overwrite. Hash is the current file's hash, so the editor can show
// what its stale draft is up against.
type WorkflowConflictError struct {
	Hash string
}

// Error implements error.
func (e *WorkflowConflictError) Error() string {
	return "workflow changed on disk"
}

// WorkflowSource is one workflow file as the editor opens it: where it came
// from, its exact bytes, a content hash, whether it is a template, and the
// parsed draft.
type WorkflowSource struct {
	Reference  string        `json:"reference"`
	Path       string        `json:"path"`
	Source     string        `json:"source"`
	Hash       string        `json:"hash"`
	IsTemplate bool          `json:"isTemplate"`
	Draft      WorkflowDraft `json:"draft"`
}

// ReadWorkflowSource opens one workflow for editing. The reference must be a
// bare filename and is resolved only against the project's two workflow
// directories, project copy first. Unlike ResolveWorkflowPath it never consults
// the process working directory or accepts an arbitrary path.
func ReadWorkflowSource(ctx context.Context, projectRoot, reference string) (*WorkflowSource, error) {
	if !validWorkflowReference(reference) {
		return nil, ErrInvalidWorkflowReference
	}

	project, err := openWorkflowProject(ctx, projectRoot)
	if err != nil {
		return nil, err
	}

	candidates := []struct {
		path     string
		template bool
	}{
		{path: filepath.Join(project.Root, ".specflow", "workflows", reference+".yaml")},
		{path: filepath.Join(project.Root, "workflows", reference+".yaml"), template: true},
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read workflow %s: %w", candidate.path, err)
		}

		workflow, err := ParseWorkflow(data, candidate.path)
		if err != nil {
			return nil, err
		}
		return &WorkflowSource{
			Reference:  reference,
			Path:       candidate.path,
			Source:     string(data),
			Hash:       hashWorkflowSource(data),
			IsTemplate: candidate.template,
			Draft:      draftFromWorkflow(workflow),
		}, nil
	}

	return nil, fmt.Errorf("%w: %s", ErrWorkflowNotFound, reference)
}

// SaveWorkflowSource validates a draft and writes it canonically to the
// project's copy. A save only ever writes
// <root>/.specflow/workflows/<reference>.yaml; a workflow that exists only as
// a template is copied, never modified. The base hash is the optimistic
// concurrency precondition: it must match whatever file the editor opened, and
// an empty base hash is a create that refuses to clobber a file that appeared
// concurrently.
func SaveWorkflowSource(ctx context.Context, projectRoot, reference string, draft WorkflowDraft, baseHash string) (string, string, error) {
	if !validWorkflowReference(reference) {
		return "", "", ErrInvalidWorkflowReference
	}

	project, err := openWorkflowProject(ctx, projectRoot)
	if err != nil {
		return "", "", err
	}

	target := filepath.Join(project.Root, ".specflow", "workflows", reference+".yaml")
	existing, err := os.ReadFile(target)
	switch {
	case err == nil:
		currentHash := hashWorkflowSource(existing)
		if baseHash == "" || baseHash != currentHash {
			return "", "", &WorkflowConflictError{Hash: currentHash}
		}
	case errors.Is(err, os.ErrNotExist):
		if baseHash != "" {
			if err := authorizeTemplateCopy(project.Root, reference, baseHash); err != nil {
				return "", "", err
			}
		}
	default:
		return "", "", fmt.Errorf("read workflow %s: %w", target, err)
	}

	workflow := workflowFromDraft(draft)
	if err := ValidateWorkflow(workflow); err != nil {
		return "", "", err
	}

	data, err := MarshalWorkflowYAML(draft)
	if err != nil {
		return "", "", err
	}
	if err := writeWorkflowAtomic(target, data); err != nil {
		return "", "", err
	}

	return target, hashWorkflowSource(data), nil
}

// authorizeTemplateCopy accepts a create whose base hash matches the template
// the editor opened. Any other state is a conflict: with the template's current
// hash when it exists, or with an empty hash when the file the client thought
// it opened is gone.
func authorizeTemplateCopy(projectRoot, reference, baseHash string) error {
	template := filepath.Join(projectRoot, "workflows", reference+".yaml")
	data, err := os.ReadFile(template)
	switch {
	case err == nil:
		currentHash := hashWorkflowSource(data)
		if currentHash != baseHash {
			return &WorkflowConflictError{Hash: currentHash}
		}
		return nil
	case errors.Is(err, os.ErrNotExist):
		return &WorkflowConflictError{}
	default:
		return fmt.Errorf("read workflow template %s: %w", template, err)
	}
}

// validWorkflowReference reports whether a reference is a bare filename.
func validWorkflowReference(reference string) bool {
	return workflowReferencePattern.MatchString(reference)
}

// openWorkflowProject resolves the project root and marks a failure as a bad
// request rather than an internal error.
func openWorkflowProject(ctx context.Context, projectRoot string) (*workspace.Project, error) {
	project, err := workspace.Open(ctx, projectRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWorkflowProject, err)
	}
	return project, nil
}

// hashWorkflowSource is the lowercase hex SHA-256 of the exact file bytes.
func hashWorkflowSource(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// writeWorkflowAtomic commits data to path through a temporary file in the same
// directory and a rename. A failed write removes the temporary and leaves the
// previous file byte-identical, and a crash cannot leave a half-written
// workflow.
func writeWorkflowAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create workflow directory %s: %w", directory, err)
	}

	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if committed {
			return
		}
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("set permissions on %s: %w", temporaryPath, err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write workflow %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write workflow %s: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("commit workflow %s: %w", path, err)
	}
	committed = true
	return nil
}
