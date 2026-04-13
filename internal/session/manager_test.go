package session

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	s, err := manager.Create()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if s.ID == "" {
		t.Fatal("expected session id")
	}

	if _, err := manager.Exec(s.ID, "echo hello > a.txt"); err != nil {
		t.Fatalf("exec write file: %v", err)
	}

	result, err := manager.Exec(s.ID, "cat a.txt")
	if err != nil {
		t.Fatalf("exec read file: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}

	failResult, err := manager.Exec(s.ID, "sh -c 'echo boom >&2; exit 7'")
	if err != nil {
		t.Fatalf("exec failing command: %v", err)
	}
	if failResult.ExitCode != 7 {
		t.Fatalf("expected exit 7, got %d", failResult.ExitCode)
	}
	if strings.TrimSpace(failResult.Stderr) != "boom" {
		t.Fatalf("unexpected stderr: %q", failResult.Stderr)
	}

	if err := manager.Delete(s.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if _, err := manager.Exec(s.ID, "pwd"); err == nil {
		t.Fatal("expected missing session after delete")
	}
}
