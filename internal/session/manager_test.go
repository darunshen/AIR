package session

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestExportWorkspace(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	workspace := filepath.Join(root, "host-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "input.txt"), []byte("host\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	s, err := manager.CreateWithOptions(CreateOptions{
		Provider:      "local",
		WorkspacePath: workspace,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if _, err := manager.Exec(s.ID, "echo guest > output.txt && mkdir -p nested && echo note > nested/note.txt"); err != nil {
		t.Fatalf("exec workspace mutation: %v", err)
	}

	exportDir := filepath.Join(root, "exported")
	result, err := manager.ExportWorkspace(s.ID, exportDir, false)
	if err != nil {
		t.Fatalf("export workspace: %v", err)
	}
	if result.SessionID != s.ID {
		t.Fatalf("unexpected session id: %s", result.SessionID)
	}
	if result.Provider != "local" {
		t.Fatalf("unexpected provider: %s", result.Provider)
	}
	if result.OutputPath != exportDir {
		t.Fatalf("unexpected output path: %s", result.OutputPath)
	}

	inputBody, err := os.ReadFile(filepath.Join(exportDir, "input.txt"))
	if err != nil {
		t.Fatalf("read exported input: %v", err)
	}
	if strings.TrimSpace(string(inputBody)) != "host" {
		t.Fatalf("unexpected exported input: %q", string(inputBody))
	}
	outputBody, err := os.ReadFile(filepath.Join(exportDir, "output.txt"))
	if err != nil {
		t.Fatalf("read exported output: %v", err)
	}
	if strings.TrimSpace(string(outputBody)) != "guest" {
		t.Fatalf("unexpected exported output: %q", string(outputBody))
	}
	nestedBody, err := os.ReadFile(filepath.Join(exportDir, "nested", "note.txt"))
	if err != nil {
		t.Fatalf("read nested note: %v", err)
	}
	if strings.TrimSpace(string(nestedBody)) != "note" {
		t.Fatalf("unexpected nested note: %q", string(nestedBody))
	}

	hostOutputPath := filepath.Join(workspace, "output.txt")
	if _, err := os.Stat(hostOutputPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected host workspace to stay unchanged, got err=%v", err)
	}
}

func TestExportWorkspaceRequiresForceForNonEmptyDirectory(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	s, err := manager.CreateWithProvider("local")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	exportDir := filepath.Join(root, "exported")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		t.Fatalf("mkdir export dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(exportDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}

	if _, err := manager.ExportWorkspace(s.ID, exportDir, false); err == nil {
		t.Fatal("expected non-empty directory error")
	}

	if _, err := manager.Exec(s.ID, "echo fresh > exported.txt"); err != nil {
		t.Fatalf("exec workspace mutation: %v", err)
	}
	if _, err := manager.ExportWorkspace(s.ID, exportDir, true); err != nil {
		t.Fatalf("export workspace with force: %v", err)
	}
	if _, err := os.Stat(filepath.Join(exportDir, "keep.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected keep.txt removed by force export, got err=%v", err)
	}
	if body, err := os.ReadFile(filepath.Join(exportDir, "exported.txt")); err != nil {
		t.Fatalf("read exported.txt: %v", err)
	} else if strings.TrimSpace(string(body)) != "fresh" {
		t.Fatalf("unexpected exported.txt content: %q", string(body))
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

func TestOpenClaudeLifecycleUsesSessionManagedProcess(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	repoPath := filepath.Join(root, "openclaude")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}

	status, err := manager.StartOpenClaude(OpenClaudeStartOptions{
		Provider: "local",
		RepoPath: repoPath,
		Command:  "while true; do sleep 1; done",
		Host:     "127.0.0.1",
		Port:     50051,
	})
	if err != nil {
		t.Fatalf("start openclaude: %v", err)
	}
	if status.SessionID == "" {
		t.Fatal("expected session id")
	}
	if !status.CreatedSession {
		t.Fatal("expected start to create a session")
	}
	if !status.Running {
		t.Fatal("expected openclaude to be running")
	}
	if status.PID <= 0 {
		t.Fatalf("expected pid, got %d", status.PID)
	}
	if status.LogPath == "" || status.PIDPath == "" || status.MetadataPath == "" {
		t.Fatalf("expected status paths, got %+v", status)
	}

	checked, err := manager.OpenClaudeStatus(status.SessionID)
	if err != nil {
		t.Fatalf("openclaude status: %v", err)
	}
	if !checked.Running {
		t.Fatalf("expected running status, got %+v", checked)
	}
	if checked.PID != status.PID {
		t.Fatalf("expected same pid %d, got %d", status.PID, checked.PID)
	}

	stopped, err := manager.StopOpenClaude(status.SessionID)
	if err != nil {
		t.Fatalf("stop openclaude: %v", err)
	}
	if stopped.Running {
		t.Fatalf("expected stopped status, got %+v", stopped)
	}
	if stopped.StoppedAt == nil {
		t.Fatal("expected stopped_at")
	}

	checked, err = manager.OpenClaudeStatus(status.SessionID)
	if err != nil {
		t.Fatalf("openclaude status after stop: %v", err)
	}
	if checked.Running {
		t.Fatalf("expected stopped after stop, got %+v", checked)
	}

	if err := manager.Delete(status.SessionID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
}

func TestOpenClaudeRequiresConfiguration(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if _, err := manager.StartOpenClaude(OpenClaudeStartOptions{Provider: "local"}); err == nil {
		t.Fatal("expected missing repo path error")
	}

	s, err := manager.Create()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := manager.OpenClaudeStatus(s.ID); !errors.Is(err, ErrOpenClaudeNotConfigured) {
		t.Fatalf("expected ErrOpenClaudeNotConfigured, got %v", err)
	}
}

func TestResolveOpenClaudeRepoPathUsesGuestDefaultForFirecracker(t *testing.T) {
	t.Helper()

	got := resolveOpenClaudeRepoPath("firecracker", OpenClaudeStartOptions{})
	if got != "/opt/openclaude" {
		t.Fatalf("expected firecracker default guest repo, got %q", got)
	}

	got = resolveOpenClaudeRepoPath("firecracker", OpenClaudeStartOptions{
		RepoPath: "/host/openclaude",
	})
	if got != "/host/openclaude" {
		t.Fatalf("expected explicit repo path to be reused when guest path is absent, got %q", got)
	}

	got = resolveOpenClaudeRepoPath("firecracker", OpenClaudeStartOptions{
		RepoPath:      "/host/openclaude",
		GuestRepoPath: "/guest/openclaude",
	})
	if got != "/guest/openclaude" {
		t.Fatalf("expected guest repo path to win, got %q", got)
	}

	got = resolveOpenClaudeRepoPath("local", OpenClaudeStartOptions{
		RepoPath:      "/host/openclaude",
		GuestRepoPath: "/guest/openclaude",
	})
	if got != "/host/openclaude" {
		t.Fatalf("expected local repo path to win, got %q", got)
	}
}

func TestDeleteStopsManagedOpenClaudeProcess(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	repoPath := filepath.Join(root, "openclaude")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}

	status, err := manager.StartOpenClaude(OpenClaudeStartOptions{
		Provider: "local",
		RepoPath: repoPath,
		Command:  "while true; do sleep 1; done",
	})
	if err != nil {
		t.Fatalf("start openclaude: %v", err)
	}
	if !status.Running || status.PID <= 0 {
		t.Fatalf("expected running process, got %+v", status)
	}

	if err := manager.Delete(status.SessionID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !testProcessExists(status.PID) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected managed process %d to stop after session delete", status.PID)
}

func TestDialOpenClaudeWithLocalProvider(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		if string(buf) == "ping" {
			_, _ = conn.Write([]byte("pong"))
		}
	}()

	repoPath := filepath.Join(root, "openclaude")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}

	started, err := manager.StartOpenClaude(OpenClaudeStartOptions{
		Provider: "local",
		RepoPath: repoPath,
		Command:  "while true; do sleep 1; done",
		Host:     "127.0.0.1",
		Port:     listener.Addr().(*net.TCPAddr).Port,
	})
	if err != nil {
		t.Fatalf("start openclaude: %v", err)
	}
	defer func() {
		_, _ = manager.StopOpenClaude(started.SessionID)
		_ = manager.Delete(started.SessionID)
	}()

	conn, status, err := manager.DialOpenClaude(started.SessionID, 2*time.Second)
	if err != nil {
		t.Fatalf("dial openclaude: %v", err)
	}
	defer conn.Close()
	if !status.Running {
		t.Fatalf("expected running status, got %+v", status)
	}

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write to openclaude conn: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read from openclaude conn: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("unexpected proxy response: %q", string(buf))
	}
}

func testProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
