package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/specflow/backend/internal/openspec"
)

// PostconditionEnv is what a postcondition is evaluated against.
type PostconditionEnv struct {
	NodeID string
	Root   string
	// Workdir is where the node ran; relative file paths resolve here.
	Workdir  string
	Change   string
	ExitCode int
	// Baseline is the change list recorded before the run, so new_change can
	// still be answered after a restart or a resume.
	Baseline []string
	Context  RunContext
}

// PostconditionFailure reports that a node finished but the repository
// disagrees. It is distinct from a harness failure: the adapter completed.
type PostconditionFailure struct {
	NodeID      string
	Requirement string
	Detail      string
}

// Error implements error.
func (e *PostconditionFailure) Error() string {
	return fmt.Sprintf("postcondition failed for node %q: %s", e.NodeID, e.Detail)
}

// EvaluatePostcondition checks one node's postcondition. It returns the change
// a `new_change` postcondition resolved, if any.
func EvaluatePostcondition(postcondition *Postcondition, env PostconditionEnv) (string, error) {
	if postcondition == nil || postcondition.Type == "" || postcondition.Type == "none" {
		return "", nil
	}

	switch postcondition.Type {
	case "file_exists":
		path := postcondition.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(env.Workdir, path)
		}
		if _, err := os.Stat(path); err != nil {
			return "", failPostcondition(env, postcondition, fmt.Sprintf("%s does not exist", postcondition.Path))
		}
		return "", nil

	case "exit_zero":
		if env.ExitCode != 0 {
			return "", failPostcondition(env, postcondition, fmt.Sprintf("command exited with %d", env.ExitCode))
		}
		return "", nil

	case "new_change":
		return evaluateNewChange(env, postcondition)

	case "tasks_complete":
		change, err := postconditionChange(postcondition, env)
		if err != nil {
			return "", err
		}
		unchecked, err := openspec.UncheckedTasks(env.Root, change)
		if err != nil {
			return "", failPostcondition(env, postcondition, err.Error())
		}
		if len(unchecked) > 0 {
			return "", failPostcondition(env, postcondition,
				fmt.Sprintf("%d unfinished task(s): %s", len(unchecked), strings.Join(unchecked, "; ")))
		}
		return "", nil

	default:
		return "", failPostcondition(env, postcondition, "unknown postcondition type "+postcondition.Type)
	}
}

func evaluateNewChange(env PostconditionEnv, postcondition *Postcondition) (string, error) {
	current, err := openspec.ListChanges(env.Root)
	if err != nil {
		return "", failPostcondition(env, postcondition, err.Error())
	}

	var added []string
	for _, change := range current {
		if !contains(env.Baseline, change) {
			added = append(added, change)
		}
	}

	switch len(added) {
	case 0:
		return "", failPostcondition(env, postcondition, "no new change appeared under openspec/changes")
	case 1:
		return added[0], nil
	default:
		return "", failPostcondition(env, postcondition,
			"several new changes appeared: "+strings.Join(added, ", "))
	}
}

func postconditionChange(postcondition *Postcondition, env PostconditionEnv) (string, error) {
	if postcondition.Change == "" {
		if env.Change == "" {
			return "", failPostcondition(env, postcondition, "no change is resolved for this run")
		}
		return env.Change, nil
	}

	rendered, err := RenderPrompt(postcondition.Change, env.Context)
	if err != nil {
		return "", failPostcondition(env, postcondition, err.Error())
	}
	if rendered == "" {
		return "", failPostcondition(env, postcondition, "no change is resolved for this run")
	}
	return rendered, nil
}

func failPostcondition(env PostconditionEnv, postcondition *Postcondition, detail string) error {
	return &PostconditionFailure{
		NodeID:      env.NodeID,
		Requirement: postcondition.Requirement(),
		Detail:      detail,
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
