// Package api wires the HTTP surface of the SpecFlow server: the JSON API
// under /api/ and the embedded web UI everywhere else.
package api

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/orchestrator"
)

// NewRouter returns the handler that serves the API and the embedded UI, with
// no harness adapters installed and no run engine.
func NewRouter(staticFS fs.FS) http.Handler {
	return NewRouterWithHarnesses(staticFS, harness.NewRegistry())
}

// NewRouterWithHarnesses returns the handler with the given harness registry
// backing the harness discovery endpoint.
func NewRouterWithHarnesses(staticFS fs.FS, registry *harness.Registry) http.Handler {
	return NewRouterWithDeps(staticFS, registry, nil)
}

// NewRouterWithDeps returns the handler with harness discovery and, when an
// engine is supplied, the run endpoints. It reports no build identity; callers
// that can say which build is running use NewRouterWithServer.
func NewRouterWithDeps(staticFS fs.FS, registry *harness.Registry, engine *orchestrator.Engine) http.Handler {
	return NewRouterWithServer(staticFS, registry, engine, ServerIdentity{})
}

// NewRouterWithServer returns the handler with the running server's identity
// for GET /api/server, alongside the API and the embedded UI.
func NewRouterWithServer(staticFS fs.FS, registry *harness.Registry, engine *orchestrator.Engine, server ServerIdentity) http.Handler {
	if registry == nil {
		registry = harness.NewRegistry()
	}

	mux := http.NewServeMux()

	mux.Handle("GET /api/health", http.HandlerFunc(health))
	mux.Handle("GET /api/server", serverHandler(server))
	mux.Handle("GET /api/harnesses", harnessesHandler(registry))
	registerDiscoveryRoutes(mux)
	if engine != nil {
		registerRunRoutes(mux, engine)
	}
	mux.Handle("/api/", http.HandlerFunc(apiNotFound))

	mux.Handle("/", spaHandler{staticFS: staticFS})

	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func apiNotFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
