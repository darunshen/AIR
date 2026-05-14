package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darunshen/AIR/internal/session"
)

func TestParseSessionCreateFlags(t *testing.T) {
	t.Helper()

	tests := []struct {
		name    string
		args    []string
		want    sessionCreateCLIOptions
		wantErr bool
	}{
		{
			name: "empty",
			args: nil,
			want: sessionCreateCLIOptions{},
		},
		{
			name: "local",
			args: []string{"--provider", "local"},
			want: sessionCreateCLIOptions{Provider: "local"},
		},
		{
			name: "firecracker",
			args: []string{"--provider", "firecracker"},
			want: sessionCreateCLIOptions{Provider: "firecracker"},
		},
		{
			name: "workspace",
			args: []string{"--provider", "firecracker", "--workspace", "/tmp/repo"},
			want: sessionCreateCLIOptions{Provider: "firecracker", WorkspacePath: "/tmp/repo"},
		},
		{
			name: "network",
			args: []string{"--provider", "firecracker", "--network", "full"},
			want: sessionCreateCLIOptions{Provider: "firecracker", Network: "full"},
		},
		{
			name: "memory",
			args: []string{"--provider", "firecracker", "--memory-mib", "2048"},
			want: sessionCreateCLIOptions{Provider: "firecracker", MemoryMiB: 2048},
		},
		{
			name: "storage",
			args: []string{"--provider", "firecracker", "--storage-mib", "4096"},
			want: sessionCreateCLIOptions{Provider: "firecracker", StorageMiB: 4096},
		},
		{
			name:    "missing value",
			args:    []string{"--provider"},
			wantErr: true,
		},
		{
			name:    "wrong flag",
			args:    []string{"--runtime", "firecracker"},
			wantErr: true,
		},
		{
			name:    "extra args",
			args:    []string{"--provider", "firecracker", "--extra"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSessionCreateFlags(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestParseSessionExecFlags(t *testing.T) {
	t.Helper()

	opts, err := parseSessionExecFlags([]string{"sess_123", "--timeout", "5m", "echo hello"})
	if err != nil {
		t.Fatalf("parse session exec flags: %v", err)
	}
	if opts.SessionID != "sess_123" {
		t.Fatalf("unexpected session id: %q", opts.SessionID)
	}
	if opts.Timeout != 5*time.Minute {
		t.Fatalf("unexpected timeout: %s", opts.Timeout)
	}
	if opts.Command != "echo hello" {
		t.Fatalf("unexpected command: %q", opts.Command)
	}
}

func TestParseSessionExecFlagsDefaultsTimeout(t *testing.T) {
	t.Helper()

	opts, err := parseSessionExecFlags([]string{"sess_123", "echo hello"})
	if err != nil {
		t.Fatalf("parse session exec flags: %v", err)
	}
	if opts.Timeout != 30*time.Second {
		t.Fatalf("unexpected default timeout: %s", opts.Timeout)
	}
}

func TestParseSessionExecFlagsRejectsInvalidInput(t *testing.T) {
	t.Helper()

	tests := [][]string{
		{"sess_123", "--timeout", "0s", "echo hello"},
		{"sess_123", "--bad", "echo hello"},
		{"sess_123"},
	}
	for _, args := range tests {
		if _, err := parseSessionExecFlags(args); err == nil {
			t.Fatalf("expected error for args=%v", args)
		}
	}
}

func TestParseSessionPTYFlags(t *testing.T) {
	t.Helper()

	opts, err := parseSessionPTYFlags([]string{"sess_123", "bash -lc 'tty'"})
	if err != nil {
		t.Fatalf("parse session pty flags: %v", err)
	}
	if opts.SessionID != "sess_123" {
		t.Fatalf("unexpected session id: %q", opts.SessionID)
	}
	if opts.Command != "bash -lc 'tty'" {
		t.Fatalf("unexpected command: %q", opts.Command)
	}
}

func TestParseSessionPTYFlagsRejectsInvalidInput(t *testing.T) {
	t.Helper()

	for _, args := range [][]string{
		nil,
		{"sess_123"},
		{"", "bash"},
		{"sess_123", ""},
		{"sess_123", "bash", "extra"},
	} {
		if _, err := parseSessionPTYFlags(args); err == nil {
			t.Fatalf("expected error for args=%v", args)
		}
	}
}

func TestParseSessionGCFlags(t *testing.T) {
	t.Helper()

	opts, err := parseSessionGCFlags([]string{"--dry-run", "--force", "--root", "/tmp/runtime/sessions"})
	if err != nil {
		t.Fatalf("parse session gc flags: %v", err)
	}
	if !opts.DryRun {
		t.Fatal("expected dry-run=true")
	}
	if !opts.Force {
		t.Fatal("expected force=true")
	}
	if opts.Root != "/tmp/runtime/sessions" {
		t.Fatalf("unexpected root: %q", opts.Root)
	}

	opts, err = parseSessionGCFlags(nil)
	if err != nil {
		t.Fatalf("parse empty session gc flags: %v", err)
	}
	if opts.DryRun {
		t.Fatal("expected default dry-run=false")
	}
	if opts.Force {
		t.Fatal("expected default force=false")
	}
}

func TestParseSessionGCFlagsRejectsUnknownFlag(t *testing.T) {
	t.Helper()

	if _, err := parseSessionGCFlags([]string{"--bad"}); err == nil {
		t.Fatal("expected error for unknown gc flag")
	}
	if _, err := parseSessionGCFlags([]string{"--root"}); err == nil {
		t.Fatal("expected error for missing root value")
	}
}

func TestCopyTailLines(t *testing.T) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "log.txt")
	body := "1\n2\n3\n4\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	var out strings.Builder
	offset, err := copyTailLines(path, 2, &out)
	if err != nil {
		t.Fatalf("copy tail: %v", err)
	}
	if out.String() != "3\n4\n" {
		t.Fatalf("unexpected tail output: %q", out.String())
	}
	if offset != int64(len(body)) {
		t.Fatalf("unexpected offset: %d", offset)
	}
}

func TestParseRunFlagsSupportsResourceOverrides(t *testing.T) {
	t.Helper()

	opts, command, err := parseRunFlags([]string{
		"--provider", "firecracker",
		"--network", "full",
		"--timeout", "45s",
		"--memory-mib", "512",
		"--storage-mib", "4096",
		"--vcpu-count", "2",
		"--",
		"echo", "hello",
	})
	if err != nil {
		t.Fatalf("parse run flags: %v", err)
	}
	if opts.Provider != "firecracker" {
		t.Fatalf("unexpected provider: %s", opts.Provider)
	}
	if opts.Timeout != 45*time.Second {
		t.Fatalf("unexpected timeout: %s", opts.Timeout)
	}
	if opts.Network != "full" {
		t.Fatalf("unexpected network: %s", opts.Network)
	}
	if opts.MemoryMiB != 512 {
		t.Fatalf("unexpected memory: %d", opts.MemoryMiB)
	}
	if opts.StorageMiB != 4096 {
		t.Fatalf("unexpected storage: %d", opts.StorageMiB)
	}
	if opts.VCPUCount != 2 {
		t.Fatalf("unexpected vcpu count: %d", opts.VCPUCount)
	}
	if command != "echo hello" {
		t.Fatalf("unexpected command: %q", command)
	}
}

func TestParseRunFlagsRejectsInvalidResources(t *testing.T) {
	t.Helper()

	tests := [][]string{
		{"--memory-mib", "0", "--", "echo", "hello"},
		{"--vcpu-count", "-1", "--", "echo", "hello"},
	}

	for _, args := range tests {
		if _, _, err := parseRunFlags(args); err == nil {
			t.Fatalf("expected resource validation error for args=%v", args)
		}
	}
}

func TestChatProfileSaveAndLoad(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	profile := &chatProfile{
		OpenAIBaseURL: "https://api.deepseek.com/v1",
		OpenAIModel:   "deepseek-chat",
		OpenAIAPIKey:  "sk-test",
	}
	if err := saveChatProfile(profile); err != nil {
		t.Fatalf("save chat profile: %v", err)
	}

	path, err := chatProfilePath()
	if err != nil {
		t.Fatalf("chat profile path: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read chat profile file: %v", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal saved profile: %v", err)
	}
	if raw["openai_api_key"] != "sk-test" {
		t.Fatalf("unexpected saved api key field: %+v", raw)
	}

	loaded, err := loadChatProfile()
	if err != nil {
		t.Fatalf("load chat profile: %v", err)
	}
	if *loaded != *profile {
		t.Fatalf("expected %+v, got %+v", *profile, *loaded)
	}
}

func TestParseChatFlagsSupportsReconfigure(t *testing.T) {
	t.Helper()

	opts, err := parseChatFlags([]string{
		"--provider", "firecracker",
		"--network", "full",
		"--memory-mib", "2048",
		"--storage-mib", "4096",
		"--workspace", "/tmp/repo",
		"--listen", "127.0.0.1:50060",
		"--reconfigure",
	})
	if err != nil {
		t.Fatalf("parse chat flags: %v", err)
	}
	if !opts.Reconfigure {
		t.Fatal("expected reconfigure=true")
	}
	if opts.Provider != "firecracker" {
		t.Fatalf("unexpected provider: %q", opts.Provider)
	}
	if opts.Network != "full" {
		t.Fatalf("unexpected network: %q", opts.Network)
	}
	if opts.MemoryMiB != 2048 {
		t.Fatalf("unexpected memory: %d", opts.MemoryMiB)
	}
	if opts.StorageMiB != 4096 {
		t.Fatalf("unexpected storage: %d", opts.StorageMiB)
	}
	if opts.WorkspacePath != "/tmp/repo" {
		t.Fatalf("unexpected workspace: %q", opts.WorkspacePath)
	}
	if opts.ListenAddress != "127.0.0.1:50060" {
		t.Fatalf("unexpected listen address: %q", opts.ListenAddress)
	}
}

func TestParseChatFlagsSupportsPTY(t *testing.T) {
	t.Helper()

	opts, err := parseChatFlags([]string{"--pty"})
	if err != nil {
		t.Fatalf("parse chat flags: %v", err)
	}
	if !opts.PTY {
		t.Fatal("expected pty=true")
	}
}

func TestParseChatGCFlags(t *testing.T) {
	t.Helper()

	opts, err := parseChatGCFlags([]string{"--dry-run", "--force"})
	if err != nil {
		t.Fatalf("parse chat gc flags: %v", err)
	}
	if !opts.DryRun {
		t.Fatal("expected dry-run=true")
	}
	if !opts.Force {
		t.Fatal("expected force=true")
	}
}

func TestParseGCFlags(t *testing.T) {
	t.Helper()

	opts, err := parseGCFlags([]string{"--all"})
	if err != nil {
		t.Fatalf("parse gc flags: %v", err)
	}
	if !opts.All {
		t.Fatal("expected all=true")
	}
	if _, err := parseGCFlags([]string{"--bad"}); err == nil {
		t.Fatal("expected error for unknown gc flag")
	}
}

func TestGCAirChatProcesses(t *testing.T) {
	t.Helper()

	oldList := sessionListProcessCmdlines
	oldKill := sessionKillPID
	defer func() {
		sessionListProcessCmdlines = oldList
		sessionKillPID = oldKill
	}()

	sessionListProcessCmdlines = func() ([]hostProcessInfo, error) {
		return []hostProcessInfo{
			{PID: 10, PPID: 1, Args: []string{"/tmp/air", "chat", "--provider", "firecracker"}},
			{PID: 11, PPID: 10, Args: []string{"bun", "run", "/tmp/air-openclaude-chat-123.mjs"}},
			{PID: 20, PPID: 1, Args: []string{"/tmp/air", "session", "list"}},
		}, nil
	}

	var killed []int
	sessionKillPID = func(pid int) error {
		killed = append(killed, pid)
		return nil
	}

	result, err := gcAirChatProcesses(chatGCCLIOptions{Force: true})
	if err != nil {
		t.Fatalf("gc air chat processes: %v", err)
	}
	if result.Checked != 2 {
		t.Fatalf("expected checked=2, got %d", result.Checked)
	}
	if result.Removed != 2 {
		t.Fatalf("expected removed=2, got %d", result.Removed)
	}
	if result.Skipped != 0 {
		t.Fatalf("expected skipped=0, got %d", result.Skipped)
	}
	if len(killed) != 2 || killed[0] != 11 || killed[1] != 10 {
		t.Fatalf("unexpected killed pids: %+v", killed)
	}
}

func TestRunUnifiedGC(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	manager, err := session.NewManagerWithPaths(
		filepath.Join(root, "data", "sessions.json"),
		filepath.Join(root, "runtime", "sessions"),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	oldList := sessionListProcessCmdlines
	oldKill := sessionKillPID
	defer func() {
		sessionListProcessCmdlines = oldList
		sessionKillPID = oldKill
	}()
	sessionListProcessCmdlines = func() ([]hostProcessInfo, error) {
		return []hostProcessInfo{
			{PID: 10, PPID: 1, Args: []string{"/tmp/air", "chat", "--provider", "firecracker"}},
			{PID: 11, PPID: 10, Args: []string{"bun", "run", "/tmp/air-openclaude-chat-123.mjs"}},
		}, nil
	}
	sessionKillPID = func(pid int) error { return nil }

	result, err := runUnifiedGC(manager)
	if err != nil {
		t.Fatalf("run unified gc: %v", err)
	}
	if result.Session == nil || result.Chat == nil {
		t.Fatalf("expected both session and chat results, got %+v", result)
	}
	if result.Chat.Checked != 2 || result.Chat.Removed != 2 {
		t.Fatalf("unexpected chat gc result: %+v", result.Chat)
	}
}

func TestParseChatFlagsDoesNotDefaultWorkspace(t *testing.T) {
	t.Helper()

	opts, err := parseChatFlags([]string{"--provider", "firecracker"})
	if err != nil {
		t.Fatalf("parse chat flags: %v", err)
	}
	if opts.WorkspacePath != "" {
		t.Fatalf("expected empty default workspace, got %q", opts.WorkspacePath)
	}
	if opts.ListenAddress != "127.0.0.1:0" {
		t.Fatalf("unexpected default listen address: %q", opts.ListenAddress)
	}
}

func TestParseSessionExportWorkspaceFlags(t *testing.T) {
	t.Helper()

	opts, err := parseSessionExportWorkspaceFlags([]string{"sess_123", "/tmp/output", "--force"})
	if err != nil {
		t.Fatalf("parse export-workspace flags: %v", err)
	}
	if opts.SessionID != "sess_123" {
		t.Fatalf("unexpected session id: %q", opts.SessionID)
	}
	if opts.OutputPath != "/tmp/output" {
		t.Fatalf("unexpected output path: %q", opts.OutputPath)
	}
	if !opts.Force {
		t.Fatal("expected force=true")
	}

	if _, err := parseSessionExportWorkspaceFlags([]string{"sess_123"}); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestParseOpenClaudeStartFlags(t *testing.T) {
	t.Helper()

	opts, err := parseOpenClaudeStartFlags([]string{
		"--session", "sess_123",
		"--provider", "firecracker",
		"--network", "full",
		"--memory-mib", "2048",
		"--storage-mib", "4096",
		"--repo", "/tmp/openclaude",
		"--guest-repo", "/opt/openclaude",
		"--host", "0.0.0.0",
		"--port", "50055",
		"--command", "bun run scripts/start-grpc.ts",
	})
	if err != nil {
		t.Fatalf("parse openclaude start flags: %v", err)
	}
	if opts.SessionID != "sess_123" {
		t.Fatalf("unexpected session id: %q", opts.SessionID)
	}
	if opts.Provider != "firecracker" {
		t.Fatalf("unexpected provider: %q", opts.Provider)
	}
	if opts.Network != "full" {
		t.Fatalf("unexpected network: %q", opts.Network)
	}
	if opts.MemoryMiB != 2048 {
		t.Fatalf("unexpected memory: %d", opts.MemoryMiB)
	}
	if opts.StorageMiB != 4096 {
		t.Fatalf("unexpected storage: %d", opts.StorageMiB)
	}
	if opts.RepoPath != "/tmp/openclaude" {
		t.Fatalf("unexpected repo path: %q", opts.RepoPath)
	}
	if opts.GuestRepoPath != "/opt/openclaude" {
		t.Fatalf("unexpected guest repo path: %q", opts.GuestRepoPath)
	}
	if opts.Host != "0.0.0.0" {
		t.Fatalf("unexpected host: %q", opts.Host)
	}
	if opts.Port != 50055 {
		t.Fatalf("unexpected port: %d", opts.Port)
	}
	if opts.Command != "bun run scripts/start-grpc.ts" {
		t.Fatalf("unexpected command: %q", opts.Command)
	}
}

func TestParseOpenClaudeStartFlagsRejectsInvalidPort(t *testing.T) {
	t.Helper()

	if _, err := parseOpenClaudeStartFlags([]string{"--port", "0"}); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestParseOpenClaudeForwardFlags(t *testing.T) {
	t.Helper()

	opts, err := parseOpenClaudeForwardFlags([]string{"sess_123", "--listen", "127.0.0.1:6000"})
	if err != nil {
		t.Fatalf("parse openclaude forward flags: %v", err)
	}
	if opts.SessionID != "sess_123" {
		t.Fatalf("unexpected session id: %q", opts.SessionID)
	}
	if opts.ListenAddress != "127.0.0.1:6000" {
		t.Fatalf("unexpected listen address: %q", opts.ListenAddress)
	}
}

func TestParseOpenClaudeForwardFlagsRequiresSessionID(t *testing.T) {
	t.Helper()

	if _, err := parseOpenClaudeForwardFlags([]string{"--listen", "127.0.0.1:6000"}); err == nil {
		t.Fatal("expected missing session id error")
	}
}

func TestParseOpenClaudeChatFlags(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_OPENCLAUDE_REPO", "/tmp/openclaude")
	opts, err := parseOpenClaudeChatFlags([]string{"sess_123", "--listen", "127.0.0.1:6000"})
	if err != nil {
		t.Fatalf("parse openclaude chat flags: %v", err)
	}
	if opts.SessionID != "sess_123" {
		t.Fatalf("unexpected session id: %q", opts.SessionID)
	}
	if opts.ListenAddress != "127.0.0.1:6000" {
		t.Fatalf("unexpected listen address: %q", opts.ListenAddress)
	}
	if opts.CLIRepoPath != "/tmp/openclaude" {
		t.Fatalf("unexpected cli repo path: %q", opts.CLIRepoPath)
	}
}

func TestParseOpenClaudeChatFlagsDefaultsToEphemeralListen(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_OPENCLAUDE_REPO", "/tmp/openclaude")
	opts, err := parseOpenClaudeChatFlags([]string{"sess_123"})
	if err != nil {
		t.Fatalf("parse openclaude chat flags: %v", err)
	}
	if opts.ListenAddress != "127.0.0.1:0" {
		t.Fatalf("unexpected default listen address: %q", opts.ListenAddress)
	}
}

func TestParseOpenClaudeChatFlagsRequiresRepo(t *testing.T) {
	t.Helper()

	if _, err := parseOpenClaudeChatFlags([]string{"sess_123"}); err == nil {
		t.Fatal("expected missing cli repo error")
	}
}

func TestRenderOpenClaudeTranscriptEvent(t *testing.T) {
	t.Helper()

	var out bytes.Buffer
	renderOpenClaudeTranscriptEvent(&out, &openClaudeTranscriptEvent{
		Timestamp: "2026-04-29T00:00:00Z",
		Event:     "tool_start",
		ToolName:  "bash",
		ArgsJSON:  `{"command":"pwd"}`,
	})
	got := out.String()
	if !strings.Contains(got, "tool.start bash") {
		t.Fatalf("unexpected replay output: %q", got)
	}
}

func TestParseOpenClaudeRunFlags(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_OPENCLAUDE_REPO", "/tmp/openclaude")
	opts, err := parseOpenClaudeRunFlags([]string{
		"--provider", "firecracker",
		"--network", "full",
		"--memory-mib", "2048",
		"--storage-mib", "4096",
		"--workspace", "/tmp/repo",
		"--guest-repo", "/opt/openclaude",
		"--listen", "127.0.0.1:6000",
	})
	if err != nil {
		t.Fatalf("parse openclaude run flags: %v", err)
	}
	if opts.Provider != "firecracker" {
		t.Fatalf("unexpected provider: %q", opts.Provider)
	}
	if opts.Network != "full" {
		t.Fatalf("unexpected network: %q", opts.Network)
	}
	if opts.MemoryMiB != 2048 {
		t.Fatalf("unexpected memory: %d", opts.MemoryMiB)
	}
	if opts.StorageMiB != 4096 {
		t.Fatalf("unexpected storage: %d", opts.StorageMiB)
	}
	if opts.WorkspacePath != "/tmp/repo" {
		t.Fatalf("unexpected workspace path: %q", opts.WorkspacePath)
	}
	if opts.RepoPath != "/tmp/openclaude" {
		t.Fatalf("unexpected repo path: %q", opts.RepoPath)
	}
	if opts.GuestRepoPath != "/opt/openclaude" {
		t.Fatalf("unexpected guest repo path: %q", opts.GuestRepoPath)
	}
	if opts.ListenAddress != "127.0.0.1:6000" {
		t.Fatalf("unexpected listen address: %q", opts.ListenAddress)
	}
}

func TestParseOpenClaudeRunFlagsDefaultsToEphemeralListen(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_OPENCLAUDE_REPO", "/tmp/openclaude")
	opts, err := parseOpenClaudeRunFlags([]string{"--provider", "firecracker"})
	if err != nil {
		t.Fatalf("parse openclaude run flags: %v", err)
	}
	if opts.ListenAddress != "127.0.0.1:0" {
		t.Fatalf("unexpected default listen address: %q", opts.ListenAddress)
	}
}

func TestReserveLocalListenAddressAllocatesConcretePort(t *testing.T) {
	t.Helper()

	address, err := reserveLocalListenAddress("127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local listen address: %v", err)
	}
	if !strings.HasPrefix(address, "127.0.0.1:") {
		t.Fatalf("unexpected reserved address: %q", address)
	}
	if strings.HasSuffix(address, ":0") {
		t.Fatalf("expected concrete port, got %q", address)
	}
}

func TestEnsureOpenClaudeGuestBootArgsSetsInitForAlpineGuest(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_FIRECRACKER_BOOT_ARGS", "")
	ensureOpenClaudeGuestBootArgs("/tmp/assets/firecracker/openclaude-ubuntu-rootfs.ext4")
	if got := os.Getenv("AIR_FIRECRACKER_BOOT_ARGS"); got != "console=ttyS0 reboot=k panic=1 pci=off init=/sbin/init" {
		t.Fatalf("unexpected boot args: %q", got)
	}
}

func TestEnsureOpenClaudeGuestBootArgsDoesNotOverrideExplicitValue(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_FIRECRACKER_BOOT_ARGS", "console=ttyS0 custom=yes")
	ensureOpenClaudeGuestBootArgs("/tmp/assets/firecracker/openclaude-ubuntu-rootfs.ext4")
	if got := os.Getenv("AIR_FIRECRACKER_BOOT_ARGS"); got != "console=ttyS0 custom=yes" {
		t.Fatalf("unexpected boot args override: %q", got)
	}
}

func TestEnsureOpenClaudeGuestBootArgsIgnoresOtherRootfs(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_FIRECRACKER_BOOT_ARGS", "")
	ensureOpenClaudeGuestBootArgs("/tmp/assets/firecracker/hello-rootfs-air.ext4")
	if got := os.Getenv("AIR_FIRECRACKER_BOOT_ARGS"); got != "" {
		t.Fatalf("unexpected boot args for non-openclaude rootfs: %q", got)
	}
}

func TestEnsureOpenClaudeGuestBootArgsIgnoresLegacyAlpineRootfs(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_FIRECRACKER_BOOT_ARGS", "")
	ensureOpenClaudeGuestBootArgs("/tmp/assets/firecracker/openclaude-alpine-rootfs.ext4")
	if got := os.Getenv("AIR_FIRECRACKER_BOOT_ARGS"); got != "" {
		t.Fatalf("unexpected boot args for legacy alpine rootfs: %q", got)
	}
}

func TestApplyChatProfileEnvOpenAI(t *testing.T) {
	t.Helper()

	t.Setenv("CLAUDE_CODE_USE_OPENAI", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_BASE_URL", "https://api.deepseek.com/anthropic")
	t.Setenv("ANTHROPIC_MODEL", "deepseek-v4-pro")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "sk-anthropic")

	applyChatProfileEnv(&chatProfile{
		ProviderMode:  "openai",
		OpenAIBaseURL: "https://api.deepseek.com/v1",
		OpenAIModel:   "deepseek-chat",
		OpenAIAPIKey:  "sk-openai",
	})

	if got := os.Getenv("CLAUDE_CODE_USE_OPENAI"); got != "1" {
		t.Fatalf("expected CLAUDE_CODE_USE_OPENAI=1, got %q", got)
	}
	if got := os.Getenv("OPENAI_BASE_URL"); got != "https://api.deepseek.com/v1" {
		t.Fatalf("unexpected openai base url: %q", got)
	}
	if got := os.Getenv("ANTHROPIC_BASE_URL"); got != "" {
		t.Fatalf("expected anthropic base url cleared, got %q", got)
	}
}

func TestApplyChatProfileEnvAnthropic(t *testing.T) {
	t.Helper()

	t.Setenv("CLAUDE_CODE_USE_OPENAI", "1")
	t.Setenv("OPENAI_BASE_URL", "https://api.deepseek.com/v1")
	t.Setenv("OPENAI_MODEL", "deepseek-chat")
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")

	applyChatProfileEnv(&chatProfile{
		ProviderMode:       "anthropic",
		AnthropicBaseURL:   "https://api.deepseek.com/anthropic",
		AnthropicModel:     "deepseek-v4-pro[1m]",
		AnthropicAuthToken: "sk-anthropic",
	})

	if got := os.Getenv("CLAUDE_CODE_USE_OPENAI"); got != "" {
		t.Fatalf("expected CLAUDE_CODE_USE_OPENAI cleared, got %q", got)
	}
	if got := os.Getenv("ANTHROPIC_BASE_URL"); got != "https://api.deepseek.com/anthropic" {
		t.Fatalf("unexpected anthropic base url: %q", got)
	}
	if got := os.Getenv("ANTHROPIC_MODEL"); got != "deepseek-v4-pro[1m]" {
		t.Fatalf("unexpected anthropic model: %q", got)
	}
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "sk-anthropic" {
		t.Fatalf("unexpected anthropic api key: %q", got)
	}
	if got := os.Getenv("ANTHROPIC_AUTH_TOKEN"); got != "sk-anthropic" {
		t.Fatalf("unexpected anthropic auth token: %q", got)
	}
	if got := os.Getenv("OPENAI_BASE_URL"); got != "" {
		t.Fatalf("expected openai base url cleared, got %q", got)
	}
}

func TestOpenClaudeChatScriptDefaultsToAutoApproveAll(t *testing.T) {
	t.Helper()

	if !strings.Contains(openClaudeChatScript, "AIR_OPENCLAUDE_AUTO_APPROVE_TOOLS || '*'") {
		t.Fatalf("expected chat script to default auto-approve all tools")
	}
	if !strings.Contains(openClaudeChatScript, "autoApproveTools.has('*')") {
		t.Fatalf("expected chat script to support wildcard auto-approval")
	}
	if !strings.Contains(openClaudeChatScript, "auto: y") {
		t.Fatalf("expected chat script to print auto-approve marker")
	}
}

func TestRenderOpenClaudePTYCommandUsesNativeCLI(t *testing.T) {
	t.Helper()

	command := renderOpenClaudePTYCommand("/opt/open claude", map[string]string{
		"CLAUDE_CONFIG_DIR":  "/root/.openclaude",
		"ANTHROPIC_API_KEY":  "sk-anthropic",
		"ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic",
		"ANTHROPIC_MODEL":    "deepseek-v4-pro[1m]",
	})
	if !strings.Contains(command, "cd '/opt/open claude'") {
		t.Fatalf("expected shell-quoted repo path, got %q", command)
	}
	if !strings.Contains(command, "mkdir -p '/root/.openclaude'") {
		t.Fatalf("expected config dir creation, got %q", command)
	}
	if !strings.Contains(command, "/root/.openclaude/.openclaude.json") {
		t.Fatalf("expected config file path, got %q", command)
	}
	if !strings.Contains(command, "exec /usr/local/bin/bun ./dist/cli.mjs") {
		t.Fatalf("expected bun dist startup, got %q", command)
	}
	if !strings.Contains(command, "exec /usr/bin/node ./dist/cli.mjs") {
		t.Fatalf("expected node dist fallback, got %q", command)
	}
	if !strings.Contains(command, "exec /usr/local/bin/bun ./dist/cli.mjs") {
		t.Fatalf("expected absolute bun dist startup, got %q", command)
	}
	if !strings.Contains(command, "dist/cli.mjs missing in guest image") {
		t.Fatalf("expected missing dist error, got %q", command)
	}
	if strings.Contains(command, "src/entrypoints/cli.tsx") {
		t.Fatalf("expected PTY launcher to avoid source entrypoint fallback, got %q", command)
	}
}

func TestCollectOpenClaudePTYEnvAllowsOnlyRuntimeEnv(t *testing.T) {
	t.Helper()

	env := collectOpenClaudePTYEnv([]string{
		"OPENAI_API_KEY=sk-test",
		"PATH=/broken/host/path",
		"HOME=/home/bigrain",
		"SHELL=/bin/bash",
		"SECRET_TOKEN=hidden",
		"EMPTY=",
	})
	if env["OPENAI_API_KEY"] != "sk-test" {
		t.Fatalf("expected openai api key to be forwarded")
	}
	if env["PATH"] != "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" {
		t.Fatalf("expected guest path fallback, got %+v", env)
	}
	if env["HOME"] != "/root" {
		t.Fatalf("expected guest home fallback, got %+v", env)
	}
	if env["SHELL"] != "/bin/sh" {
		t.Fatalf("expected guest shell fallback, got %+v", env)
	}
	if env["CLAUDE_CONFIG_DIR"] != "/root/.openclaude" {
		t.Fatalf("expected guest config dir fallback, got %+v", env)
	}
	if _, ok := env["SECRET_TOKEN"]; ok {
		t.Fatalf("unexpected secret forwarded: %+v", env)
	}
	if _, ok := env["EMPTY"]; ok {
		t.Fatalf("unexpected empty env forwarded: %+v", env)
	}
}

func TestCollectOpenClaudePTYEnvFallsBackToSavedProfile(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := saveChatProfile(&chatProfile{
		ProviderMode:       "anthropic",
		AnthropicBaseURL:   "https://api.deepseek.com/anthropic",
		AnthropicModel:     "deepseek-v4-pro[1m]",
		AnthropicAuthToken: "sk-anthropic",
	}); err != nil {
		t.Fatalf("save chat profile: %v", err)
	}

	env := collectOpenClaudePTYEnv([]string{
		"PATH=/usr/local/bin:/usr/bin",
		"HOME=/home/bigrain",
	})
	if env["ANTHROPIC_BASE_URL"] != "https://api.deepseek.com/anthropic" {
		t.Fatalf("unexpected anthropic base url: %+v", env)
	}
	if env["ANTHROPIC_MODEL"] != "deepseek-v4-pro[1m]" {
		t.Fatalf("unexpected anthropic model: %+v", env)
	}
	if env["ANTHROPIC_API_KEY"] != "sk-anthropic" {
		t.Fatalf("unexpected anthropic api key: %+v", env)
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Fatalf("expected anthropic auth token to be normalized away: %+v", env)
	}
	if env["TERM"] != "xterm-256color" {
		t.Fatalf("expected default TERM, got %+v", env)
	}
	if env["HOME"] != "/root" {
		t.Fatalf("expected guest HOME, got %+v", env)
	}
	if env["CLAUDE_CONFIG_DIR"] != "/root/.openclaude" {
		t.Fatalf("expected guest config dir, got %+v", env)
	}
	if env["PATH"] != "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" {
		t.Fatalf("expected guest PATH, got %+v", env)
	}
}

func TestCollectOpenClaudePTYEnvRewritesLoopbackProxyToGuestProxy(t *testing.T) {
	t.Helper()

	env := collectOpenClaudePTYEnv([]string{
		"HTTP_PROXY=http://127.0.0.1:3067/",
		"HTTPS_PROXY=http://127.0.0.1:3067/",
		"ALL_PROXY=socks://127.0.0.1:3067/",
	})

	if env["HTTP_PROXY"] != "http://127.0.0.1:18080" {
		t.Fatalf("expected guest HTTP proxy, got %+v", env)
	}
	if env["HTTPS_PROXY"] != "http://127.0.0.1:18080" {
		t.Fatalf("expected guest HTTPS proxy, got %+v", env)
	}
	if env["ALL_PROXY"] != "http://127.0.0.1:18080" {
		t.Fatalf("expected guest ALL proxy, got %+v", env)
	}
}

func TestCollectOpenClaudePTYEnvNormalizesAnthropicAuthToken(t *testing.T) {
	t.Helper()

	env := collectOpenClaudePTYEnv([]string{
		"ANTHROPIC_AUTH_TOKEN=sk-anthropic",
		"ANTHROPIC_BASE_URL=https://api.deepseek.com/anthropic",
		"ANTHROPIC_MODEL=deepseek-v4-pro[1m]",
	})

	if env["ANTHROPIC_API_KEY"] != "sk-anthropic" {
		t.Fatalf("expected anthropic api key, got %+v", env)
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Fatalf("expected anthropic auth token to be removed, got %+v", env)
	}
	if env["CLAUDE_CODE_USE_OPENAI"] != "" {
		t.Fatalf("expected openai flag removed for anthropic env, got %+v", env)
	}
}

func TestBuildOpenClaudePTYGlobalConfigApprovesAnthropicAPIKey(t *testing.T) {
	t.Helper()

	apiKey := "sk-1234567890ABCDEFGHIJKLMN"
	body := buildOpenClaudePTYGlobalConfig(map[string]string{
		"ANTHROPIC_API_KEY":  apiKey,
		"ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic",
		"ANTHROPIC_MODEL":    "deepseek-v4-pro[1m]",
		"CLAUDE_CONFIG_DIR":  "/root/.openclaude",
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
	if got := approved[0]; got != normalizeOpenClaudePTYAPIKeyForConfig(apiKey) {
		t.Fatalf("unexpected approved key suffix: %#v", got)
	}

	env, ok := decoded["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env object, got %#v", decoded["env"])
	}
	if got := env["ANTHROPIC_MODEL"]; got != "deepseek-v4-pro[1m]" {
		t.Fatalf("expected ANTHROPIC_MODEL in env, got %#v", got)
	}
}
