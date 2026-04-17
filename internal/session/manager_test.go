package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestCreateWithExplicitProvider(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	assetsDir := filepath.Join(root, "assets", "firecracker")
	for _, path := range []string{
		filepath.Join(assetsDir, "firecracker"),
		filepath.Join(assetsDir, "hello-vmlinux.bin"),
		filepath.Join(assetsDir, "hello-rootfs.ext4"),
		filepath.Join(root, "dev", "kvm"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir path: %v", err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
			t.Fatalf("write path %s: %v", path, err)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	t.Setenv("AIR_VM_RUNTIME", "local")
	t.Setenv("AIR_FIRECRACKER_BIN", filepath.Join(root, "assets", "firecracker", "firecracker"))
	t.Setenv("AIR_FIRECRACKER_KERNEL", filepath.Join(root, "assets", "firecracker", "hello-vmlinux.bin"))
	t.Setenv("AIR_FIRECRACKER_ROOTFS", filepath.Join(root, "assets", "firecracker", "hello-rootfs.ext4"))
	t.Setenv("AIR_KVM_DEVICE", filepath.Join(root, "dev", "kvm"))

	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	s, err := manager.CreateWithProvider("local")
	if err != nil {
		t.Fatalf("create session with explicit local provider: %v", err)
	}
	if s.Provider != "local" {
		t.Fatalf("expected local provider, got %q", s.Provider)
	}
}

func TestRunCreatesExecutesAndCleansUp(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	result, err := manager.Run("printf hello", RunOptions{})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if result.Provider != "local" {
		t.Fatalf("expected local provider, got %q", result.Provider)
	}
	if result.SessionID == "" {
		t.Fatal("expected session id in run result")
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.ErrorType != RunErrorTypeNone {
		t.Fatalf("expected empty error type, got %q", result.ErrorType)
	}

	items, err := manager.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no remaining sessions after run, got %d", len(items))
	}

	if _, err := os.Stat(filepath.Join(root, "runtime", "sessions", "local", result.SessionID)); !os.IsNotExist(err) {
		t.Fatalf("expected runtime directory removed, stat err=%v", err)
	}
}

func TestRunTimeoutReturnsStructuredResult(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	result, err := manager.Run("sleep 1", RunOptions{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("run timeout command: %v", err)
	}
	if result == nil {
		t.Fatal("expected structured run result")
	}
	if !result.Timeout {
		t.Fatal("expected timeout flag")
	}
	if result.ErrorType != RunErrorTypeTimeout {
		t.Fatalf("expected timeout error type, got %q", result.ErrorType)
	}
	if result.ExitCode != 124 {
		t.Fatalf("expected exit code 124, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "timed out") {
		t.Fatalf("expected timeout stderr, got %q", result.Stderr)
	}
}

func TestRunNonZeroExitReturnsExecError(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	result, err := manager.Run("sh -c 'echo boom >&2; exit 7'", RunOptions{})
	if err != nil {
		t.Fatalf("run non-zero exit command: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", result.ExitCode)
	}
	if result.ErrorType != RunErrorTypeExec {
		t.Fatalf("expected exec error type, got %q", result.ErrorType)
	}
	if !strings.Contains(result.ErrorMessage, "code 7") {
		t.Fatalf("unexpected error message: %q", result.ErrorMessage)
	}
	if strings.TrimSpace(result.Stderr) != "boom" {
		t.Fatalf("unexpected stderr: %q", result.Stderr)
	}
}

func TestRunRejectsInvalidResourceOptions(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	result, err := manager.Run("echo hello", RunOptions{MemoryMiB: -1})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if result == nil {
		t.Fatal("expected structured result")
	}
	if result.ErrorType != RunErrorTypeInvalidArgument {
		t.Fatalf("expected invalid argument error type, got %q", result.ErrorType)
	}
}
