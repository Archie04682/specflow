package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBridgeSourceIsDependencyFree(t *testing.T) {
	source := string(bridgeSource)

	if !strings.Contains(source, `import type { ExtensionAPI }`) {
		t.Fatal("bridge does not use a type-only import; loading it may need a package install")
	}
	if strings.Contains(source, "from \"typebox\"") || strings.Contains(source, "require(") {
		t.Fatal("bridge has a runtime dependency")
	}
	if strings.Contains(source, "registerTool") == false {
		t.Fatal("bridge does not register the ask tool")
	}
	if !strings.Contains(source, BridgeMarker) {
		t.Fatalf("bridge does not emit the %q marker", BridgeMarker)
	}
	if !strings.Contains(source, "specflow_ask") {
		t.Fatal("bridge does not define the specflow_ask tool")
	}
}

func TestMaterializeBridgeIsContentAddressedAndRestricted(t *testing.T) {
	cacheRoot := t.TempDir()

	first, err := materializeBridge(cacheRoot)
	if err != nil {
		t.Fatalf("materializeBridge: %v", err)
	}
	second, err := materializeBridge(cacheRoot)
	if err != nil {
		t.Fatalf("materializeBridge (second call): %v", err)
	}

	if first != second {
		t.Fatalf("paths differ between calls: %q and %q", first, second)
	}
	if !strings.HasSuffix(first, ".ts") {
		t.Fatalf("path %q does not end in .ts", first)
	}
	if !strings.HasPrefix(filepath.Base(first), "specflow-bridge-") {
		t.Fatalf("path %q is not content addressed", first)
	}

	written, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read materialized bridge: %v", err)
	}
	if string(written) != string(bridgeSource) {
		t.Fatal("materialized bridge differs from the embedded source")
	}

	info, err := os.Stat(first)
	if err != nil {
		t.Fatalf("stat bridge: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("bridge mode = %o, want 600", perm)
	}

	dirInfo, err := os.Stat(filepath.Dir(first))
	if err != nil {
		t.Fatalf("stat bridge directory: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("bridge directory mode = %o, want 700", perm)
	}
}

func TestMaterializeBridgeReplacesStaleFile(t *testing.T) {
	cacheRoot := t.TempDir()

	first, err := materializeBridge(cacheRoot)
	if err != nil {
		t.Fatalf("materializeBridge: %v", err)
	}
	if err := os.WriteFile(first, []byte("stale"), 0o600); err != nil {
		t.Fatalf("corrupt bridge: %v", err)
	}

	second, err := materializeBridge(cacheRoot)
	if err != nil {
		t.Fatalf("materializeBridge after corruption: %v", err)
	}
	if first != second {
		t.Fatalf("path changed: %q -> %q", first, second)
	}

	written, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("read bridge: %v", err)
	}
	if string(written) != string(bridgeSource) {
		t.Fatal("a stale bridge file was reused")
	}
}
