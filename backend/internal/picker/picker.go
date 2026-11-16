// Package picker opens the operating system's folder dialog.
//
// The browser cannot supply an absolute path: FileSystemDirectoryHandle exposes
// only a display name and webkitRelativePath is root-relative. The server runs
// on the same machine in the local deployment, so it opens the dialog and
// returns the path the project API needs. Each platform has a small
// implementation behind a build tag; a platform without one reports
// ErrUnsupported instead of guessing.
package picker

import (
	"context"
	"errors"
	"strings"
)

var (
	// ErrCancelled reports a dialog the user dismissed.
	ErrCancelled = errors.New("folder selection cancelled")
	// ErrUnsupported reports a platform with no folder-dialog implementation.
	ErrUnsupported = errors.New("no native folder dialog is available")
)

// Pick opens the native folder dialog and returns the chosen absolute path.
func Pick(ctx context.Context) (string, error) {
	return pick(ctx)
}

// cleanPath trims the platform output and a trailing separator. The root
// itself is preserved.
func cleanPath(output string) string {
	path := strings.TrimSpace(output)
	if path == "/" {
		return path
	}
	return strings.TrimRight(path, "/\\")
}
