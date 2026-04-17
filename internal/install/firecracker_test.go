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
