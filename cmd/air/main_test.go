package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		"--timeout", "45s",
		"--memory-mib", "512",
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
	if opts.MemoryMiB != 512 {
		t.Fatalf("unexpected memory: %d", opts.MemoryMiB)
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
	if opts.WorkspacePath != "/tmp/repo" {
		t.Fatalf("unexpected workspace: %q", opts.WorkspacePath)
	}
	if opts.ListenAddress != "127.0.0.1:50060" {
		t.Fatalf("unexpected listen address: %q", opts.ListenAddress)
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
