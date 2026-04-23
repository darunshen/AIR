package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/darunshen/AIR/internal/vm"
)

const (
	defaultOpenClaudeCommand = "bun run scripts/start-grpc.ts"
	defaultOpenClaudeHost    = "127.0.0.1"
	defaultOpenClaudePort    = 50051
)

var ErrOpenClaudeNotConfigured = errors.New("openclaude is not configured for this session")

type OpenClaudeStartOptions struct {
	SessionID     string
	Provider      string
	RepoPath      string
	GuestRepoPath string
	WorkspacePath string
	Command       string
	Host          string
	Port          int
}

type openClaudeMetadata struct {
	SessionID   string     `json:"session_id"`
	Provider    string     `json:"provider"`
	RepoPath    string     `json:"repo_path"`
	Command     string     `json:"command"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	StateDir    string     `json:"state_dir"`
	PIDPath     string     `json:"pid_path"`
	LogPath     string     `json:"log_path"`
	LastPID     int        `json:"last_pid,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	StoppedAt   *time.Time `json:"stopped_at,omitempty"`
	LastChecked *time.Time `json:"last_checked_at,omitempty"`
}

type OpenClaudeStatus struct {
	SessionID      string     `json:"session_id"`
	Provider       string     `json:"provider"`
	SessionStatus  string     `json:"session_status"`
	RepoPath       string     `json:"repo_path"`
	Command        string     `json:"command"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	StateDir       string     `json:"state_dir"`
	PIDPath        string     `json:"pid_path"`
	LogPath        string     `json:"log_path"`
	MetadataPath   string     `json:"metadata_path"`
	PID            int        `json:"pid,omitempty"`
	Running        bool       `json:"running"`
	CreatedSession bool       `json:"created_session,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	StoppedAt      *time.Time `json:"stopped_at,omitempty"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
}

type OpenClaudeForwardOptions struct {
	ListenAddress string
	DialTimeout   time.Duration
}

