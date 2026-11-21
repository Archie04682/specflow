package onboarding

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Bounds mirrored from the skills CLI, so both providers refuse the same
// archives.
const (
	maxSourceEntries = 1000
	maxSourceBytes   = 25 << 20
)

type sourceKind int

const (
	sourceDirectory sourceKind = iota
	sourceZip
	sourceTar
	sourceTarGz
)

// skillCandidate is a directory inside a source that holds a SKILL.md.
type skillCandidate struct {
	name string // base name, which becomes the installed skill name
	path string // slash-separated path inside the source; "." is the source root
}

// skillsSource is a directory or an archive that provides skills.
type skillsSource struct {
	path       string
	kind       sourceKind
	candidates []skillCandidate
}

// openSkillsSource discovers the skill directories in a source without
// extracting anything.
func openSkillsSource(sourcePath string) (*skillsSource, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("skills source %s: %w", sourcePath, err)
	}

	source := &skillsSource{path: sourcePath}
	lower := strings.ToLower(sourcePath)
	switch {
	case info.IsDir():
		source.kind = sourceDirectory
		source.candidates, err = discoverDirectory(sourcePath)
	case info.Mode().IsRegular():
		switch {
		case strings.HasSuffix(lower, ".zip"):
			source.kind = sourceZip
			source.candidates, err = scanZip(sourcePath)
		case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
			source.kind = sourceTarGz
			source.candidates, err = scanTar(sourcePath, true)
		case strings.HasSuffix(lower, ".tar"):
			source.kind = sourceTar
			source.candidates, err = scanTar(sourcePath, false)
		default:
			return nil, fmt.Errorf("skills source %s: unsupported file type", sourcePath)
		}
	default:
		return nil, fmt.Errorf("skills source %s: not a directory or archive", sourcePath)
	}
	if err != nil {
		return nil, err
	}

	source.candidates = pruneNested(source.candidates)
	if len(source.candidates) == 0 {
		return nil, fmt.Errorf("skills source %s: no directory with a SKILL.md", sourcePath)
	}
	if err := uniqueNames(source.candidates); err != nil {
		return nil, fmt.Errorf("skills source %s: %w", sourcePath, err)
	}
	sort.Slice(source.candidates, func(i, j int) bool { return source.candidates[i].name < source.candidates[j].name })
	return source, nil
}

// discoverDirectory walks a directory tree for skill directories. A discovered
// skill is not searched further: nested manifests belong to it, not beside it.
func discoverDirectory(root string) ([]skillCandidate, error) {
	candidates := make([]skillCandidate, 0)
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() || !hasSkillManifest(current) {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		name := entry.Name()
		if relative == "." {
			name = filepath.Base(root)
		}
		candidates = append(candidates, skillCandidate{name: name, path: relative})
		return fs.SkipDir
	})
	return candidates, err
}

func hasSkillManifest(directory string) bool {
	return isFile(filepath.Join(directory, "SKILL.md"))
}

// scanZip indexes an archive without extracting it.
func scanZip(file string) ([]skillCandidate, error) {
	reader, err := zip.OpenReader(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	defer reader.Close()

	if len(reader.File) > maxSourceEntries {
		return nil, fmt.Errorf("%s: more than %d entries", file, maxSourceEntries)
	}
	var total uint64
	directories := map[string]bool{}
	for _, entry := range reader.File {
		if !safeArchivePath(entry.Name) {
			return nil, fmt.Errorf("%s: unsafe entry %q", file, entry.Name)
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s: link entry %q", file, entry.Name)
		}
		total += entry.UncompressedSize64
		if total > maxSourceBytes {
			return nil, fmt.Errorf("%s: expands beyond %d MiB", file, maxSourceBytes>>20)
		}
		if path.Base(entry.Name) == "SKILL.md" && !entry.FileInfo().IsDir() {
			directories[path.Dir(entry.Name)] = true
		}
	}
	return candidatesFromDirs(directories, archiveBaseName(file)), nil
}

// scanTar indexes a tar or tar.gz archive. It needs a second pass to extract,
// which is why extraction re-opens the file.
func scanTar(file string, compressed bool) ([]skillCandidate, error) {
	handle, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	defer handle.Close()

	var reader io.Reader = handle
	if compressed {
		gzipped, err := gzip.NewReader(handle)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		defer gzipped.Close()
		reader = gzipped
	}

	archive := tar.NewReader(reader)
	directories := map[string]bool{}
	entries := 0
	var total int64
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		entries++
		if entries > maxSourceEntries {
			return nil, fmt.Errorf("%s: more than %d entries", file, maxSourceEntries)
		}
		switch header.Typeflag {
		case tar.TypeSymlink, tar.TypeLink:
			return nil, fmt.Errorf("%s: link entry %q", file, header.Name)
		}
		if !safeArchivePath(header.Name) {
			return nil, fmt.Errorf("%s: unsafe entry %q", file, header.Name)
		}
		total += header.Size
		if total > maxSourceBytes {
			return nil, fmt.Errorf("%s: expands beyond %d MiB", file, maxSourceBytes>>20)
		}
		regular := header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA
		if regular && path.Base(header.Name) == "SKILL.md" {
			directories[path.Dir(header.Name)] = true
		}
	}
	return candidatesFromDirs(directories, archiveBaseName(file)), nil
}

func candidatesFromDirs(directories map[string]bool, fallback string) []skillCandidate {
	candidates := make([]skillCandidate, 0, len(directories))
	for directory := range directories {
		name := path.Base(directory)
		if directory == "." || name == "." || name == "/" {
			name = fallback
		}
		candidates = append(candidates, skillCandidate{name: name, path: directory})
	}
	return candidates
}

