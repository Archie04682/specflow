package pi

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// bridgeSource is the Pi extension that provides the ask-user tool. It is
// compiled into the binary so the product stays a single executable; Pi itself
// remains a separately installed harness.
//
//go:embed bridge/specflow-bridge.ts
var bridgeSource []byte

// BridgeMarker is the notification the extension emits once per session. The
// adapter uses it to confirm the extension loaded; it is swallowed rather than
// shown to the user.
const BridgeMarker = "specflow-bridge:ready"

// materializeBridge writes the embedded extension to a content-addressed path
// and returns it.
//
// The path includes a hash of the source, so an upgraded binary never reuses a
// stale extension. cacheRoot empty means the platform cache directory. When
// that is unavailable or unwritable, a private temporary directory is used:
// failing to ask a question must not be caused by a cache permission.
func materializeBridge(cacheRoot string) (string, error) {
	digest := sha256.Sum256(bridgeSource)
	name := "specflow-bridge-" + hex.EncodeToString(digest[:8]) + ".ts"

	if cacheRoot == "" {
		root, err := os.UserCacheDir()
		if err != nil {
			return writeBridgeTemp(name)
		}
		cacheRoot = filepath.Join(root, "specflow")
	}

	dir := filepath.Join(cacheRoot, "pi-bridge")
	path := filepath.Join(dir, name)

	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() == int64(len(bridgeSource)) {
		return path, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return writeBridgeTemp(name)
	}

	tmp, err := os.CreateTemp(dir, ".specflow-bridge-*")
	if err != nil {
		return writeBridgeTemp(name)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(bridgeSource); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("write bridge extension: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("restrict bridge extension: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("close bridge extension: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("install bridge extension: %w", err)
	}

	return path, nil
}

func writeBridgeTemp(name string) (string, error) {
	dir, err := os.MkdirTemp("", "specflow-pi-bridge-")
	if err != nil {
		return "", fmt.Errorf("create bridge directory: %w", err)
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, bridgeSource, 0o600); err != nil {
		return "", fmt.Errorf("write bridge extension: %w", err)
	}
	return path, nil
}