func (m *Manager) StartOpenClaude(opts OpenClaudeStartOptions) (*OpenClaudeStatus, error) {
	opts = normalizeOpenClaudeStartOptions(opts)

	sessionID := opts.SessionID
	createdSession := false
	if sessionID == "" {
		s, err := m.CreateWithOptions(CreateOptions{
			Provider:      opts.Provider,
			WorkspacePath: opts.WorkspacePath,
		})
		if err != nil {
			return nil, err
		}
		sessionID = s.ID
		createdSession = true
	}

	inspect, err := m.Inspect(sessionID)
	if err != nil {
		return nil, err
	}
	if inspect.Session.Status != "running" {
		return nil, errors.New("session is not running")
	}

	repoPath := resolveOpenClaudeRepoPath(inspect.Session.Provider, opts)
	if repoPath == "" {
		return nil, errors.New("openclaude repo path is required; use --repo, --guest-repo, or AIR_OPENCLAUDE_REPO")
	}

	metadataPath := openClaudeMetadataPath(inspect.Runtime.RootPath)
	if existing, err := readOpenClaudeMetadata(metadataPath); err == nil {
		status, statusErr := m.openClaudeStatusWithMetadata(inspect, existing)
		if statusErr != nil {
			return nil, statusErr
		}
		if status.Running {
			return nil, fmt.Errorf("openclaude is already running in session %s", sessionID)
		}
	} else if !errors.Is(err, ErrOpenClaudeNotConfigured) {
		return nil, err
	}

	if !path.IsAbs(repoPath) {
		repoPath = path.Clean(repoPath)
	}

	meta := &openClaudeMetadata{
		SessionID: sessionID,
		Provider:  inspect.Session.Provider,
		RepoPath:  repoPath,
		Command:   opts.Command,
		Host:      opts.Host,
		Port:      opts.Port,
		StateDir:  openClaudeStateDirForProvider(inspect.Session.Provider, repoPath, sessionID),
		StartedAt: time.Now().UTC(),
	}
	meta.PIDPath = path.Join(meta.StateDir, "server.pid")
	meta.LogPath = path.Join(meta.StateDir, "server.log")

	result, err := m.Exec(sessionID, renderOpenClaudeStartCommand(meta))
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("openclaude start command exited with code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}

	pid, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if err != nil {
		return nil, fmt.Errorf("parse openclaude pid: %w", err)
	}
	meta.LastPID = pid
	now := time.Now().UTC()
	meta.LastChecked = &now
	meta.StoppedAt = nil
	if err := writeOpenClaudeMetadata(metadataPath, meta); err != nil {
		return nil, err
	}

	status := openClaudeStatusFromMetadata(inspect, meta)
	status.PID = pid
	status.Running = true
	status.CreatedSession = createdSession
	status.LastCheckedAt = meta.LastChecked
	return status, nil
}

func (m *Manager) OpenClaudeStatus(sessionID string) (*OpenClaudeStatus, error) {
	inspect, err := m.Inspect(sessionID)
	if err != nil {
		return nil, err
	}

	meta, err := readOpenClaudeMetadata(openClaudeMetadataPath(inspect.Runtime.RootPath))
	if err != nil {
		return nil, err
	}

	return m.openClaudeStatusWithMetadata(inspect, meta)
}

func (m *Manager) StopOpenClaude(sessionID string) (*OpenClaudeStatus, error) {
	inspect, err := m.Inspect(sessionID)
	if err != nil {
		return nil, err
	}

	metadataPath := openClaudeMetadataPath(inspect.Runtime.RootPath)
	meta, err := readOpenClaudeMetadata(metadataPath)
	if err != nil {
		return nil, err
	}

	status := openClaudeStatusFromMetadata(inspect, meta)
	if inspect.Session.Status == "running" {
		result, execErr := m.Exec(sessionID, renderOpenClaudeStopCommand(meta.PIDPath))
		if execErr != nil {
			return nil, execErr
		}
		if result.ExitCode != 0 {
			return nil, fmt.Errorf("openclaude stop command exited with code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
		}
	}

	now := time.Now().UTC()
	meta.StoppedAt = &now
	meta.LastChecked = &now
	meta.LastPID = 0
	if err := writeOpenClaudeMetadata(metadataPath, meta); err != nil {
		return nil, err
	}

	status.Running = false
	status.PID = 0
	status.StoppedAt = meta.StoppedAt
	status.LastCheckedAt = meta.LastChecked
	return status, nil
}

func (m *Manager) DialOpenClaude(sessionID string, timeout time.Duration) (net.Conn, *OpenClaudeStatus, error) {
	status, err := m.OpenClaudeStatus(sessionID)
	if err != nil {
		return nil, nil, err
	}
	if !status.Running {
		return nil, nil, errors.New("openclaude is not running")
	}

	sessionModel, err := m.store.Get(sessionID)
	if err != nil {
		return nil, nil, err
	}
	if err := m.ensureProvider(sessionModel); err != nil {
		return nil, nil, err
	}

	runtime, err := m.runtimeForSession(sessionModel)
	if err != nil {
		return nil, nil, err
	}
	dialer, ok := runtime.(vm.TCPDialer)
	if !ok {
		return nil, nil, errors.New("session provider does not support tcp dialing")
	}

	conn, err := dialer.DialTCP(sessionID, net.JoinHostPort(status.Host, strconv.Itoa(status.Port)), timeout)
	if err != nil {
		return nil, nil, err
	}
	return conn, status, nil
}

func (m *Manager) ForwardOpenClaude(ctx context.Context, sessionID string, opts OpenClaudeForwardOptions) error {
	listenAddress := opts.ListenAddress
	if listenAddress == "" {
		listenAddress = net.JoinHostPort(defaultOpenClaudeHost, strconv.Itoa(defaultOpenClaudePort))
	}
	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return err
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go func() {
			defer clientConn.Close()
			targetConn, _, err := m.DialOpenClaude(sessionID, dialTimeout)
			if err != nil {
				return
			}
			defer targetConn.Close()
			bridgeConns(clientConn, targetConn)
		}()
	}
}

func (m *Manager) openClaudeStatusWithMetadata(inspect *InspectResult, meta *openClaudeMetadata) (*OpenClaudeStatus, error) {
	status := openClaudeStatusFromMetadata(inspect, meta)
	now := time.Now().UTC()

	if inspect.Session.Status != "running" {
		meta.LastChecked = &now
		if err := writeOpenClaudeMetadata(status.MetadataPath, meta); err != nil {
			return nil, err
		}
		status.LastCheckedAt = meta.LastChecked
		return status, nil
	}

	result, err := m.Exec(inspect.Session.ID, renderOpenClaudeStatusCommand(meta.PIDPath))
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("openclaude status command exited with code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}

	line := strings.TrimSpace(result.Stdout)
	switch {
	case strings.HasPrefix(line, "running "):
		pid, convErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "running ")))
		if convErr != nil {
			return nil, fmt.Errorf("parse openclaude status pid: %w", convErr)
		}
		status.Running = true
		status.PID = pid
		meta.LastPID = pid
		meta.StoppedAt = nil
	case line == "stopped":
		status.Running = false
		status.PID = 0
		meta.LastPID = 0
		if meta.StoppedAt == nil {
			meta.StoppedAt = &now
		}
	default:
		return nil, fmt.Errorf("unexpected openclaude status output: %q", line)
	}

	meta.LastChecked = &now
	if err := writeOpenClaudeMetadata(status.MetadataPath, meta); err != nil {
		return nil, err
	}
	status.LastCheckedAt = meta.LastChecked
	status.StoppedAt = meta.StoppedAt
	return status, nil
}

