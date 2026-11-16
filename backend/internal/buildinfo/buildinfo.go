// Package buildinfo reports which build of SpecFlow is running and whether the
// checkout it was launched from has moved past that build.
package buildinfo

import (
	"runtime/debug"
	"strings"
	"time"
)

// Stamps injected by the build with -ldflags -X. They stay empty when the
// build did not set them, which is a normal state rather than an error: the
// Go toolchain fills the same fields for a plain `go build`, but records
// nothing for a `go run` binary, so the development workflow injects them.
var (
	version      string
	revision     string
	revisionTime string
	modified     string
	builtAt      string
)

// readBuildInfo is a seam so tests can supply toolchain settings without a
// real build. Production always reads the running executable's own metadata.
var readBuildInfo = debug.ReadBuildInfo

// Info is what a running build can say about itself. A zero value means the
// build could not determine that field; callers omit it rather than guess.
type Info struct {
	Version      string
	Revision     string
	RevisionTime time.Time
	// Modified is nil when the build did not record whether the tree was
	// dirty, so a clean tree and an unknown tree stay distinguishable.
	Modified *bool
	BuiltAt  time.Time
}

// Read reports the running build's identity. Injected stamps win over the Go
// toolchain's build settings, which is the only source available to a plain
// `go build` and no source at all for `go run`.
func Read() Info {
	stamp := toolchain()

	info := Info{
		Version:  firstNonEmpty(version, stamp.version),
		Revision: firstNonEmpty(revision, stamp.revision),
	}
	info.RevisionTime = parseTime(firstNonEmpty(revisionTime, stamp.revisionTime))
	info.BuiltAt = parseTime(builtAt)

	info.Modified = parseBool(modified)
	if info.Modified == nil {
		info.Modified = parseBool(stamp.modified)
	}
	return info
}

// toolchain is the part of debug.ReadBuildInfo this package uses, so the
// fallback can be tested without building the test binary from a VCS checkout.
type toolchainInfo struct {
	version      string
	revision     string
	revisionTime string
	modified     string
}

// toolchain reads the Go toolchain's own build settings. A build without them
// (notably `go run`) yields zero values, which Read treats as unknown.
func toolchain() toolchainInfo {
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return toolchainInfo{}
	}

	stamp := toolchainInfo{}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		stamp.version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			stamp.revision = setting.Value
		case "vcs.time":
			stamp.revisionTime = setting.Value
		case "vcs.modified":
			stamp.modified = setting.Value
		}
	}
	return stamp
}

// firstNonEmpty returns the first set value, so injected stamps win over the
// toolchain's fallback without either source needing to know about the other.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// parseTime accepts an RFC 3339 stamp and treats anything else as unknown, so
// a malformed injection cannot fail a request later.
func parseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func parseBool(raw string) *bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true":
		value := true
		return &value
	case "false":
		value := false
		return &value
	default:
		return nil
	}
}
