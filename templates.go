// Package specflow holds the assets the executable carries besides the web UI.
//
// The package lives at the module root because //go:embed patterns resolve
// against the package directory, and the workflow templates live in workflows/.
// Keeping the embed here means there is one copy of every template: no build
// step to generate it and no drift to test for.
package specflow

import (
	"embed"
	"io/fs"
)

//go:embed workflows/*.yaml
var workflowTemplates embed.FS

// WorkflowTemplates returns the workflow templates compiled into the
// executable, rooted so a reference names the file directly: "standard-
// feature.yaml".
func WorkflowTemplates() fs.FS {
	return workflowTemplates
}

// WorkflowTemplateNames lists the embedded template file names.
func WorkflowTemplateNames() ([]string, error) {
	entries, err := fs.ReadDir(workflowTemplates, "workflows")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}
