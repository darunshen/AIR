package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficialFirecrackerBundleURL(t *testing.T) {
	t.Helper()

	url, err := OfficialFirecrackerBundleURL("v0.1.1", "linux", "amd64")
	if err != nil {
		t.Fatalf("official bundle url: %v", err)
	}
	want := "https://github.com/darunshen/AIR/releases/download/v0.1.1/air_firecracker_linux_amd64.tar.gz"
	if url != want {
		t.Fatalf("unexpected url: got %q want %q", url, want)
	}
}

func TestOfficialFirecrackerBundleURLUsesLatestForDevBuild(t *testing.T) {
	t.Helper()

	url, err := OfficialFirecrackerBundleURL("dev", "linux", "arm64")
	if err != nil {
		t.Fatalf("official bundle latest url: %v", err)
	}
	want := "https://github.com/darunshen/AIR/releases/latest/download/air_firecracker_linux_arm64.tar.gz"
	if url != want {
		t.Fatalf("unexpected latest url: got %q want %q", url, want)
	}
}

func TestExtractTarGz(t *testing.T) {
	t.Helper()

	var archive bytes.Buffer
	gzw := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gzw)

	writeFile := func(name, body string, mode int64) {
		header := &tar.Header{
			Name: name,
			Mode: mode,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}

	writeFile("firecracker", "bin", 0o755)
	writeFile("hello-vmlinux.bin", "kernel", 0o644)
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	outputDir := t.TempDir()
	if err := extractTarGz(bytes.NewReader(archive.Bytes()), outputDir); err != nil {
		t.Fatalf("extract tar.gz: %v", err)
	}

	for _, name := range []string{"firecracker", "hello-vmlinux.bin"} {
		path := filepath.Join(outputDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func TestExtractTarGzSupportsHardLinks(t *testing.T) {
	t.Helper()

	var archive bytes.Buffer
	gzw := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gzw)

	fileHeader := &tar.Header{
		Name: "openclaude/package.json",
		Mode: 0o644,
		Size: int64(len("{}\n")),
	}
	if err := tw.WriteHeader(fileHeader); err != nil {
		t.Fatalf("write file header: %v", err)
	}
	if _, err := tw.Write([]byte("{}\n")); err != nil {
		t.Fatalf("write file body: %v", err)
	}

	linkHeader := &tar.Header{
		Name:     "openclaude/node_modules/pkg/package.json",
		Typeflag: tar.TypeLink,
		Linkname: "openclaude/package.json",
	}
	if err := tw.WriteHeader(linkHeader); err != nil {
		t.Fatalf("write hardlink header: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	outputDir := t.TempDir()
	if err := extractTarGz(bytes.NewReader(archive.Bytes()), outputDir); err != nil {
		t.Fatalf("extract tar.gz with hardlinks: %v", err)
	}

	path := filepath.Join(outputDir, "openclaude", "node_modules", "pkg", "package.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extracted hardlink target: %v", err)
	}
	if string(body) != "{}\n" {
		t.Fatalf("unexpected hardlink body: %q", string(body))
	}
}

func TestExtractTarGzSupportsSymlinks(t *testing.T) {
	t.Helper()

	var archive bytes.Buffer
	gzw := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gzw)

	fileHeader := &tar.Header{
		Name: "openclaude/package.json",
		Mode: 0o644,
		Size: int64(len("{}\n")),
	}
	if err := tw.WriteHeader(fileHeader); err != nil {
		t.Fatalf("write file header: %v", err)
	}
	if _, err := tw.Write([]byte("{}\n")); err != nil {
		t.Fatalf("write file body: %v", err)
	}

	linkHeader := &tar.Header{
		Name:     "openclaude/current-package.json",
		Typeflag: tar.TypeSymlink,
		Linkname: "package.json",
	}
	if err := tw.WriteHeader(linkHeader); err != nil {
		t.Fatalf("write symlink header: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	outputDir := t.TempDir()
	if err := extractTarGz(bytes.NewReader(archive.Bytes()), outputDir); err != nil {
		t.Fatalf("extract tar.gz with symlinks: %v", err)
	}

	path := filepath.Join(outputDir, "openclaude", "current-package.json")
	linkTarget, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("read extracted symlink: %v", err)
	}
	if linkTarget != "package.json" {
		t.Fatalf("unexpected symlink target: got %q want %q", linkTarget, "package.json")
	}
}

func TestBuildCustomInstallGuide(t *testing.T) {
	t.Helper()

	guide := BuildCustomInstallGuide("/tmp/air/firecracker")
	if !strings.Contains(guide, "/tmp/air/firecracker") {
		t.Fatalf("expected output dir in guide: %s", guide)
	}
	if !strings.Contains(guide, "air doctor --provider firecracker --human") {
		t.Fatalf("expected doctor hint in guide: %s", guide)
	}
}
