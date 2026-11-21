package api

import (
	"errors"
	"net/http"

	"example.com/specflow/backend/internal/onboarding"
)

// registerOnboardingRoutes wires the readiness read and the provisioning
// actions. The options carry the server's offline mode and skills source.
func registerOnboardingRoutes(mux *http.ServeMux, options onboarding.Options) {
	mux.Handle("GET /api/project/readiness", projectReadiness(options))
	mux.Handle("POST /api/project/flow", projectSeedFlow())
	mux.Handle("POST /api/project/skills", projectInstallSkills(options))
	mux.Handle("GET /api/project/repository", projectRepositoryStatus())
	mux.Handle("POST /api/project/repository", projectInitRepository())
	mux.Handle("POST /api/project/setup", projectSetup())
	mux.Handle("POST /api/project/commit", projectCommit())
	mux.Handle("POST /api/project/ignore", projectIgnore())
	mux.Handle("GET /api/templates", workflowTemplates())
}

// workflowTemplates lists the flows this build can seed.
func workflowTemplates() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		references, err := onboarding.Templates()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"templates": references})
	})
}

// projectReadiness reports what a project is missing without changing it.
func projectReadiness(options onboarding.Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		if project == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project query parameter is required"})
			return
		}

		readiness, err := onboarding.Read(r.Context(), project, options)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, readiness)
	})
}

// projectSeedFlow writes the embedded default flow into a project.
func projectSeedFlow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Project   string `json:"project"`
			Reference string `json:"reference"`
		}
		if !decodeOrFail(w, r, &request) {
			return
		}

		result, err := onboarding.SeedFlow(request.Project, request.Reference)
		if err != nil {
			writeJSON(w, seedStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// seedStatus maps a seed refusal: an existing file is a conflict, everything
// else the client can fix is a bad request.
func seedStatus(err error) int {
	if errors.Is(err, onboarding.ErrFlowExists) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

// projectRepositoryStatus reports whether a directory is a repository, which is
// what the project tree needs before it can show chats and flows.
func projectRepositoryStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		if project == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project query parameter is required"})
			return
		}

		status, err := onboarding.ReadRepository(r.Context(), project)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
}

// projectCommit commits the project's changes on request. Secret-looking paths
// are refused with their list so the client can offer to ignore them.
func projectCommit() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Project string `json:"project"`
			Message string `json:"message"`
		}
		if !decodeOrFail(w, r, &request) {
			return
		}

		result, err := onboarding.Commit(r.Context(), request.Project, request.Message)
		if err != nil {
			var secrets *onboarding.SecretPathsError
			if errors.As(err, &secrets) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error":   err.Error(),
					"secrets": secrets.Paths,
				})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// projectIgnore appends paths to the project's .gitignore on request.
func projectIgnore() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Project string   `json:"project"`
			Paths   []string `json:"paths"`
		}
		if !decodeOrFail(w, r, &request) {
			return
		}

		result, err := onboarding.Ignore(r.Context(), request.Project, request.Paths)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// projectSetup records a project's workspace mode, initialising a repository
// for git mode and creating nothing else.
func projectSetup() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Project string `json:"project"`
			VCS     string `json:"vcs"`
		}
		if !decodeOrFail(w, r, &request) {
			return
		}

		result, err := onboarding.Setup(r.Context(), request.Project, request.VCS)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// projectInitRepository initialises a git repository in the project directory.
func projectInitRepository() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Project string `json:"project"`
		}
		if !decodeOrFail(w, r, &request) {
			return
		}

		result, err := onboarding.InitRepository(r.Context(), request.Project)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, onboarding.ErrAlreadyRepository) {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// projectInstallSkills installs the skills the project's flows bind.
func projectInstallSkills(options onboarding.Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Project string `json:"project"`
			Replace bool   `json:"replace"`
		}
		if !decodeOrFail(w, r, &request) {
			return
		}

		report, err := onboarding.InstallSkills(r.Context(), request.Project, options, request.Replace, nil)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
}
