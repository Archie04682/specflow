package api

import (
	"errors"
	"net/http"

	"example.com/specflow/backend/internal/orchestrator"
)

// workflowSource opens one workflow for editing: its resolved path, exact
// source, content hash, template flag, and parsed draft.
func workflowSource(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project query parameter is required"})
		return
	}

	source, err := orchestrator.ReadWorkflowSource(r.Context(), project, r.PathValue("reference"))
	if err != nil {
		// A reference or project the client supplied is a bad request; the
		// only dedicated outcome is the workflow being absent.
		status := http.StatusBadRequest
		if errors.Is(err, orchestrator.ErrWorkflowNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, source)
}

// workflowSourceSave validates a draft and writes it to the project copy. It
// reports structural problems as 400 with their list and a stale base hash as
// 409 with the file's current hash.
func workflowSourceSave(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project query parameter is required"})
		return
	}

	var request struct {
		Draft    orchestrator.WorkflowDraft `json:"draft"`
		BaseHash string                     `json:"baseHash"`
	}
	if !decodeOrFail(w, r, &request) {
		return
	}

	reference := r.PathValue("reference")
	path, hash, err := orchestrator.SaveWorkflowSource(r.Context(), project, reference, request.Draft, request.BaseHash)
	if err != nil {
		writeWorkflowSaveError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "hash": hash})
}

// writeWorkflowSaveError maps the save's typed outcomes to statuses.
func writeWorkflowSaveError(w http.ResponseWriter, err error) {
	var conflict *orchestrator.WorkflowConflictError
	var validation *orchestrator.ValidationError

	switch {
	case errors.Is(err, orchestrator.ErrInvalidWorkflowReference):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.As(err, &validation):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":    validation.Error(),
			"problems": validation.Problems,
		})
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": conflict.Error(),
			"hash":  conflict.Hash,
		})
	case errors.Is(err, orchestrator.ErrWorkflowProject):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}
