package api

import (
	"net/http"

	"example.com/specflow/backend/internal/openspec"
	"example.com/specflow/backend/internal/orchestrator"
)

// skillCatalog lists the skills installed in the project's configured roots.
// The catalog is discovery only: a skill is still resolved and passed by
// absolute path when a run is created.
func skillCatalog(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project query parameter is required"})
		return
	}

	skills, err := orchestrator.ReadSkillCatalog(r.Context(), project)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if skills == nil {
		skills = []openspec.SkillInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}