// pruneNested drops a candidate that lives inside another one: the outer
// directory is the skill, and its files include the inner one.
func pruneNested(candidates []skillCandidate) []skillCandidate {
	kept := make([]skillCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		nested := false
		for _, other := range candidates {
			if other.path == candidate.path {
				continue
			}
			if other.path == "." || strings.HasPrefix(candidate.path, other.path+"/") {
				nested = true
				break
			}
		}
		if !nested {
			kept = append(kept, candidate)
		}
	}
	return kept
}

func uniqueNames(candidates []skillCandidate) error {
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if seen[candidate.name] {
			return fmt.Errorf("two directories are named %q", candidate.name)
		}
		seen[candidate.name] = true
	}
	return nil
}

// safeArchivePath rejects an entry that is absolute or resolves outside the
// extraction root.
func safeArchivePath(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return false
	}
	cleaned := path.Clean(name)
	return cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

func archiveBaseName(file string) string {
	base := filepath.Base(file)
	for _, suffix := range []string{".tar.gz", ".tgz", ".zip", ".tar"} {
		if strings.HasSuffix(strings.ToLower(base), suffix) {
			return base[:len(base)-len(suffix)]
		}
	}
	return base
}

// materialize writes only the candidate directories into dest, one directory
// per skill. Everything else in the source is left alone.
func (s *skillsSource) materialize(dest string) error {
	switch s.kind {
	case sourceDirectory:
		return s.materializeDirectory(dest)
	case sourceZip:
		return s.materializeZip(dest)
	default:
		return s.materializeTar(dest, s.kind == sourceTarGz)
	}
}

func (s *skillsSource) materializeDirectory(dest string) error {
	for _, candidate := range s.candidates {
		source := filepath.Join(s.path, filepath.FromSlash(candidate.path))
		if err := copyTree(source, filepath.Join(dest, candidate.name)); err != nil {
			return err
		}
	}
	return nil
}

func (s *skillsSource) materializeZip(dest string) error {
	reader, err := zip.OpenReader(s.path)
	if err != nil {
		return fmt.Errorf("read %s: %w", s.path, err)
	}
	defer reader.Close()

	for _, entry := range reader.File {
		candidate, relative, ok := matchCandidate(entry.Name, s.candidates)
		if !ok {
			continue
		}
		target := filepath.Join(dest, candidate.name, filepath.FromSlash(relative))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := writeArchiveFile(target, entry.Mode().Perm(), func() (io.ReadCloser, error) { return entry.Open() }); err != nil {
			return err
		}
	}
	return nil
}

func (s *skillsSource) materializeTar(dest string, compressed bool) error {
	handle, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("read %s: %w", s.path, err)
	}
	defer handle.Close()

	var reader io.Reader = handle
	if compressed {
		gzipped, err := gzip.NewReader(handle)
		if err != nil {
			return fmt.Errorf("read %s: %w", s.path, err)
		}
		defer gzipped.Close()
		reader = gzipped
	}

	archive := tar.NewReader(reader)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", s.path, err)
		}
		candidate, relative, ok := matchCandidate(header.Name, s.candidates)
		if !ok {
			continue
		}
		target := filepath.Join(dest, candidate.name, filepath.FromSlash(relative))
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := writeArchiveFile(target, os.FileMode(header.Mode).Perm(), func() (io.ReadCloser, error) {
			return io.NopCloser(archive), nil
		}); err != nil {
			return err
		}
	}
}

// matchCandidate maps an archive entry to the candidate it belongs to and the
// path inside that candidate.
func matchCandidate(name string, candidates []skillCandidate) (skillCandidate, string, bool) {
	for _, candidate := range candidates {
		if candidate.path == "." {
			return candidate, name, true
		}
		if name == candidate.path {
			return candidate, "", true
		}
		if strings.HasPrefix(name, candidate.path+"/") {
			return candidate, name[len(candidate.path)+1:], true
		}
	}
	return skillCandidate{}, "", false
}

// writeArchiveFile writes one extracted entry, bounded by the source limits.
func writeArchiveFile(target string, mode os.FileMode, open func() (io.ReadCloser, error)) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	reader, err := open()
	if err != nil {
		return err
	}
	defer reader.Close()

	if mode == 0 {
		mode = 0o644
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	// The archive's declared sizes were checked already; the extra bound stops
	// a header that lied about its size.
	written, err := io.Copy(file, io.LimitReader(reader, maxSourceBytes))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if written >= maxSourceBytes {
		return fmt.Errorf("%s: extracted content exceeds %d MiB", target, maxSourceBytes>>20)
	}
	return nil
}

// copyTree copies a directory's regular files to a destination.
func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		// Links are never copied: extraction promises regular files only.
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destination, data, info.Mode().Perm())
	})
}

// treesEqual reports whether two directory trees hold the same files with the
// same content.
func treesEqual(left, right string) bool {
	leftDigests, err := treeDigests(left)
	if err != nil {
		return false
	}
	rightDigests, err := treeDigests(right)
	if err != nil {
		return false
	}
	if len(leftDigests) != len(rightDigests) {
		return false
	}
	for name, digest := range leftDigests {
		if rightDigests[name] != digest {
			return false
		}
	}
	return true
}

// treeDigests maps each regular file under root to its content digest.
func treeDigests(root string) (map[string]string, error) {
	digests := make(map[string]string)
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		digests[filepath.ToSlash(relative)] = hex.EncodeToString(sum[:])
		return nil
	})
	return digests, err
}
