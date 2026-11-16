package buildinfo

import (
	"runtime/debug"
	"testing"
	"time"
)

// setStamps injects build stamps for one test and restores the zero state
// afterwards, so a table case cannot leak into the next.
func setStamps(t *testing.T, v, rev, revTime, mod, built string) {
	t.Helper()

	previousVersion, previousRevision, previousRevisionTime := version, revision, revisionTime
	previousModified, previousBuiltAt := modified, builtAt
	t.Cleanup(func() {
		version, revision, revisionTime = previousVersion, previousRevision, previousRevisionTime
		modified, builtAt = previousModified, previousBuiltAt
	})

	version, revision, revisionTime, modified, builtAt = v, rev, revTime, mod, built
}

// setToolchain fakes the toolchain's build metadata for one test.
func setToolchain(t *testing.T, info *debug.BuildInfo) {
	t.Helper()

	previous := readBuildInfo
	t.Cleanup(func() { readBuildInfo = previous })
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return info, info != nil
	}
}

func TestReadUsesInjectedStamps(t *testing.T) {
	setStamps(t, "0.2.0", "abc123", "2026-11-15T13:07:33+03:00", "true", "2026-11-15T10:30:12Z")
	setToolchain(t, nil)

	info := Read()

	if info.Version != "0.2.0" {
		t.Fatalf("version = %q, want 0.2.0", info.Version)
	}
	if info.Revision != "abc123" {
		t.Fatalf("revision = %q, want abc123", info.Revision)
	}
	if got := info.RevisionTime.UTC().Format(time.RFC3339); got != "2026-11-15T10:07:33Z" {
		t.Fatalf("revision time = %q, want 2026-11-15T10:07:33Z", got)
	}
	if info.Modified == nil || !*info.Modified {
		t.Fatalf("modified = %v, want true", info.Modified)
	}
	if got := info.BuiltAt.UTC().Format(time.RFC3339); got != "2026-11-15T10:30:12Z" {
		t.Fatalf("build time = %q, want 2026-11-15T10:30:12Z", got)
	}
}

func TestReadKeepsACleanTreeDistinctFromAnUnknownOne(t *testing.T) {
	setStamps(t, "", "", "", "false", "")
	setToolchain(t, nil)

	info := Read()

	if info.Modified == nil {
		t.Fatal("modified = nil for a recorded clean tree, want false")
	}
	if *info.Modified {
		t.Fatal("modified = true for a recorded clean tree")
	}

	setStamps(t, "", "", "", "", "")
	if got := Read(); got.Modified != nil {
		t.Fatalf("modified = %v with no stamp, want nil", got.Modified)
	}
}

func TestReadOmitsUnknownStamps(t *testing.T) {
	setStamps(t, "", "abc123", "", "", "")
	setToolchain(t, nil)

	info := Read()

	if info.Revision != "abc123" {
		t.Fatalf("revision = %q, want abc123", info.Revision)
	}
	if info.Version != "" || !info.RevisionTime.IsZero() || info.Modified != nil || !info.BuiltAt.IsZero() {
		t.Fatalf("unknown fields were filled: %+v", info)
	}
}

func TestReadFallsBackToTheToolchain(t *testing.T) {
	setStamps(t, "", "", "", "", "")
	setToolchain(t, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "def456"},
			{Key: "vcs.time", Value: "2026-11-14T17:04:03Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	})

	info := Read()

	if info.Version != "v0.1.0" || info.Revision != "def456" {
		t.Fatalf("identity = %+v, want the toolchain version and revision", info)
	}
	if got := info.RevisionTime.UTC().Format(time.RFC3339); got != "2026-11-14T17:04:03Z" {
		t.Fatalf("revision time = %q, want the toolchain value", got)
	}
	if info.Modified == nil || !*info.Modified {
		t.Fatalf("modified = %v, want the toolchain value", info.Modified)
	}
	if !info.BuiltAt.IsZero() {
		t.Fatalf("build time = %v, want unknown: the toolchain records no build time", info.BuiltAt)
	}
}

func TestReadDropsTheToolchainDevelopmentMarker(t *testing.T) {
	setStamps(t, "", "", "", "", "")
	setToolchain(t, &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "def456"}},
	})

	if got := Read().Version; got != "" {
		t.Fatalf("version = %q, want it omitted for a development build", got)
	}
}

func TestInjectedStampsWinOverTheToolchain(t *testing.T) {
	setStamps(t, "0.2.0", "abc123", "", "", "")
	setToolchain(t, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "def456"},
		},
	})

	info := Read()

	if info.Version != "0.2.0" || info.Revision != "abc123" {
		t.Fatalf("identity = %+v, want the injected stamps", info)
	}
}

func TestReadWithoutBuildMetadata(t *testing.T) {
	setStamps(t, "", "", "", "", "")
	setToolchain(t, nil)

	info := Read()

	if info != (Info{}) {
		t.Fatalf("identity = %+v, want the zero value", info)
	}
}

func TestReadIgnoresMalformedStamps(t *testing.T) {
	setStamps(t, "", "", "yesterday", "maybe", "not-a-date")
	setToolchain(t, nil)

	info := Read()

	if !info.RevisionTime.IsZero() || !info.BuiltAt.IsZero() || info.Modified != nil {
		t.Fatalf("malformed stamps were accepted: %+v", info)
	}
}
