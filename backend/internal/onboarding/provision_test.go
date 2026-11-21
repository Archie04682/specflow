package onboarding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOnlineInstallBuildsTheExpectedCommand(t *testing.T) {
	project := newGitProject(t)

	var directory, name string
	var env, args []string
	runner := func(_ context.Context, dir string, commandEnv []string, command string, commandArgs ...string) (string, int, error) {
		directory, name, env, args = dir, command, commandEnv, commandArgs
		// Stand in for the CLI: a real run writes into .agents/skills.
		writeSkill(t, filepath.Join(dir, ".agents", "skills"), "openspec-explore", "Explore")
		return "installed 1 skill", 0, nil
	}

	report, err := InstallSkills(context.Background(), project, Options{}, false, runner)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if name != "npx" {
		t.Fatalf("command = %q, want npx", name)
	}
	if directory != project {
		t.Fatalf("working directory = %q", directory)
	}
	if !slices.Contains(args, SkillsRef) {
		t.Fatalf("args = %v, want the pinned source", args)
	}
	if !slices.Contains(args, "universal") {
		t.Fatalf("args = %v, want the shared agent target", args)
	}
	if !slices.Contains(args, "skills") || !slices.Contains(args, "add") {
		t.Fatalf("args = %v", args)
	}
	if !slices.Contains(env, "DISABLE_TELEMETRY=1") {
		t.Fatalf("env = %v, want telemetry disabled", env)
	}
	if !slices.Equal(report.Installed, []string{"openspec-explore"}) {
		t.Fatalf("installed = %v", report.Installed)
	}
	if report.ExitCode != 0 || !strings.Contains(report.Output, "installed 1 skill") {
		t.Fatalf("report = %+v", report)
	}
}

func TestOnlineInstallReportsAFailingCommand(t *testing.T) {
	project := newGitProject(t)
	runner := func(context.Context, string, []string, string, ...string) (string, int, error) {
		return "npm ERR! offline", 2, errors.New("exit status 2")
	}

	report, err := InstallSkills(context.Background(), project, Options{}, false, runner)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if report.ExitCode != 2 {
		t.Fatalf("exit code = %d", report.ExitCode)
	}
	if !strings.Contains(report.Output, "npm ERR! offline") {
		t.Fatalf("output = %q", report.Output)
	}
	if len(report.Installed) != 0 {
		t.Fatalf("a failing command reported installs: %v", report.Installed)
	}
}

func TestOfflineInstallRefusesBeforeRunning(t *testing.T) {
	project := newGitProject(t)
	called := false
	runner := func(context.Context, string, []string, string, ...string) (string, int, error) {
		called = true
		return "", 0, nil
	}

	_, err := InstallSkills(context.Background(), project, Options{Offline: true}, false, runner)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("install = %v, want ErrOffline", err)
	}
	if called {
		t.Fatal("offline mode built and ran a command")
	}
	if _, statErr := os.Stat(filepath.Join(project, ".agents")); !os.IsNotExist(statErr) {
		t.Fatal("a refused install wrote into the project")
	}
}

func TestALocalSourceMakesTheOnlineBranchUnreachable(t *testing.T) {
	project := newGitProject(t)
	source := t.TempDir()
	writeSkill(t, source, "alpha", "Alpha")

	runner := func(context.Context, string, []string, string, ...string) (string, int, error) {
		t.Fatal("the online branch ran with a local source configured")
		return "", 0, nil
	}

	report, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, runner)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Equal(report.Installed, []string{"alpha"}) {
		t.Fatalf("installed = %v", report.Installed)
	}
}

func TestReadActionsInOfflineMode(t *testing.T) {
	project := newGitProject(t)

	readiness, err := Read(context.Background(), project, Options{Offline: true})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	local := actionFor(readiness, ActionInstallSkillsLocal)
	if local.Available || !strings.Contains(local.Reason, "offline") {
		t.Fatalf("local action = %+v", local)
	}
	online := actionFor(readiness, ActionInstallSkillsOnline)
	if online.Available || !strings.Contains(online.Reason, "offline") {
		t.Fatalf("online action = %+v", online)
	}
	if !readiness.Offline {
		t.Fatal("the report does not say it is offline")
	}

	source := t.TempDir()
	readiness, err = Read(context.Background(), project, Options{Offline: true, SkillsSource: source})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !actionFor(readiness, ActionInstallSkillsLocal).Available {
		t.Fatal("a configured source is not offered offline")
	}
	if actionFor(readiness, ActionInstallSkillsOnline).Available {
		t.Fatal("the online action is offered offline")
	}
}

func TestReadActionsOnlineWithASource(t *testing.T) {
	project := newGitProject(t)
	source := t.TempDir()

	readiness, err := Read(context.Background(), project, Options{SkillsSource: source})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !actionFor(readiness, ActionInstallSkillsLocal).Available {
		t.Fatal("the local action is not offered")
	}
	online := actionFor(readiness, ActionInstallSkillsOnline)
	if online.Available || !strings.Contains(online.Reason, "local skills source") {
		t.Fatalf("online action = %+v", online)
	}
}
