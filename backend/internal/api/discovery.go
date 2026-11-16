package api

import (
	"errors"
	"net/http"

	"example.com/specflow/backend/internal/orchestrator"
)

// registerDiscoveryRoutes wires the project-facing discovery endpoints: the
// workflow catalog and single-workflow source, a run's pinned graph, and the
// folder picker. The workflow source routes also accept an editor save.
func registerDiscoveryRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/workflows", http.HandlerFunc(workflowCatalog))
	mux.Handle("GET /api/skills", http.HandlerFunc(skillCatalog))
	mux.Handle("GET /api/workflows/{reference}", http.HandlerFunc(workflowSource))
	mux.Handle("PUT /api/workflows/{reference}", http.HandlerFunc(workflowSourceSave))
	mux.Handle("GET /api/runs/{id}/graph", http.HandlerFunc(runGraph))
	mux.Handle("POST /api/project/pick", http.HandlerFunc(projectPicker))
}

// workflowCatalog lists the project's workflows. An unusable project is a bad
// request, because the path is client state; a broken workflow file is not an
// error and is returned as an invalid entry instead.
func workflowCatalog(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project query parameter is required"})
		return
	}

	workflows, err := orchestrator.ReadWorkflowCatalog(r.Context(), project)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if workflows == nil {
		workflows = []orchestrator.WorkflowSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": workflows})
}

// runGraph returns the topology of the definition the run was pinned to. An
// unknown run is a 404; a definition that is missing, unreadable, or
// unparseable is a server-side failure because the run record points at it.
func runGraph(w http.ResponseWriter, r *http.Request) {
	project, runID, ok := runTarget(w, r)
	if !ok {
		return
	}

	graph, err := orchestrator.ReadRunGraph(r.Context(), project, runID)
	if err != nil {
		if errors.Is(err, orchestrator.ErrRunNotFound) {
			writeRunError(w, err)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, graph)
}
