package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/orchestrator"
)

type runHandlers struct {
	engine *orchestrator.Engine
}

func registerRunRoutes(mux *http.ServeMux, engine *orchestrator.Engine) {
	handlers := &runHandlers{engine: engine}

	mux.Handle("POST /api/runs", http.HandlerFunc(handlers.create))
	mux.Handle("GET /api/runs", http.HandlerFunc(handlers.list))
	mux.Handle("GET /api/runs/{id}", http.HandlerFunc(handlers.get))
	mux.Handle("GET /api/runs/{id}/events", http.HandlerFunc(handlers.events))
	mux.Handle("POST /api/runs/{id}/answers", http.HandlerFunc(handlers.answer))
	mux.Handle("POST /api/runs/{id}/injections", http.HandlerFunc(handlers.inject))
	mux.Handle("POST /api/runs/{id}/cancel", http.HandlerFunc(handlers.cancel))
	mux.Handle("POST /api/runs/{id}/approve", http.HandlerFunc(handlers.approve))
	mux.Handle("POST /api/runs/{id}/retry", http.HandlerFunc(handlers.retry))
	mux.Handle("POST /api/runs/{id}/resume", http.HandlerFunc(handlers.resume))
	mux.Handle("DELETE /api/runs/{id}", http.HandlerFunc(handlers.discard))
}

func (h *runHandlers) create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Project   string `json:"project"`
		Workflow  string `json:"workflow"`
		Objective string `json:"objective"`
	}
	if !decodeOrFail(w, r, &request) {
		return
	}

	run, err := h.engine.Create(r.Context(), orchestrator.CreateRequest{
		Project:   request.Project,
		Workflow:  request.Workflow,
		Objective: request.Objective,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.engine.Start(r.Context(), request.Project, run.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "run": run})
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (h *runHandlers) list(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project query parameter is required"})
		return
	}

	runs, err := h.engine.List(r.Context(), project)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if runs == nil {
		runs = []*orchestrator.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *runHandlers) get(w http.ResponseWriter, r *http.Request) {
	project, runID, ok := runTarget(w, r)
	if !ok {
		return
	}

	run, err := h.engine.Get(r.Context(), project, runID)
	if err != nil {
		writeRunError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *runHandlers) answer(w http.ResponseWriter, r *http.Request) {
	var request struct {
		QuestionID string `json:"questionId"`
		Value      string `json:"value"`
		Confirmed  *bool  `json:"confirmed"`
		Cancelled  bool   `json:"cancelled"`
	}
	if !decodeOrFail(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.QuestionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "questionId is required"})
		return
	}

	answer := harness.Answer{
		ID:        request.QuestionID,
		Value:     request.Value,
		Confirmed: request.Confirmed,
		Cancelled: request.Cancelled,
	}
	h.runAction(w, r, func(project, runID string) (any, error) {
		return nil, h.engine.Answer(r.Context(), project, runID, request.QuestionID, answer)
	})
}

func (h *runHandlers) inject(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Text string `json:"text"`
	}
	if !decodeOrFail(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}
	if len(request.Text) > 64*1024 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text exceeds 64 KiB"})
		return
	}

	h.runAction(w, r, func(project, runID string) (any, error) {
		id, err := h.engine.Inject(r.Context(), project, runID, request.Text)
		if err != nil {
			return nil, err
		}
		return map[string]string{"id": id, "status": "queued"}, nil
	})
}

func (h *runHandlers) cancel(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, func(project, runID string) (any, error) {
		return map[string]string{"status": "cancelled"}, h.engine.Cancel(r.Context(), project, runID)
	})
}

func (h *runHandlers) approve(w http.ResponseWriter, r *http.Request) {
	var request struct {
		NodeID   string `json:"nodeId"`
		Decision string `json:"decision"`
		Attempt  int    `json:"attempt"`
		Approved *bool  `json:"approved"`
	}
	if !decodeOrFail(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.NodeID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nodeId is required"})
		return
	}
	if request.Attempt < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "attempt must not be negative"})
		return
	}

	approved, ok := approvalDecision(request.Decision, request.Approved)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `decision must be "approve" or "reject"`})
		return
	}

	h.runAction(w, r, func(project, runID string) (any, error) {
		return nil, h.engine.Approve(r.Context(), project, runID, orchestrator.ApprovalDecision{
			NodeID:   request.NodeID,
			Attempt:  request.Attempt,
			Approved: approved,
		})
	})
}

// approvalDecision accepts the explicit decision field and the older approved
// boolean, but never guesses when neither is present.
func approvalDecision(decision string, approved *bool) (bool, bool) {
	switch decision {
	case "approve":
		return true, true
	case "reject":
		return false, true
	case "":
		if approved != nil {
			return *approved, true
		}
		return false, false
	default:
		return false, false
	}
}

func (h *runHandlers) retry(w http.ResponseWriter, r *http.Request) {
	var request struct {
		NodeID string `json:"nodeId"`
		Mode   string `json:"mode"`
	}
	if !decodeOrFail(w, r, &request) {
		return
	}
	if request.Mode == "" {
		request.Mode = orchestrator.RetryForward
	}

	h.runAction(w, r, func(project, runID string) (any, error) {
		return map[string]string{"status": "running"}, h.engine.Retry(r.Context(), project, runID, request.NodeID, request.Mode)
	})
}

func (h *runHandlers) resume(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, func(project, runID string) (any, error) {
		return map[string]string{"status": "running"}, h.engine.Resume(r.Context(), project, runID)
	})
}

func (h *runHandlers) discard(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, func(project, runID string) (any, error) {
		return map[string]string{"status": "discarded"}, h.engine.Discard(r.Context(), project, runID)
	})
}

// runAction resolves the project and run, performs the action, and writes the
// standard JSON response. It is the shared shape behind every run mutation, so
// the handlers only carry their own request and payload.
func (h *runHandlers) runAction(w http.ResponseWriter, r *http.Request, action func(project, runID string) (any, error)) {
	project, runID, ok := runTarget(w, r)
	if !ok {
		return
	}

	payload, err := action(project, runID)
	if err != nil {
		writeRunError(w, err)
		return
	}
	if payload == nil {
		payload = map[string]string{"status": "ok"}
	}
	writeJSON(w, http.StatusOK, payload)
}

func runTarget(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project query parameter is required"})
		return "", "", false
	}

	runID := r.PathValue("id")
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
		return "", "", false
	}
	return project, runID, true
}

func writeRunError(w http.ResponseWriter, err error) {
	status := http.StatusConflict
	switch {
	case errors.Is(err, orchestrator.ErrRunNotFound):
		status = http.StatusNotFound
	case errors.Is(err, orchestrator.ErrInteractionResolved):
		status = http.StatusGone
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decodeOrFail(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := decodeJSON(r, target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return false
	}
	return true
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}
