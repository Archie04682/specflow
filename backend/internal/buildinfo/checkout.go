package buildinfo

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ModulePath is the module this build belongs to. Only a checkout declaring it
// is treated as a SpecFlow source tree; anything else is not ours to judge.
const ModulePath = "example.com/specflow"

// Staleness reasons the checkout report can carry.
const (
	ReasonCheckoutAhead = "checkout-ahead"
	ReasonSourcesNewer  = "sources-newer"
)

// maxNewerSources caps the reported count: the operator needs to know that
// sources moved, not how many files a walk happened to touch.
const maxNewerSources = 99

// checkoutCacheTTL is short enough to notice a real edit on the next page
// focus and long enough that a focus storm cannot rescan per request.
const checkoutCacheTTL = 3 * time.Second

// gitTimeout bounds one version-control lookup. A hung git must not hold a
// request; every failure simply drops the report.
const gitTimeout = 3 * time.Second

// Checkout is the running build's relationship to the source tree it was
// launched from. A nil *Checkout means the tree is not a SpecFlow checkout.
type Checkout struct {
	// Revision is the checkout's current HEAD, empty when it has no VCS state.
	Revision string
	// StaleReason is empty for a current build, otherwise one of the Reason
	// constants.
	StaleReason string
	// NewerSources is the number of build inputs modified after the build,
	// capped at maxNewerSources.
	NewerSources int
}

// Prober inspects a checkout with a short cache so repeated requests cannot
// rescan. Its zero value is not usable; use NewProber.
type Prober struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	at       time.Time
	checkout *Checkout
}

// NewProber returns a prober that caches a checkout's report for ttl. A
// non-positive ttl disables caching.
func NewProber(ttl time.Duration) *Prober {
	return newProber(ttl, time.Now)
}

// newProber is the testable constructor: the clock is a seam so the cache
// window can be exercised without sleeping.
func newProber(ttl time.Duration, now func() time.Time) *Prober {
	return &Prober{ttl: ttl, now: now, entries: make(map[string]cacheEntry)}
}

// defaultProber serves the package-level convenience probe.
var defaultProber = NewProber(checkoutCacheTTL)

// ProbeCheckout reports the checkout at root, or nil when root is not a
// SpecFlow source checkout. It never fails: unreadable VCS state and missing
// tooling drop individual findings rather than the report.
func ProbeCheckout(root string, built Info, startedAt time.Time) *Checkout {
	return defaultProber.Probe(root, built, startedAt)
}

// Probe reports the checkout at root with a short cache. built supplies the
// revision and build time the running binary was made from; startedAt is the
// process start used when the build time is unknown.
func (p *Prober) Probe(root string, built Info, startedAt time.Time) *Checkout {
	if root == "" {
		return nil
	}
	root = filepath.Clean(root)

	if p.ttl > 0 {
		p.mu.Lock()
		entry, hit := p.entries[root]
		p.mu.Unlock()
		if hit && p.now().Sub(entry.at) < p.ttl {
			return entry.checkout
		}
	}

	checkout := p.scan(root, built, startedAt)

	if p.ttl > 0 {
		p.mu.Lock()
		p.entries[root] = cacheEntry{at: p.now(), checkout: checkout}
		p.mu.Unlock()
	}
	return checkout
}

func (p *Prober) scan(root string, built Info, startedAt time.Time) *Checkout {
	moduleRoot := findModuleRoot(root)
	if moduleRoot == "" {
		return nil
	}

	checkout := &Checkout{}
	if head, err := git(moduleRoot, "rev-parse", "HEAD"); err == nil {
		checkout.Revision = strings.TrimSpace(head)
	}

	// A build from unrelated history is not comparable: the ancestor check
	// fails and the report stays silent rather than claiming "behind".
	if built.Revision != "" && checkout.Revision != "" && checkout.Revision != built.Revision {
		if _, err := git(moduleRoot, "merge-base", "--is-ancestor", built.Revision, checkout.Revision); err == nil {
			checkout.StaleReason = ReasonCheckoutAhead
		}
	}

	reference := built.BuiltAt
	if reference.IsZero() {
		reference = startedAt
	}
	if count := countNewerSources(moduleRoot, reference); count > 0 {
		checkout.NewerSources = count
		if checkout.StaleReason == "" {
			checkout.StaleReason = ReasonSourcesNewer
		}
	}
	return checkout
}

// findModuleRoot walks up from start looking for this module's go.mod and a
// backend directory. A go.mod naming another module ends the walk: a nested
// module is a boundary, not something to look past.
func findModuleRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}

	for {
		data, readErr := os.ReadFile(filepath.Join(dir, "go.mod"))
		if readErr == nil {
			if modulePath(data) != ModulePath {
				return ""
			}
			if info, statErr := os.Stat(filepath.Join(dir, "backend")); statErr != nil || !info.IsDir() {
				return ""
			}
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// modulePath reads the module directive without pulling in a YAML or module
// parser: go.mod's directive grammar is small and stable.
func modulePath(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`)
		}
	}
	return ""
}

// countNewerSources counts build inputs modified after reference. Test files
// and testdata are excluded: they are not compiled into the server binary, so
// editing them does not make the running server stale.
func countNewerSources(root string, reference time.Time) int {
	if reference.IsZero() {
		return 0
	}

	count := 0
	countFile := func(path string) {
		if count >= maxNewerSources {
			return
		}
		if info, err := os.Stat(path); err == nil && info.ModTime().After(reference) {
			count++
		}
	}

	countFile(filepath.Join(root, "go.mod"))
	countFile(filepath.Join(root, "go.sum"))

	_ = filepath.WalkDir(filepath.Join(root, "backend"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || entry.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		countFile(path)
		return nil
	})
	return count
}

// git runs one git command in dir with a bounded wait and returns its output.
func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	output, err := command.Output()
	return string(output), err
}
