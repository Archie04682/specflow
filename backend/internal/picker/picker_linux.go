//go:build linux

package picker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

func pick(ctx context.Context) (string, error) {
	binary, err := exec.LookPath("zenity")
	if err != nil {
		return "", ErrUnsupported
	}

	output, err := exec.CommandContext(
		ctx,
		binary,
		"--file-selection",
		"--directory",
		"--title=Select a SpecFlow project",
	).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
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
