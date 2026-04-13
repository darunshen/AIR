package vm

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecTimeout(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	r, err := New(filepath.Join(root, "runtime"))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if _, err := r.Start("sess_test"); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	result, err := r.Exec("sess_test", "sleep 1", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("exec timeout command: %v", err)
	}
	if result.ExitCode != 124 {
		t.Fatalf("expected timeout exit code 124, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "timed out") {
		t.Fatalf("expected timeout stderr, got %q", result.Stderr)
	}
}
