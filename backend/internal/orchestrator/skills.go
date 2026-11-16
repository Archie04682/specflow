package orchestrator

import (
	"context"
	"fmt"

	"example.com/specflow/backend/internal/openspec"
	"example.com/specflow/backend/internal/workspace"
)

// ReadSkillCatalog lists the skills installed for a project. It opens the
// project only to read its configured skill roots; the skills themselves are
// never copied or loaded here.
func ReadSkillCatalog(ctx context.Context, projectPath string) ([]openspec.SkillInfo, error) {
	project, err := workspace.Open(ctx, projectPath)
	if err != nil {
		return nil, err
	}

	sources, err := project.SkillSources()
	if err != nil {
		return nil, fmt.Errorf("read skill roots: %w", err)
	}

	roots := make([]openspec.SkillRoot, 0, len(sources))
	for _, source := range sources {
		roots = append(roots, openspec.SkillRoot{Label: source.Label, Path: source.Path})
	}
	return openspec.ListSkills(roots), nil
}
