//go:build windows

package picker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

const chooserScript = `Add-Type -AssemblyName System.Windows.Forms | Out-Null
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Select a SpecFlow project'
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.SelectedPath } else { exit 1 }`

func pick(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(
		ctx,
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		chooserScript,
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
