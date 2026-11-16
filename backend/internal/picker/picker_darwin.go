//go:build darwin

package picker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// chooserScript opens the standard folder chooser. The prompt is fixed text,
// never user input.
const chooserScript = `POSIX path of (choose folder with prompt "Select a SpecFlow project")`

func pick(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "osascript", "-e", chooserScript).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && isCancellation(exit.Stderr) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("open folder dialog: %w", err)
	}

	path := cleanPath(string(output))
	if path == "" {
		return "", ErrCancelled
	}
	return path, nil
}

// isCancellation recognizes AppleScript's dismissal wording. A timeout or a
// missing desktop session is a failure, not a cancellation.
func isCancellation(stderr []byte) bool {
	lower := strings.ToLower(string(stderr))
	return strings.Contains(lower, "user canceled") ||
		strings.Contains(lower, "user cancelled")
}
