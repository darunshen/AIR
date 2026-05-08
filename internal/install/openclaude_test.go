package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOfficialOpenClaudeBundleURL(t *testing.T) {
	t.Helper()

	url, err := OfficialOpenClaudeBundleURL("v0.1.1", "linux", "amd64")
	if err != nil {
		t.Fatalf("official openclaude bundle url: %v", err)
	}
	want := "https://github.com/darunshen/AIR/releases/download/v0.1.1/air_openclaude_linux_amd64.tar.gz"
	if url != want {
		t.Fatalf("unexpected url: got %q want %q", url, want)
	}
}

func TestOfficialOpenClaudeBundleURLUsesLatestForDevBuild(t *testing.T) {
	t.Helper()

	url, err := OfficialOpenClaudeBundleURL("dev", "linux", "amd64")
	if err != nil {
		t.Fatalf("official openclaude latest url: %v", err)
	}
	want := "https://github.com/darunshen/AIR/releases/latest/download/air_openclaude_linux_amd64.tar.gz"
	if url != want {
		t.Fatalf("unexpected latest url: got %q want %q", url, want)
	}
}

func TestOfficialOpenClaudeGuestBundleURL(t *testing.T) {
	t.Helper()

	url, err := OfficialOpenClaudeGuestBundleURL("v0.1.1", "linux", "arm64")
	if err != nil {
		t.Fatalf("official guest bundle url: %v", err)
	}
	want := "https://github.com/darunshen/AIR/releases/download/v0.1.1/air_openclaude_firecracker_linux_arm64.tar.gz"
	if url != want {
		t.Fatalf("unexpected url: got %q want %q", url, want)
	}
}

func TestOfficialOpenClaudeGuestBundleURLUsesLatestForDevBuild(t *testing.T) {
	t.Helper()

	url, err := OfficialOpenClaudeGuestBundleURL("dev", "linux", "amd64")
	if err != nil {
		t.Fatalf("official guest bundle latest url: %v", err)
	}
	want := "https://github.com/darunshen/AIR/releases/latest/download/air_openclaude_firecracker_linux_amd64.tar.gz"
	if url != want {
		t.Fatalf("unexpected latest url: got %q want %q", url, want)
	}
}

func TestOfficialOpenClaudeBundleURLRejectsUnsupportedArch(t *testing.T) {
	t.Helper()

	if _, err := OfficialOpenClaudeBundleURL("v0.1.1", "linux", "arm64"); err == nil {
		t.Fatal("expected unsupported architecture error")
	}
}

func TestDownloadOfficialOpenClaudeBundleExtractsBundledLayout(t *testing.T) {
	t.Helper()

	tmp := t.TempDir()
	repoRoot := filepath.Join(tmp, "openclaude")
	if err := os.MkdirAll(filepath.Join(repoRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "scripts", "start-grpc.ts"), []byte("console.log('ok')\n"), 0o644); err != nil {
		t.Fatalf("write grpc script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "package.json"), []byte("{\"name\":\"openclaude\"}\n"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "bin", "bun"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write bun: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, "scripts", "start-grpc.ts")); err != nil {
		t.Fatalf("expected bundled repo layout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "bin", "bun")); err != nil {
		t.Fatalf("expected bundled bun layout: %v", err)
	}
}