func normalizeOpenClaudeStartOptions(opts OpenClaudeStartOptions) OpenClaudeStartOptions {
	if opts.Command == "" {
		opts.Command = defaultOpenClaudeCommand
	}
	if opts.Host == "" {
		opts.Host = defaultOpenClaudeHost
	}
	if opts.Port <= 0 {
		opts.Port = defaultOpenClaudePort
	}
	if opts.RepoPath == "" {
		opts.RepoPath = os.Getenv("AIR_OPENCLAUDE_REPO")
	}
	if opts.GuestRepoPath == "" {
		opts.GuestRepoPath = os.Getenv("AIR_OPENCLAUDE_GUEST_REPO")
	}
	return opts
}

func resolveOpenClaudeRepoPath(provider string, opts OpenClaudeStartOptions) string {
	if provider == "firecracker" {
		if opts.GuestRepoPath != "" {
			return opts.GuestRepoPath
		}
		if opts.RepoPath == "" {
			return "/opt/openclaude"
		}
	}
	return opts.RepoPath
}

func openClaudeStateDirForProvider(provider, repoPath, sessionID string) string {
	if provider == "firecracker" {
		return path.Join("/run/air/openclaude", sessionID)
	}
	return openClaudeStateDir(repoPath, sessionID)
}

func openClaudeStatusFromMetadata(inspect *InspectResult, meta *openClaudeMetadata) *OpenClaudeStatus {
	status := &OpenClaudeStatus{
		SessionID:     inspect.Session.ID,
		Provider:      inspect.Session.Provider,
		SessionStatus: inspect.Session.Status,
		RepoPath:      meta.RepoPath,
		Command:       meta.Command,
		Host:          meta.Host,
		Port:          meta.Port,
		StateDir:      meta.StateDir,
		PIDPath:       meta.PIDPath,
		LogPath:       meta.LogPath,
		MetadataPath:  openClaudeMetadataPath(inspect.Runtime.RootPath),
		StartedAt:     meta.StartedAt,
		StoppedAt:     meta.StoppedAt,
		LastCheckedAt: meta.LastChecked,
	}
	if meta.LastPID > 0 {
		status.PID = meta.LastPID
	}
	return status
}

func openClaudeMetadataPath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "openclaude.json")
}

func openClaudeStateDir(repoPath, sessionID string) string {
	return path.Join(repoPath, ".air", "openclaude", sessionID)
}

func readOpenClaudeMetadata(metadataPath string) (*openClaudeMetadata, error) {
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrOpenClaudeNotConfigured
		}
		return nil, err
	}

	var meta openClaudeMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	if meta.SessionID == "" {
		return nil, errors.New("openclaude metadata is invalid")
	}
	return &meta, nil
}

func writeOpenClaudeMetadata(metadataPath string, meta *openClaudeMetadata) error {
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(metadataPath, body, 0o644)
}

func renderOpenClaudeStartCommand(meta *openClaudeMetadata) string {
	return strings.Join([]string{
		"mkdir -p " + shellQuote(meta.StateDir),
		"cd " + shellQuote(meta.RepoPath),
		"nohup env GRPC_HOST=" + shellQuote(meta.Host) +
			" GRPC_PORT=" + shellQuote(strconv.Itoa(meta.Port)) +
			" AIR_OPENCLAUDE_SESSION_ID=" + shellQuote(meta.SessionID) +
			" sh -c " + shellQuote(meta.Command) +
			" >> " + shellQuote(meta.LogPath) + " 2>&1 < /dev/null & pid=$!",
		"mkdir -p " + shellQuote(meta.StateDir),
		"printf '%s\\n' \"$pid\" > " + shellQuote(meta.PIDPath),
		"printf '%s\\n' \"$pid\"",
	}, " && ")
}

func renderOpenClaudeStatusCommand(pidPath string) string {
	return "if [ -s " + shellQuote(pidPath) + " ]; then " +
		"pid=$(cat " + shellQuote(pidPath) + "); " +
		"if [ -n \"$pid\" ] && kill -0 \"$pid\" 2>/dev/null; then printf 'running %s\\n' \"$pid\"; exit 0; fi; " +
		"fi; printf 'stopped\\n'"
}

func renderOpenClaudeStopCommand(pidPath string) string {
	return "if [ -s " + shellQuote(pidPath) + " ]; then " +
		"pid=$(cat " + shellQuote(pidPath) + "); " +
		"if [ -n \"$pid\" ] && kill -0 \"$pid\" 2>/dev/null; then " +
		"kill \"$pid\" 2>/dev/null || true; " +
		"for _ in 1 2 3 4 5 6 7 8 9 10; do " +
		"if kill -0 \"$pid\" 2>/dev/null; then sleep 1; else break; fi; " +
		"done; " +
		"if kill -0 \"$pid\" 2>/dev/null; then kill -9 \"$pid\" 2>/dev/null || true; fi; " +
		"fi; " +
		"fi; rm -f " + shellQuote(pidPath)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func bridgeConns(left, right net.Conn) {
	copyErrCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(left, right)
		copyErrCh <- err
	}()
	go func() {
		_, err := io.Copy(right, left)
		copyErrCh <- err
	}()
	<-copyErrCh
	_ = left.Close()
	_ = right.Close()
	<-copyErrCh
}
