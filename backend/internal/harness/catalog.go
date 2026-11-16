package harness

import "context"

// ModelOption is one selectable model a harness reports. ID is the value the
// workflow stores (the same spelling the harness accepts on request); the
// remaining fields are display and capability metadata.
type ModelOption struct {
	ID             string   `json:"id"`
	Name           string   `json:"name,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Reasoning      bool     `json:"reasoning"`
	ThinkingLevels []string `json:"thinkingLevels,omitempty"`
}

// ToolOption is one tool name a workflow may select for a harness.
type ToolOption struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Catalog is the optional set of choices a harness exposes to the editor.
// ThinkingLevels is the harness-wide fallback; a model may narrow it through
// ModelOption.ThinkingLevels.
type Catalog struct {
	Models         []ModelOption `json:"models,omitempty"`
	ThinkingLevels []string      `json:"thinkingLevels,omitempty"`
	Tools          []ToolOption  `json:"tools,omitempty"`
}

// CatalogProvider is implemented by adapters that can describe their choices
// without executing a node. The registry and API treat it as optional so an
// adapter that only runs work keeps compiling and keeps the contract small.
type CatalogProvider interface {
	Catalog(ctx context.Context) (Catalog, error)
}
