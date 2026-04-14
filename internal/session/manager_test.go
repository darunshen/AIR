package session

import (
	"os"
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
	if s.Provider != "local" {
		t.Fatalf("expected local provider, got %q", s.Provider)
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

func TestSessionListAndInspect(t *testing.T) {
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

	items, err := manager.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 session, got %d", len(items))
	}
	if items[0].ID != s.ID {
		t.Fatalf("unexpected session id: %s", items[0].ID)
	}
	if items[0].Provider != "local" {
		t.Fatalf("expected local provider, got %q", items[0].Provider)
	}

	inspect, err := manager.Inspect(s.ID)
	if err != nil {
		t.Fatalf("inspect session: %v", err)
	}
	if inspect.Session.ID != s.ID {
		t.Fatalf("unexpected inspect session id: %s", inspect.Session.ID)
	}
	if inspect.Runtime.Provider != "local" {
		t.Fatalf("unexpected runtime provider: %s", inspect.Runtime.Provider)
	}
	if !inspect.Runtime.Exists || !inspect.Runtime.Running {
		t.Fatalf("expected existing running runtime, got exists=%v running=%v", inspect.Runtime.Exists, inspect.Runtime.Running)
	}
	if inspect.Runtime.WorkspacePath == "" || inspect.Runtime.TaskPath == "" {
		t.Fatalf("expected local runtime paths, got %+v", inspect.Runtime)
	}

	if _, err := manager.ConsolePath(s.ID); err == nil {
		t.Fatal("expected local session to have no console path")
	}
}

func TestSessionListBackfillsMissingProvider(t *testing.T) {
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

	s.Provider = ""
	if err := manager.store.Save(s); err != nil {
		t.Fatalf("save session without provider: %v", err)
	}

	items, err := manager.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if items[0].Provider != "local" {
		t.Fatalf("expected provider backfilled to local, got %q", items[0].Provider)
	}

	inspect, err := manager.Inspect(s.ID)
	if err != nil {
		t.Fatalf("inspect session: %v", err)
	}
	if inspect.Session.Provider != "local" {
		t.Fatalf("expected inspect provider backfilled to local, got %q", inspect.Session.Provider)
	}
}

func TestSessionListRefreshesStoppedStatus(t *testing.T) {
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

	if err := os.RemoveAll(filepath.Join(root, "runtime", "sessions", "local", s.ID)); err != nil {
		t.Fatalf("remove runtime directory: %v", err)
	}

	items, err := manager.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if items[0].Status != "stopped" {
		t.Fatalf("expected stopped status after runtime removal, got %q", items[0].Status)
	}

	inspect, err := manager.Inspect(s.ID)
	if err != nil {
		t.Fatalf("inspect session: %v", err)
	}
	if inspect.Session.Status != "stopped" {
		t.Fatalf("expected inspect to report stopped status, got %q", inspect.Session.Status)
	}
	if inspect.Runtime.Exists || inspect.Runtime.Running {
		t.Fatalf("expected missing runtime, got exists=%v running=%v", inspect.Runtime.Exists, inspect.Runtime.Running)
	}
}
