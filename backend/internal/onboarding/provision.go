package onboarding

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

// SkillsRef is the pinned source of the OpenSpec skills. It tracks the
// `openspec` CLI version the shipped templates were written against, so the
// instructions a flow receives and the binary its gates call stay in step.
const SkillsRef = "Fission-AI/OpenSpec@v1.16.1"

// CommandRunner runs one provisioning command. It is injectable so tests never
// start a process and so the online path is observable without a network.
type CommandRunner func(ctx context.Context, dir string, env []string, name string, args ...string) (output string, exitCode int, err error)

// ExecRunner is the production runner.
func ExecRunner(ctx context.Context, dir string, env []string, name string, args ...string) (string, int, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), env...)
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return string(output), exitError.ExitCode(), err
	}
	return string(output), -1, err
}

// networkAllowed reports whether a network-capable action may run. Every such
// action consults this one predicate, so a set local source or offline mode
// makes the online branch unreachable rather than merely unlikely.
func networkAllowed(options Options) bool {
	return !options.Offline && options.SkillsSource == ""
}
