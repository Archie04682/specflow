package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type shimMode string

const (
	// shimPass records its own pid and becomes the real harness.
	shimPass shimMode = "pass"
	// shimCrash starts the real harness, kills it after a delay, and exits
	// with the status of a process killed by SIGKILL.
	shimCrash shimMode = "crash"
	// shimHang records its pid and sleeps without ever starting the harness,
	// which gives timeout and cancellation tests a child that ignores stdin.
	shimHang shimMode = "hang"
)

// writePiShim returns an executable that stands in for "pi" as the adapter's
// Binary. The adapter still finds a program named pi; the shim decides whether
// it passes through, crashes mid-run, or hangs.
func writePiShim(t *testing.T, realBinary string, mode shimMode) (path, pidfile string) {
	t.Helper()

	dir := t.TempDir()
	path = filepath.Join(dir, "pi")
	pidfile = filepath.Join(dir, "pi.pid")

	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	fmt.Fprintf(&script, "printf '%%s\\n' \"$@\" > %q\n", pidfile+".args")
	switch mode {
	case shimPass:
		fmt.Fprintf(&script, "echo $$ > %q\n", pidfile)
		fmt.Fprintf(&script, "exec %q \"$@\"\n", realBinary)
	case shimCrash:
		fmt.Fprintf(&script, "%q \"$@\" 0<&0 &\n", realBinary)
		script.WriteString("child=$!\n")
		script.WriteString("trap 'kill -9 \"$child\" 2>/dev/null' EXIT TERM INT\n")
		fmt.Fprintf(&script, "echo $child > %q\n", pidfile)
		script.WriteString("sleep ${PI_SHIM_CRASH_DELAY:-2}\n")
		script.WriteString("kill -9 \"$child\" 2>/dev/null\n")
		script.WriteString("wait \"$child\" 2>/dev/null\n")
		script.WriteString("exit 137\n")
	case shimHang:
		fmt.Fprintf(&script, "echo $$ > %q\n", pidfile)
		script.WriteString("exec sleep 3600\n")
	default:
		t.Fatalf("unknown shim mode %q", mode)
	}

	if err := os.WriteFile(path, []byte(script.String()), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	return path, pidfile
}

// writeArgEcho returns a stand-in for the real harness binary that prints each
// argument on its own line. The host's echo cannot serve as one: uutils echo
// answers --version with its own banner, so the forwarded argument would be
// missing on a runner whose coreutils is not GNU.
func writeArgEcho(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "arg-echo")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755); err != nil {
		t.Fatalf("write arg echo: %v", err)
	}
	return path
}

// readShimArgs returns the arguments the shim received.
func readShimArgs(t *testing.T, pidfile string) []string {
	t.Helper()

	raw, err := os.ReadFile(pidfile + ".args")
	if err != nil {
		t.Fatalf("read shim args: %v", err)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func TestPassShimExecsTheRealBinary(t *testing.T) {
	shim, pidfile := writePiShim(t, writeArgEcho(t), shimPass)

	output, err := exec.Command(shim, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("run shim: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "--version") {
		t.Fatalf("output = %q, want the forwarded argument", output)
	}
	if args := readShimArgs(t, pidfile); len(args) != 1 || args[0] != "--version" {
		t.Fatalf("args = %v, want [--version]", args)
	}

	pid := readPID(t, pidfile)
	assertProcessGone(t, pid)
}

func TestCrashShimKillsItsChild(t *testing.T) {
	shim, pidfile := writePiShim(t, "/bin/sleep", shimCrash)

	command := exec.Command(shim, "30")
	command.Env = append(os.Environ(), "PI_SHIM_CRASH_DELAY=1")
	err := command.Run()

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("run crash shim: %v, want a non-zero exit", err)
	}
	if code := exit.ExitCode(); code != 137 {
		t.Fatalf("exit = %d, want 137", code)
	}

	assertProcessGone(t, readPID(t, pidfile))
}

func TestHangShimRecordsAKillablePID(t *testing.T) {
	shim, pidfile := writePiShim(t, "", shimHang)

	command := exec.Command(shim)
	if err := command.Start(); err != nil {
		t.Fatalf("start hang shim: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(done)
	}()

	pid := readPID(t, pidfile)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill hang shim: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("hang shim did not exit after SIGKILL")
	}
	assertProcessGone(t, pid)
}

// readPID waits briefly for a shim to write its pid file.
func readPID(t *testing.T, path string) int {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatalf("parse pid %q: %v", raw, err)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pid file %s was never written", path)
	return 0
}

// assertProcessGone proves a recorded pid no longer exists.
func assertProcessGone(t *testing.T, pid int) {
	t.Helper()

	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("process %d is still alive (err = %v)", pid, err)
	}
}
