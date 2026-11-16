package buildinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func setMTime(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("set modification time on %s: %v", path, err)
	}
}

// runGit runs one git command in dir without depending on the machine's git
// identity or signing configuration.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-C", dir, "-c", "user.name=SpecFlow Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}
	command := exec.Command("git", append(base, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// newCheckout creates a minimal SpecFlow checkout with its sources dated in
// the past, so a build made "an hour ago" is clean unless a test edits them.
func newCheckout(t *testing.T) (dir, head string, base time.Time) {
	t.Helper()

	dir = t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/specflow\n\ngo 1.27\n")
	writeFile(t, filepath.Join(dir, "backend", "main.go"), "package main\n\nfunc main() {}\n")

	base = time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	setMTime(t, filepath.Join(dir, "go.mod"), base)
	setMTime(t, filepath.Join(dir, "backend", "main.go"), base)

	runGit(t, dir, "init")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "first")
	return dir, runGit(t, dir, "rev-parse", "HEAD"), base
}

func TestProbeReportsACleanCheckout(t *testing.T) {
	dir, head, base := newCheckout(t)
	prober := newProber(0, time.Now)

	got := prober.Probe(dir, Info{Revision: head, BuiltAt: base.Add(time.Hour)}, base)

	if got == nil {
		t.Fatal("a SpecFlow checkout was not recognized")
	}
	if got.Revision != head {
		t.Fatalf("revision = %q, want %q", got.Revision, head)
	}
	if got.StaleReason != "" || got.NewerSources != 0 {
		t.Fatalf("clean checkout reported stale: %+v", got)
	}
}

func TestProbeWalksUpFromASubdirectory(t *testing.T) {
	dir, head, base := newCheckout(t)
	prober := newProber(0, time.Now)

	got := prober.Probe(filepath.Join(dir, "backend"), Info{Revision: head, BuiltAt: base.Add(time.Hour)}, base)

	if got == nil {
		t.Fatal("a checkout was not recognized from its subdirectory")
	}
	if got.Revision != head {
		t.Fatalf("revision = %q, want %q", got.Revision, head)
	}
}

func TestProbeReportsCheckoutAhead(t *testing.T) {
	dir, first, _ := newCheckout(t)
	built := Info{Revision: first, BuiltAt: time.Now().Add(time.Minute)}

	writeFile(t, filepath.Join(dir, "backend", "extra.go"), "package main\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "second")
	second := runGit(t, dir, "rev-parse", "HEAD")

	got := newProber(0, time.Now).Probe(dir, built, time.Now())

	if got == nil {
		t.Fatal("a SpecFlow checkout was not recognized")
	}
	if got.StaleReason != ReasonCheckoutAhead {
		t.Fatalf("stale reason = %q, want %q", got.StaleReason, ReasonCheckoutAhead)
	}
	if got.Revision != second {
		t.Fatalf("revision = %q, want the new HEAD %q", got.Revision, second)
	}
}

func TestProbeReportsUnrelatedHistoryAsCurrent(t *testing.T) {
	dir, head, base := newCheckout(t)
	built := Info{Revision: strings.Repeat("0", 40), BuiltAt: base.Add(time.Hour)}

	got := newProber(0, time.Now).Probe(dir, built, base)

	if got == nil {
		t.Fatal("a SpecFlow checkout was not recognized")
	}
	if got.StaleReason != "" {
		t.Fatalf("unrelated history reported %q, want no reason", got.StaleReason)
	}
	if got.Revision != head {
		t.Fatalf("revision = %q, want %q", got.Revision, head)
	}
}

func TestProbeReportsNewerSources(t *testing.T) {
	dir, head, base := newCheckout(t)
	writeFile(t, filepath.Join(dir, "backend", "later.go"), "package main\n")

	got := newProber(0, time.Now).Probe(dir, Info{Revision: head, BuiltAt: base}, base)

	if got == nil {
		t.Fatal("a SpecFlow checkout was not recognized")
	}
	if got.StaleReason != ReasonSourcesNewer {
		t.Fatalf("stale reason = %q, want %q", got.StaleReason, ReasonSourcesNewer)
	}
	if got.NewerSources < 1 {
		t.Fatalf("newer sources = %d, want at least one", got.NewerSources)
	}
}

func TestProbePrefersCheckoutAheadOverNewerSources(t *testing.T) {
	dir, first, base := newCheckout(t)
	writeFile(t, filepath.Join(dir, "backend", "later.go"), "package main\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "second")

	got := newProber(0, time.Now).Probe(dir, Info{Revision: first, BuiltAt: base}, base)

	if got == nil {
		t.Fatal("a SpecFlow checkout was not recognized")
	}
	if got.StaleReason != ReasonCheckoutAhead {
		t.Fatalf("stale reason = %q, want the checkout-ahead reason to win", got.StaleReason)
	}
}

func TestProbeIgnoresTestAndTestdataEdits(t *testing.T) {
	dir, head, base := newCheckout(t)
	writeFile(t, filepath.Join(dir, "backend", "main_test.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "backend", "testdata", "fixture.go"), "package fixture\n")

	got := newProber(0, time.Now).Probe(dir, Info{Revision: head, BuiltAt: base}, base)

	if got == nil {
		t.Fatal("a SpecFlow checkout was not recognized")
	}
	if got.StaleReason != "" {
		t.Fatalf("test-only edits reported stale: %+v", got)
	}
}

func TestProbeIgnoresSourcesOlderThanTheBuild(t *testing.T) {
	dir, head, base := newCheckout(t)

	got := newProber(0, time.Now).Probe(dir, Info{Revision: head, BuiltAt: time.Now()}, base)

	if got == nil {
		t.Fatal("a SpecFlow checkout was not recognized")
	}
	if got.StaleReason != "" {
		t.Fatalf("sources older than the build reported stale: %+v", got)
	}
}

func TestProbeFallsBackToTheProcessStartWithoutABuildTime(t *testing.T) {
	dir, head, base := newCheckout(t)
	writeFile(t, filepath.Join(dir, "backend", "later.go"), "package main\n")

	// No build time: the process start is the reference, and the edit is newer.
	got := newProber(0, time.Now).Probe(dir, Info{Revision: head}, base)

	if got == nil {
		t.Fatal("a SpecFlow checkout was not recognized")
	}
	if got.StaleReason != ReasonSourcesNewer {
		t.Fatalf("stale reason = %q, want %q without a build time", got.StaleReason, ReasonSourcesNewer)
	}
}

func TestProbeRejectsANonCheckout(t *testing.T) {
	if got := newProber(0, time.Now).Probe(t.TempDir(), Info{}, time.Now()); got != nil {
		t.Fatalf("a plain directory was treated as a checkout: %+v", got)
	}
}

func TestProbeRejectsAnotherModule(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/other\n")
	writeFile(t, filepath.Join(dir, "backend", "main.go"), "package main\n")

	if got := newProber(0, time.Now).Probe(dir, Info{}, time.Now()); got != nil {
		t.Fatalf("another module was treated as a SpecFlow checkout: %+v", got)
	}
}

func TestProbeSurvivesMissingVCSState(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/specflow\n")
	writeFile(t, filepath.Join(dir, "backend", "main.go"), "package main\n")
	base := time.Now().Add(-time.Hour)
	setMTime(t, filepath.Join(dir, "go.mod"), base.Add(-time.Hour))
	setMTime(t, filepath.Join(dir, "backend", "main.go"), base.Add(-time.Hour))

	// No git repository: identity keeps the layout's report but no revision.
	got := newProber(0, time.Now).Probe(dir, Info{BuiltAt: base}, base.Add(time.Minute))
	if got == nil {
		t.Fatal("a checkout layout without VCS state was not recognized")
	}
	if got.Revision != "" {
		t.Fatalf("revision = %q, want it omitted", got.Revision)
	}
	if got.StaleReason != "" {
		t.Fatalf("clean layout reported stale: %+v", got)
	}

	// A failed git lookup must not hide the source-recency finding.
	writeFile(t, filepath.Join(dir, "backend", "later.go"), "package main\n")
	got = newProber(0, time.Now).Probe(dir, Info{BuiltAt: base}, base)
	if got == nil || got.StaleReason != ReasonSourcesNewer {
		t.Fatalf("newer sources without git = %+v, want %q", got, ReasonSourcesNewer)
	}
}

func TestProbeCachesWithinItsWindow(t *testing.T) {
	dir, head, base := newCheckout(t)
	now := base.Add(time.Hour)
	prober := newProber(time.Hour, func() time.Time { return now })
	built := Info{Revision: head, BuiltAt: base.Add(30 * time.Minute)}

	if got := prober.Probe(dir, built, base); got == nil || got.StaleReason != "" {
		t.Fatalf("first probe = %+v, want a clean checkout", got)
	}

	// An edit inside the window is not seen: the cached report is reused.
	writeFile(t, filepath.Join(dir, "backend", "later.go"), "package main\n")
	if got := prober.Probe(dir, built, base); got == nil || got.StaleReason != "" {
		t.Fatalf("second probe = %+v, want the cached clean report", got)
	}

	// Past the window the probe rescans and reports the edit.
	now = now.Add(2 * time.Hour)
	got := prober.Probe(dir, built, base)
	if got == nil || got.StaleReason != ReasonSourcesNewer {
		t.Fatalf("expired probe = %+v, want %q", got, ReasonSourcesNewer)
	}
}
