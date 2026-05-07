package install

import "testing"

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

func TestOfficialOpenClaudeBundleURLRejectsUnsupportedArch(t *testing.T) {
	t.Helper()

	if _, err := OfficialOpenClaudeBundleURL("v0.1.1", "linux", "arm64"); err == nil {
		t.Fatal("expected unsupported architecture error")
	}
}
