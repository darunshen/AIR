package session

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/darunshen/AIR/internal/vm"
)

func TestOpenClaudeTransportDebugRequiresExplicitFlag(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_LOG_LEVEL", "debug")
	t.Setenv("AIR_OPENCLAUDE_TRANSPORT_DEBUG", "")
	if openClaudeTransportDebugEnabled() {
		t.Fatal("expected openclaude transport debug disabled by default")
	}

	t.Setenv("AIR_OPENCLAUDE_TRANSPORT_DEBUG", "1")
	if !openClaudeTransportDebugEnabled() {
		t.Fatal("expected openclaude transport debug enabled when explicit flag is set")
	}
}

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
	if s.Network != "none" {
		t.Fatalf("expected default network none, got %q", s.Network)
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

func TestSessionExecStreaming(t *testing.T) {
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

	var chunks []vm.ExecChunk
	result, err := manager.ExecStreaming(s.ID, "printf first; sleep 0.1; printf second; >&2 printf boom", 5*time.Second, func(chunk vm.ExecChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("exec streaming: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", result.ExitCode)
	}
	if result.Stdout != "firstsecond" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.Stderr != "boom" {
		t.Fatalf("unexpected stderr: %q", result.Stderr)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected streamed chunks, got %d", len(chunks))
	}
	if chunks[0].Stream != "stdout" || chunks[0].Data != "first" {
		t.Fatalf("unexpected first chunk: %+v", chunks[0])
	}

	foundStderr := false
	for _, chunk := range chunks {
		if chunk.Stream == "stderr" && chunk.Data == "boom" {
			foundStderr = true
			break
		}
	}
	if !foundStderr {
		t.Fatalf("expected stderr chunk in %+v", chunks)
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
	if items[0].Network != "none" {
		t.Fatalf("expected default network none, got %q", items[0].Network)
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

func TestSessionGCDryRunReportsStoppedSessions(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	running, err := manager.Create()
	if err != nil {
		t.Fatalf("create running session: %v", err)
	}
	stopped, err := manager.Create()
	if err != nil {
		t.Fatalf("create stopped session: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "runtime", "sessions", "local", stopped.ID)); err != nil {
		t.Fatalf("remove stopped runtime dir: %v", err)
	}

	result, err := manager.GC(GCOptions{DryRun: true})
	if err != nil {
		t.Fatalf("gc dry-run: %v", err)
	}
	if result.Checked != 2 {
		t.Fatalf("expected checked=2, got %d", result.Checked)
	}
	if result.Removed != 1 {
		t.Fatalf("expected removed=1, got %d", result.Removed)
	}
	if result.Skipped != 1 {
		t.Fatalf("expected skipped=1, got %d", result.Skipped)
	}

	items, err := manager.List()
	if err != nil {
		t.Fatalf("list sessions after dry-run: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected both sessions to remain after dry-run, got %d", len(items))
	}
	if items[0].ID != running.ID && items[1].ID != running.ID {
		t.Fatalf("expected running session %s to remain", running.ID)
	}
}

func TestSessionGCRemovesStoppedSessions(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	running, err := manager.Create()
	if err != nil {
		t.Fatalf("create running session: %v", err)
	}
	stopped, err := manager.Create()
	if err != nil {
		t.Fatalf("create stopped session: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "runtime", "sessions", "local", stopped.ID)); err != nil {
		t.Fatalf("remove stopped runtime dir: %v", err)
	}

	result, err := manager.GC(GCOptions{})
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if result.Removed != 1 {
		t.Fatalf("expected removed=1, got %d", result.Removed)
	}
	if result.Skipped != 1 {
		t.Fatalf("expected skipped=1, got %d", result.Skipped)
	}

	if _, err := manager.store.Get(stopped.ID); err == nil {
		t.Fatalf("expected stopped session %s removed from store", stopped.ID)
	}
	if _, err := manager.store.Get(running.ID); err != nil {
		t.Fatalf("expected running session %s to remain: %v", running.ID, err)
	}
}

func TestSessionGCForceRemovesRunningSessions(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	running, err := manager.Create()
	if err != nil {
		t.Fatalf("create running session: %v", err)
	}

	result, err := manager.GC(GCOptions{Force: true})
	if err != nil {
		t.Fatalf("gc force: %v", err)
	}
	if result.Checked != 1 {
		t.Fatalf("expected checked=1, got %d", result.Checked)
	}
	if result.Removed != 1 {
		t.Fatalf("expected removed=1, got %d", result.Removed)
	}
	if result.Skipped != 0 {
		t.Fatalf("expected skipped=0, got %d", result.Skipped)
	}
	if len(result.Items) != 1 || result.Items[0].Reason != "force cleanup requested" {
		t.Fatalf("unexpected gc items: %+v", result.Items)
	}

	if _, err := manager.store.Get(running.ID); err == nil {
		t.Fatalf("expected running session %s removed from store", running.ID)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "sessions", "local", running.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected runtime directory removed, got err=%v", err)
	}
}

func TestSessionGCSkipsRunningOrphanWithoutForce(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	orphan, err := manager.Create()
	if err != nil {
		t.Fatalf("create orphan candidate: %v", err)
	}
	if err := manager.store.Delete(orphan.ID); err != nil {
		t.Fatalf("delete orphan from store: %v", err)
	}

	result, err := manager.GC(GCOptions{})
	if err != nil {
		t.Fatalf("gc without force: %v", err)
	}
	if result.Checked != 1 {
		t.Fatalf("expected checked=1, got %d", result.Checked)
	}
	if result.Removed != 0 {
		t.Fatalf("expected removed=0, got %d", result.Removed)
	}
	if result.Skipped != 1 {
		t.Fatalf("expected skipped=1, got %d", result.Skipped)
	}
	if len(result.Items) != 1 || result.Items[0].Reason != "orphan runtime is still running" {
		t.Fatalf("unexpected gc items: %+v", result.Items)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "sessions", "local", orphan.ID)); err != nil {
		t.Fatalf("expected orphan runtime directory to remain, got err=%v", err)
	}
}

func TestSessionGCForceRemovesRunningOrphan(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	orphan, err := manager.Create()
	if err != nil {
		t.Fatalf("create orphan candidate: %v", err)
	}
	if err := manager.store.Delete(orphan.ID); err != nil {
		t.Fatalf("delete orphan from store: %v", err)
	}

	result, err := manager.GC(GCOptions{Force: true})
	if err != nil {
		t.Fatalf("gc force on orphan: %v", err)
	}
	if result.Checked != 1 {
		t.Fatalf("expected checked=1, got %d", result.Checked)
	}
	if result.Removed != 1 {
		t.Fatalf("expected removed=1, got %d", result.Removed)
	}
	if result.Skipped != 0 {
		t.Fatalf("expected skipped=0, got %d", result.Skipped)
	}
	if len(result.Items) != 1 || result.Items[0].Reason != "force cleanup requested for orphan runtime" {
		t.Fatalf("unexpected gc items: %+v", result.Items)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "sessions", "local", orphan.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected orphan runtime directory removed, got err=%v", err)
	}
}

func TestSessionGCForceRemovesOrphanFirecrackerProcessDir(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	sessionID := "sess_orphanprocess123"
	oldList := listProcessCmdlines
	oldKill := killPID
	defer func() {
		listProcessCmdlines = oldList
		killPID = oldKill
	}()

	listProcessCmdlines = func() ([]processCmdline, error) {
		return []processCmdline{
			{
				PID: 101,
				Args: []string{
					"/tmp/air-ubuntu-assets/firecracker",
					"--api-sock",
					filepath.Join(root, "runtime", "sessions", "firecracker", sessionID, "firecracker.sock"),
				},
			},
			{
				PID: 202,
				Args: []string{
					"/tmp/air",
					"__firecracker-egress-proxy",
					filepath.Join(root, "runtime", "sessions", "firecracker", sessionID, "firecracker.vsock_18080"),
				},
			},
		}, nil
	}

	var killed []int
	killPID = func(pid int) error {
		killed = append(killed, pid)
		return nil
	}

	result, err := manager.GC(GCOptions{Force: true})
	if err != nil {
		t.Fatalf("gc force orphan process dir: %v", err)
	}
	if result.Checked != 1 {
		t.Fatalf("expected checked=1, got %d", result.Checked)
	}
	if result.Removed != 1 {
		t.Fatalf("expected removed=1, got %d", result.Removed)
	}
	if result.Skipped != 0 {
		t.Fatalf("expected skipped=0, got %d", result.Skipped)
	}
	if len(result.Items) != 1 || result.Items[0].Reason != "force cleanup requested for orphan firecracker processes" {
		t.Fatalf("unexpected gc items: %+v", result.Items)
	}
	if len(killed) != 2 || killed[0] != 202 || killed[1] != 101 {
		t.Fatalf("unexpected killed pids: %+v", killed)
	}
}

func TestCreateWithExplicitProvider(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	assetsDir := filepath.Join(root, "assets", "firecracker")
	for _, path := range []string{
		filepath.Join(assetsDir, "firecracker"),
		filepath.Join(assetsDir, "vmlinux.bin"),
		filepath.Join(assetsDir, "ubuntu-rootfs.ext4"),
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
	t.Setenv("AIR_FIRECRACKER_KERNEL", filepath.Join(root, "assets", "firecracker", "vmlinux.bin"))
	t.Setenv("AIR_FIRECRACKER_ROOTFS", filepath.Join(root, "assets", "firecracker", "ubuntu-rootfs.ext4"))
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

func TestCreateWithOptionsStoresNetwork(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	s, err := manager.CreateWithOptions(CreateOptions{
		Provider: "local",
		Network:  "full",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if s.Network != "full" {
		t.Fatalf("expected full network, got %q", s.Network)
	}

	inspect, err := manager.Inspect(s.ID)
	if err != nil {
		t.Fatalf("inspect session: %v", err)
	}
	if inspect.Session.Network != "full" {
		t.Fatalf("expected inspect session network full, got %q", inspect.Session.Network)
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

func TestNormalizeOpenClaudeStartOptionsCollectsProviderEnv(t *testing.T) {
	t.Helper()

	t.Setenv("CLAUDE_CODE_USE_OPENAI", "1")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_BASE_URL", "https://api.deepseek.com/v1")
	t.Setenv("OPENAI_MODEL", "deepseek-chat")
	t.Setenv("UNRELATED_ENV", "should-not-pass")

	opts := normalizeOpenClaudeStartOptions(OpenClaudeStartOptions{})
	if opts.Env["CLAUDE_CODE_USE_OPENAI"] != "1" {
		t.Fatalf("expected CLAUDE_CODE_USE_OPENAI in env, got %+v", opts.Env)
	}
	if opts.Env["OPENAI_API_KEY"] != "sk-test" {
		t.Fatalf("expected OPENAI_API_KEY in env, got %+v", opts.Env)
	}
	if _, ok := opts.Env["UNRELATED_ENV"]; ok {
		t.Fatalf("did not expect unrelated env to be passed: %+v", opts.Env)
	}
}

func TestRenderOpenClaudeStartCommandIncludesWhitelistedEnv(t *testing.T) {
	t.Helper()

	meta := &openClaudeMetadata{
		SessionID: "sess_123",
		RepoPath:  "/workspace/openclaude",
		Command:   "bun run scripts/start-grpc.ts",
		Host:      "127.0.0.1",
		Port:      50051,
		StateDir:  "/run/air/openclaude/sess_123",
		PIDPath:   "/run/air/openclaude/sess_123/server.pid",
		LogPath:   "/run/air/openclaude/sess_123/server.log",
	}
	command := renderOpenClaudeStartCommand(meta, map[string]string{
		"CLAUDE_CODE_USE_OPENAI": "1",
		"OPENAI_API_KEY":         "sk-test",
		"OPENAI_MODEL":           "deepseek-chat",
	})
	for _, part := range []string{
		"GRPC_HOST='127.0.0.1'",
		"GRPC_PORT='50051'",
		"HOME='/run/air/openclaude/sess_123/home'",
		"CLAUDE_CONFIG_DIR='/run/air/openclaude/sess_123/home/.openclaude'",
		"PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'",
		"SHELL='/bin/bash'",
		"CLAUDE_CODE_SHELL='/bin/bash'",
		"/bin/sh -c 'bun run scripts/start-grpc.ts'",
		"CLAUDE_CODE_USE_OPENAI='1'",
		"OPENAI_API_KEY='sk-test'",
		"OPENAI_MODEL='deepseek-chat'",
	} {
		if !strings.Contains(command, part) {
			t.Fatalf("expected command to contain %q, got %s", part, command)
		}
	}
}

func TestOpenClaudePATHPrefersExplicitEnv(t *testing.T) {
	t.Helper()

	if got := openClaudePATH(map[string]string{"PATH": "/custom/bin:/usr/bin"}); got != "/custom/bin:/usr/bin" {
		t.Fatalf("expected explicit PATH to be preserved, got %q", got)
	}
	if got := openClaudePATH(map[string]string{}); got != defaultOpenClaudePATH {
		t.Fatalf("expected default PATH %q, got %q", defaultOpenClaudePATH, got)
	}
}

func TestApplyOpenClaudeRuntimeEnvAddsFirecrackerProxyDefaults(t *testing.T) {
	t.Helper()

	env := applyOpenClaudeRuntimeEnv("firecracker", map[string]string{
		"OPENAI_API_KEY": "sk-test",
	})
	if env["HTTP_PROXY"] != "http://127.0.0.1:18080" {
		t.Fatalf("expected HTTP_PROXY default, got %+v", env)
	}
	if env["HTTPS_PROXY"] != "http://127.0.0.1:18080" {
		t.Fatalf("expected HTTPS_PROXY default, got %+v", env)
	}
	if env["ALL_PROXY"] != "http://127.0.0.1:18080" {
		t.Fatalf("expected ALL_PROXY default, got %+v", env)
	}
}

func TestApplyOpenClaudeRuntimeEnvPreservesExplicitProxy(t *testing.T) {
	t.Helper()

	env := applyOpenClaudeRuntimeEnv("firecracker", map[string]string{
		"HTTP_PROXY": "http://custom-proxy:8080",
	})
	if env["HTTP_PROXY"] != "http://custom-proxy:8080" {
		t.Fatalf("expected explicit HTTP_PROXY to be preserved, got %+v", env)
	}
}

func TestApplyOpenClaudeRuntimeEnvRewritesLoopbackProxyForFirecracker(t *testing.T) {
	t.Helper()

	env := applyOpenClaudeRuntimeEnv("firecracker", map[string]string{
		"HTTP_PROXY":  "http://127.0.0.1:3067/",
		"HTTPS_PROXY": "http://localhost:3067/",
		"ALL_PROXY":   "socks://127.0.0.1:3067/",
	})
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		if env[key] != "http://127.0.0.1:18080" {
			t.Fatalf("expected %s to be rewritten to guest proxy, got %+v", key, env)
		}
	}
}

func TestApplyOpenClaudeRuntimeEnvStripsAnthropicEnvWhenUsingOpenAI(t *testing.T) {
	t.Helper()

	env := applyOpenClaudeRuntimeEnv("firecracker", map[string]string{
		"CLAUDE_CODE_USE_OPENAI": "1",
		"OPENAI_API_KEY":         "sk-openai",
		"ANTHROPIC_API_KEY":      "sk-anthropic",
		"ANTHROPIC_BASE_URL":     "https://api.routeai.cc",
	})
	if _, ok := env["ANTHROPIC_API_KEY"]; ok {
		t.Fatalf("expected ANTHROPIC_API_KEY to be stripped, got %+v", env)
	}
	if _, ok := env["ANTHROPIC_BASE_URL"]; ok {
		t.Fatalf("expected ANTHROPIC_BASE_URL to be stripped, got %+v", env)
	}
	if env["OPENAI_API_KEY"] != "sk-openai" {
		t.Fatalf("expected OPENAI_API_KEY to be preserved, got %+v", env)
	}
}

func TestApplyOpenClaudeRuntimeEnvMapsAnthropicAuthTokenToAPIKeyForOpenClaude(t *testing.T) {
	t.Helper()

	env := applyOpenClaudeRuntimeEnv("firecracker", map[string]string{
		"ANTHROPIC_AUTH_TOKEN": "sk-anthropic",
		"ANTHROPIC_BASE_URL":   "https://api.deepseek.com/anthropic",
		"ANTHROPIC_MODEL":      "deepseek-v4-pro",
		"OPENAI_API_KEY":       "sk-openai",
	})
	if _, ok := env["ANTHROPIC_AUTH_TOKEN"]; ok {
		t.Fatalf("expected ANTHROPIC_AUTH_TOKEN to be removed before OpenClaude launch, got %+v", env)
	}
	if env["ANTHROPIC_API_KEY"] != "sk-anthropic" {
		t.Fatalf("expected ANTHROPIC_API_KEY mirrored from auth token, got %+v", env)
	}
	if _, ok := env["OPENAI_API_KEY"]; ok {
		t.Fatalf("expected OPENAI_API_KEY to be stripped in anthropic mode, got %+v", env)
	}
}

func TestBuildOpenClaudeGlobalConfigIncludesManagedAnthropicKey(t *testing.T) {
	t.Helper()

	apiKey := "sk-1234567890ABCDEFGHIJKLMN"
	body := buildOpenClaudeGlobalConfig(map[string]string{
		"ANTHROPIC_API_KEY":  apiKey,
		"ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic",
		"ANTHROPIC_MODEL":    "deepseek-v4-pro",
		"HTTP_PROXY":         "http://127.0.0.1:18080",
	})

	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if got := decoded["primaryApiKey"]; got != apiKey {
		t.Fatalf("expected primaryApiKey %q, got %#v", apiKey, got)
	}

	custom, ok := decoded["customApiKeyResponses"].(map[string]any)
	if !ok {
		t.Fatalf("expected customApiKeyResponses object, got %#v", decoded["customApiKeyResponses"])
	}
	approved, ok := custom["approved"].([]any)
	if !ok || len(approved) != 1 {
		t.Fatalf("expected approved list, got %#v", custom["approved"])
	}
	if got := approved[0]; got != normalizeOpenClaudeAPIKeyForConfig(apiKey) {
		t.Fatalf("unexpected approved key suffix: %#v", got)
	}

	env, ok := decoded["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env object, got %#v", decoded["env"])
	}
	if got := env["ANTHROPIC_MODEL"]; got != "deepseek-v4-pro" {
		t.Fatalf("expected ANTHROPIC_MODEL in env, got %#v", got)
	}
}

func TestBuildOpenClaudeDiagnosticConfigMasksSecretsAndIncludesMode(t *testing.T) {
	t.Helper()

	meta := &openClaudeMetadata{
		SessionID: "sess_diag",
		Provider:  "firecracker",
		RepoPath:  "/opt/openclaude",
		Host:      "127.0.0.1",
		Port:      50051,
	}
	body := buildOpenClaudeDiagnosticConfig(map[string]string{
		"ANTHROPIC_AUTH_TOKEN": "sk-1234567890ABCDEFGHIJKLMN",
		"ANTHROPIC_BASE_URL":   "https://api.deepseek.com/anthropic",
		"ANTHROPIC_MODEL":      "deepseek-v4-pro",
		"HTTP_PROXY":           "http://127.0.0.1:18080",
	}, meta)

	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("unmarshal diagnostic: %v", err)
	}

	if got := decoded["event"]; got != "air_openclaude_launch" {
		t.Fatalf("unexpected event: %#v", got)
	}

	selected, ok := decoded["selected_env"].(map[string]any)
	if !ok {
		t.Fatalf("expected selected_env object, got %#v", decoded["selected_env"])
	}
	if got := selected["ANTHROPIC_MODEL"]; got != "deepseek-v4-pro" {
		t.Fatalf("unexpected ANTHROPIC_MODEL: %#v", got)
	}
	if got := selected["ANTHROPIC_AUTH_TOKEN"]; got != normalizeOpenClaudeAPIKeyForConfig("sk-1234567890ABCDEFGHIJKLMN") {
		t.Fatalf("unexpected masked auth token: %#v", got)
	}
	if _, ok := selected["ANTHROPIC_API_KEY"]; ok {
		t.Fatalf("expected ANTHROPIC_API_KEY to remain absent, got %#v", selected["ANTHROPIC_API_KEY"])
	}
}

func TestNormalizeOpenClaudeAPIKeyForConfig(t *testing.T) {
	t.Helper()

	if got := normalizeOpenClaudeAPIKeyForConfig("short-key"); got != "short-key" {
		t.Fatalf("expected short key unchanged, got %q", got)
	}
	if got := normalizeOpenClaudeAPIKeyForConfig("1234567890123456789012345"); got != "67890123456789012345" {
		t.Fatalf("unexpected normalized key: %q", got)
	}
}

func TestOpenClaudeStateDirForFirecrackerUsesWorkspace(t *testing.T) {
	t.Helper()

	got := openClaudeStateDirForProvider("firecracker", "/opt/openclaude", "sess_123")
	want := "/workspace/.air/openclaude/sess_123"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
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
