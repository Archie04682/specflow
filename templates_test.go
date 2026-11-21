package specflow

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestEmbeddedTemplatesMatchTheCheckout(t *testing.T) {
	embedded, err := WorkflowTemplateNames()
	if err != nil {
		t.Fatalf("list embedded templates: %v", err)
	}
	if len(embedded) == 0 {
		t.Fatal("no workflow templates are embedded")
	}

	entries, err := os.ReadDir("workflows")
	if err != nil {
		t.Fatalf("read workflows: %v", err)
	}
	onDisk := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		onDisk = append(onDisk, entry.Name())
	}

	sort.Strings(embedded)
	sort.Strings(onDisk)
	if !reflect.DeepEqual(embedded, onDisk) {
		t.Fatalf("embedded = %v, on disk = %v", embedded, onDisk)
	}
}

func TestEmbeddedTemplateIsByteIdentical(t *testing.T) {
	const name = "workflows/standard-feature.yaml"

	embedded, err := fs.ReadFile(WorkflowTemplates(), name)
	if err != nil {
		t.Fatalf("read embedded template: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join("workflows", "standard-feature.yaml"))
	if err != nil {
		t.Fatalf("read checkout template: %v", err)
	}
	if string(embedded) != string(onDisk) {
		t.Fatal("the embedded template differs from the checkout's copy")
	}
}
