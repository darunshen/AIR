package vm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDirExcludesRuntimeArtifacts(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")

	files := map[string]string{
		"cmd/air/main.go":                          "package main\n",
		"runtime/sessions/firecracker/rootfs.ext4": "large image",
		"runtime/openclaude/bundle.tgz":            "bundle",
		"artifacts/output.log":                     "log",
	}
	for rel, body := range files {
		path := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	if err := copyDir(dst, src, defaultWorkspaceExcludes(), defaultWorkspaceRelExcludes()); err != nil {
		t.Fatalf("copy dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "cmd", "air", "main.go")); err != nil {
		t.Fatalf("expected source file to be copied: %v", err)
	}
	for _, rel := range []string{
		"runtime/sessions",
		"runtime/openclaude",
		"artifacts",
	} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be excluded, stat err=%v", rel, err)
		}
	}
}
