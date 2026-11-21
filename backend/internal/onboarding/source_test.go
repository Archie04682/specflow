package onboarding

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const skillFile = "---\nname: placeholder\ndescription: A skill\n---\n"

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()

	handle, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(handle)
	for _, name := range sortedKeys(entries) {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(entries[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTar(t *testing.T, path string, entries map[string]string, compressed bool) {
	t.Helper()

	handle, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	var sink io.Writer = handle
	var gzipped *gzip.Writer
	if compressed {
		gzipped = gzip.NewWriter(handle)
		sink = gzipped
	}

	writer := tar.NewWriter(sink)
	for _, name := range sortedKeys(entries) {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(entries[name])), Typeflag: tar.TypeReg}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(entries[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if gzipped != nil {
		if err := gzipped.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
}

// candidateNames returns the discovered skill names in order.
func candidateNames(source *skillsSource) []string {
	names := make([]string, 0, len(source.candidates))
	for _, candidate := range source.candidates {
		names = append(names, candidate.name)
	}
	return names
}

func TestDirectorySourceDiscoversSkills(t *testing.T) {
	source := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		writeSkill(t, source, name, name)
	}
	writeFile(t, filepath.Join(source, "README.md"), "not a skill\n")

	opened, err := openSkillsSource(source)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	names := candidateNames(opened)
	if !slices.Equal(names, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("candidates = %v", names)
	}
}

func TestSingleSkillDirectoryIsASource(t *testing.T) {
	// The source root itself holds the manifest: the directory is the skill.
	source := filepath.Join(t.TempDir(), "one-skill")
	writeFile(t, filepath.Join(source, "SKILL.md"), skillFile)

	opened, err := openSkillsSource(source)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	names := candidateNames(opened)
	if !slices.Equal(names, []string{"one-skill"}) {
		t.Fatalf("candidates = %v", names)
	}
}

func TestGitHubStyleArchiveIsLocated(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "OpenSpec-1.16.1.zip")
	writeZip(t, archive, map[string]string{
		"OpenSpec-1.16.1/README.md":                        "readme\n",
		"OpenSpec-1.16.1/skills/openspec-explore/SKILL.md": skillFile,
		"OpenSpec-1.16.1/skills/openspec-propose/SKILL.md": skillFile,
		"OpenSpec-1.16.1/src/main.go":                      "package main\n",
	})

	opened, err := openSkillsSource(archive)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	names := candidateNames(opened)
	if !slices.Equal(names, []string{"openspec-explore", "openspec-propose"}) {
		t.Fatalf("candidates = %v", names)
	}
}

func TestTarGzArchiveIsLocated(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "skills.tgz")
	writeTar(t, archive, map[string]string{
		"pack/skills/alpha/SKILL.md": skillFile,
		"pack/skills/alpha/tool.sh":  "#!/bin/sh\n",
	}, true)

	opened, err := openSkillsSource(archive)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if names := candidateNames(opened); !slices.Equal(names, []string{"alpha"}) {
		t.Fatalf("candidates = %v", names)
	}
}

func TestSourceWithNoSkillsIsRefused(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "README.md"), "nothing here\n")

	_, err := openSkillsSource(source)
	if err == nil || !strings.Contains(err.Error(), "no directory with a SKILL.md") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "evil.zip")
	writeZip(t, archive, map[string]string{"../outside/SKILL.md": skillFile})

	_, err := openSkillsSource(archive)
	if err == nil || !strings.Contains(err.Error(), "unsafe entry") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceRejectsLinks(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "links.tar")
	handle, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(handle)
	header := &tar.Header{Name: "alpha/SKILL.md", Mode: 0o644, Size: int64(len(skillFile)), Typeflag: tar.TypeReg}
	if err := writer.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(skillFile)); err != nil {
		t.Fatal(err)
	}
	link := &tar.Header{Name: "alpha/linked", Linkname: "/etc/passwd", Mode: 0o777, Typeflag: tar.TypeSymlink}
	if err := writer.WriteHeader(link); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = openSkillsSource(archive)
	if err == nil || !strings.Contains(err.Error(), "link entry") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceRejectsTooManyEntries(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "big.zip")
	handle, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(handle)
	for index := 0; index <= maxSourceEntries; index++ {
		if _, err := writer.Create("entry-" + strconv.Itoa(index)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = openSkillsSource(archive)
	if err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("error = %v", err)
	}
}

func TestMaterializeWritesOnlySkillDirectories(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "repo.zip")
	writeZip(t, archive, map[string]string{
		"repo/skills/alpha/SKILL.md": skillFile,
		"repo/skills/alpha/notes.md": "notes\n",
		"repo/README.md":             "readme\n",
		"repo/src/main.go":           "package main\n",
	})

	opened, err := openSkillsSource(archive)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	destination := t.TempDir()
	if err := opened.materialize(destination); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "alpha" {
		t.Fatalf("materialized = %v", entries)
	}
	if _, err := os.Stat(filepath.Join(destination, "alpha", "notes.md")); err != nil {
		t.Fatalf("the skill's own file was not written: %v", err)
	}
	for _, leaked := range []string{"README.md", "src", "repo"} {
		if _, err := os.Stat(filepath.Join(destination, leaked)); !os.IsNotExist(err) {
			t.Fatalf("%q was extracted into the project", leaked)
		}
	}
}
