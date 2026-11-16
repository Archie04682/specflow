package api

import (
	"context"
	"net/http"
	"time"

	"example.com/specflow/backend/internal/harness"
)

// harnessProbeTimeout bounds one harness probe. A harness that cannot answer
// in time is reported as not ready instead of failing the request: listing
// harnesses must never be the reason the UI cannot load.
const harnessProbeTimeout = 2 * time.Second

// harnessJSON is the wire shape of one installed harness.
type harnessJSON struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	Version        string                `json:"version"`
	Protocol       int                   `json:"protocol"`
	Ready          bool                  `json:"ready"`
	ReadyError     string                `json:"readyError,omitempty"`
	Capabilities   harness.Capabilities  `json:"capabilities"`
	Models         []harness.ModelOption `json:"models,omitempty"`
	ThinkingLevels []string              `json:"thinkingLevels,omitempty"`
	Tools          []harness.ToolOption  `json:"tools,omitempty"`
	CatalogError   string                `json:"catalogError,omitempty"`
}

// harnessesHandler lists the installed harnesses with their effective
// capabilities. Capabilities come from Probe, so a component that failed to
// initialize is reported honestly rather than only on first use.
func harnessesHandler(registry *harness.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		listed := make([]harnessJSON, 0)

		for _, adapter := range registry.All() {
			descriptor := adapter.Descriptor()
			entry := harnessJSON{
				ID:           descriptor.ID,
				Name:         descriptor.Name,
				Version:      descriptor.Version,
				Protocol:     descriptor.Protocol,
				Capabilities: adapter.Capabilities(),
			}

			ctx, cancel := context.WithTimeout(r.Context(), harnessProbeTimeout)
			probe, err := adapter.Probe(ctx)
			cancel()

			if err != nil {
				entry.Ready = false
				entry.ReadyError = err.Error()
				// A capability is true only when the feature works in this
				// environment; an unready harness supports nothing yet.
				entry.Capabilities = harness.Capabilities{}
			} else {
				entry.Ready = true
				entry.Capabilities = probe.Capabilities
			}

			// The catalog is optional: an adapter that only executes work
			// omits it, and a catalog read failure must not hide the
			// harness from the editor's fallback.
			if provider, ok := adapter.(harness.CatalogProvider); ok {
				catalogCtx, cancelCatalog := context.WithTimeout(r.Context(), harnessProbeTimeout)
				catalog, catalogErr := provider.Catalog(catalogCtx)
				cancelCatalog()
				if catalogErr != nil {
					entry.CatalogError = catalogErr.Error()
				} else {
					entry.Models = catalog.Models
					entry.ThinkingLevels = catalog.ThinkingLevels
					entry.Tools = catalog.Tools
				}
			}

			listed = append(listed, entry)
		}

		writeJSON(w, http.StatusOK, map[string]any{"harnesses": listed})
	}
}
