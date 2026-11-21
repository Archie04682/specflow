package onboarding

import (
	"context"
	"errors"
	"fmt"

	"example.com/specflow/backend/internal/workspace"
)

// SetupResult is the recorded workspace mode and the project's git state after
// setup, so a client can render the next gap without a second read.
type SetupResult struct {
	VCS        string `json:"vcs"`
	Repository bool   `json:"repository"`
	Branch     string `json:"branch,omitempty"`
	HasCommit  bool   `json:"hasCommit"`
}

// Setup records a project's workspace mode. Choosing git initialises a
// repository when the directory is not one; choosing folder writes the mode and
// creates nothing. It never seeds a flow, installs a skill, or starts a run.
func Setup(ctx context.Context, projectPath, mode string) (SetupResult, error) {
	if mode != workspace.ModeGit && mode != workspace.ModeFolder {
		return SetupResult{}, fmt.Errorf("unknown workspace mode %q: use %q or %q", mode, workspace.ModeGit, workspace.ModeFolder)
	}

	root, err := projectDir(projectPath)
	if err != nil {
		return SetupResult{}, err
	}
	project, err := workspace.Open(ctx, root)
	if err != nil {
		return SetupResult{}, err
	}

	current, err := project.Mode()
	if err != nil {
		return SetupResult{}, err
	}
	explicit, err := project.ModeIsExplicit()
	if err != nil {
		return SetupResult{}, err
	}
	// Choosing the default mode still records it: the choice then stops being
	// offered, which is what makes the setup read as one decision rather than a
	// permanent control.
	if current != mode || !explicit {
		if err := project.SetMode(mode); err != nil {
			return SetupResult{}, err
		}
	}

	if mode == workspace.ModeGit {
		if _, err := InitRepository(ctx, root); err != nil && !errors.Is(err, ErrAlreadyRepository) {
			return SetupResult{}, err
		}
	}

	status, err := project.GitStatus(ctx)
	if err != nil {
		return SetupResult{}, err
	}
	return SetupResult{
		VCS:        status.VCS,
		Repository: status.Repository,
		Branch:     status.Branch,
		HasCommit:  status.HasCommit,
	}, nil
}
