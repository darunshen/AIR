package session

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/darunshen/AIR/internal/model"
	"github.com/darunshen/AIR/internal/store"
	"github.com/darunshen/AIR/internal/vm"
)

type ExecResult struct {
	RequestID string
	Stdout    string
	Stderr    string
	ExitCode  int
	TimedOut  bool
	Duration  time.Duration
}

type RunOptions struct {
	Provider      string
	Network       string
	Timeout       time.Duration
	MemoryMiB     int
	VCPUCount     int
	StorageMiB    int
	WorkspacePath string
}

type RunResult struct {
	SessionID    string       `json:"session_id,omitempty"`
	Provider     string       `json:"provider"`
	RequestID    string       `json:"request_id,omitempty"`
	Stdout       string       `json:"stdout"`
	Stderr       string       `json:"stderr"`
	ExitCode     int          `json:"exit_code"`
	DurationMS   int64        `json:"duration_ms"`
	Timeout      bool         `json:"timeout"`
	ErrorType    RunErrorType `json:"error_type,omitempty"`
	ErrorMessage string       `json:"error_message,omitempty"`
}

type CreateOptions struct {
	Provider      string
	Network       string
	MemoryMiB     int
	VCPUCount     int
	StorageMiB    int
	WorkspacePath string
}

type ExportWorkspaceResult struct {
	SessionID  string `json:"session_id"`
	Provider   string `json:"provider"`
	OutputPath string `json:"output_path"`
}

type GCOptions struct {
	DryRun bool
	Force  bool
}

type GCItem struct {
	SessionID string `json:"session_id"`
	Provider  string `json:"provider"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Removed   bool   `json:"removed"`
}

type GCResult struct {
	Checked int      `json:"checked"`
	Removed int      `json:"removed"`
	Skipped int      `json:"skipped"`
	Items   []GCItem `json:"items"`
}

type Manager struct {
	store         *store.Store
	runtimeRoot   string
	runtimeConfig vm.Config
	provider      string
	vm            vm.Runtime
}

type processCmdline struct {
	PID  int
	Args []string
}

var listProcessCmdlines = defaultListProcessCmdlines
var killPID = defaultKillPID

func NewManager() (*Manager, error) {
	return NewManagerWithPaths("runtime/sessions/store.json", "runtime/sessions")
}

func NewManagerWithPaths(storePath, runtimeRoot string) (*Manager, error) {
	s, err := store.New(storePath)
	if err != nil {
		return nil, err
	}

	cfg := vm.ResolveConfig(runtimeRoot)
	r, err := vm.NewWithConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Manager{
		store:         s,
		runtimeRoot:   runtimeRoot,
		runtimeConfig: cfg,
		provider:      cfg.Provider,
		vm:            r,
	}, nil
}

func (m *Manager) Create() (*model.Session, error) {
	return m.CreateWithProvider("")
}

func (m *Manager) CreateWithProvider(provider string) (*model.Session, error) {
	return m.CreateWithOptions(CreateOptions{Provider: provider})
}

func (m *Manager) CreateWithOptions(opts CreateOptions) (*model.Session, error) {
	runtime, resolvedProvider, err := m.runtimeForCreateOptions(opts)
	if err != nil {
		return nil, err
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}

	vmid, err := runtime.StartWithOptions(id, vm.StartOptions{
		WorkspacePath: opts.WorkspacePath,
		Network:       opts.Network,
		StorageMiB:    opts.StorageMiB,
	})
	if err != nil {
		return nil, fmt.Errorf("start %s runtime session %s: %w", resolvedProvider, id, err)
	}

	now := time.Now().UTC()
	s := &model.Session{
		ID:            id,
		VMID:          vmid,
		Provider:      resolvedProvider,
		Network:       normalizeSessionNetwork(opts.Network),
		Status:        "running",
		WorkspacePath: opts.WorkspacePath,
		CreatedAt:     now,
		LastUsedAt:    now,
	}

	if err := m.store.Save(s); err != nil {
		return nil, fmt.Errorf("save session %s: %w", id, err)
	}

	return s, nil
}

func (m *Manager) Exec(sessionID, command string) (*ExecResult, error) {
	return m.ExecWithTimeout(sessionID, command, 30*time.Second)
}

func (m *Manager) ExecStreaming(sessionID, command string, timeout time.Duration, onChunk func(vm.ExecChunk)) (*ExecResult, error) {
	s, err := m.store.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if err := m.ensureProvider(s); err != nil {
		return nil, err
	}
	runtime, err := m.runtimeForSession(s)
	if err != nil {
		return nil, err
	}
	info, err := runtime.Inspect(sessionID)
	if err != nil {
		return nil, err
	}
	if err := m.syncSessionState(s, info); err != nil {
		return nil, err
	}
	if s.Status != "running" {
		return nil, errors.New("session is not running")
	}
	streamingRuntime, ok := runtime.(vm.StreamingRuntime)
	if !ok {
		return m.ExecWithTimeout(sessionID, command, timeout)
	}
	result, err := streamingRuntime.ExecStreaming(sessionID, command, timeout, onChunk)
	if err != nil {
		return nil, err
	}
	s.LastUsedAt = time.Now().UTC()
	if err := m.store.Save(s); err != nil {
		return nil, err
	}
	return &ExecResult{
		RequestID: result.RequestID,
		Stdout:    result.Stdout,
		Stderr:    result.Stderr,
		ExitCode:  result.ExitCode,
		TimedOut:  result.TimedOut,
		Duration:  result.Duration,
	}, nil
}

func (m *Manager) AttachPTY(sessionID string, opts vm.PTYOptions) error {
	s, err := m.store.Get(sessionID)
	if err != nil {
		return err
	}
	if err := m.ensureProvider(s); err != nil {
		return err
	}
	runtime, err := m.runtimeForSession(s)
	if err != nil {
		return err
	}
	info, err := runtime.Inspect(sessionID)
	if err != nil {
		return err
	}
	if err := m.syncSessionState(s, info); err != nil {
		return err
	}
	if s.Status != "running" {
		return errors.New("session is not running")
	}
	ptyRuntime, ok := runtime.(vm.PTYRuntime)
	if !ok {
		return fmt.Errorf("provider %s does not support pty attach", s.Provider)
	}
	if err := ptyRuntime.AttachPTY(sessionID, opts); err != nil {
		return err
	}
	s.LastUsedAt = time.Now().UTC()
	return m.store.Save(s)
}

func (m *Manager) ExecWithTimeout(sessionID, command string, timeout time.Duration) (*ExecResult, error) {
	s, err := m.store.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if err := m.ensureProvider(s); err != nil {
		return nil, err
	}

	runtime, err := m.runtimeForSession(s)
	if err != nil {
		return nil, err
	}

	info, err := runtime.Inspect(sessionID)
	if err != nil {
		return nil, err
	}
	if err := m.syncSessionState(s, info); err != nil {
		return nil, err
	}
	if s.Status != "running" {
		return nil, errors.New("session is not running")
	}

	result, err := runtime.Exec(sessionID, command, timeout)
	if err != nil {
		return nil, err
	}

	s.LastUsedAt = time.Now().UTC()
	if err := m.store.Save(s); err != nil {
		return nil, err
	}

	return &ExecResult{
		RequestID: result.RequestID,
		Stdout:    result.Stdout,
		Stderr:    result.Stderr,
		ExitCode:  result.ExitCode,
		TimedOut:  result.TimedOut,
		Duration:  result.Duration,
	}, nil
}

func (m *Manager) Run(command string, opts RunOptions) (*RunResult, error) {
	result := &RunResult{
		Provider: m.resolveProvider(opts.Provider),
		ExitCode: -1,
	}

	if err := validateRunOptions(opts); err != nil {
		result.ErrorType = RunErrorTypeInvalidArgument
		result.ErrorMessage = err.Error()
		return result, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	s, err := m.CreateWithOptions(CreateOptions{
		Provider:      opts.Provider,
		Network:       opts.Network,
		MemoryMiB:     opts.MemoryMiB,
		VCPUCount:     opts.VCPUCount,
		StorageMiB:    opts.StorageMiB,
		WorkspacePath: opts.WorkspacePath,
	})
	if err != nil {
		result.ErrorType = classifyRunErrorType("create", err)
		result.ErrorMessage = err.Error()
		return result, err
	}

	result.SessionID = s.ID
	result.Provider = s.Provider

	execResult, execErr := m.ExecWithTimeout(s.ID, command, timeout)
	if execErr == nil {
		result.RequestID = execResult.RequestID
		result.Stdout = execResult.Stdout
		result.Stderr = execResult.Stderr
		result.ExitCode = execResult.ExitCode
		result.DurationMS = execResult.Duration.Milliseconds()
		result.Timeout = execResult.TimedOut
		switch {
		case execResult.TimedOut:
			result.ErrorType = RunErrorTypeTimeout
			result.ErrorMessage = "command timed out"
		case execResult.ExitCode != 0:
			result.ErrorType = RunErrorTypeExec
			result.ErrorMessage = fmt.Sprintf("command exited with code %d", execResult.ExitCode)
		}
	} else {
		result.ErrorType = classifyRunErrorType("exec", execErr)
		result.ErrorMessage = execErr.Error()
	}

	deleteErr := m.Delete(s.ID)
	if deleteErr != nil {
		if result.ErrorType == "" {
			result.ErrorType = classifyRunErrorType("delete", deleteErr)
			result.ErrorMessage = deleteErr.Error()
		} else {
			result.ErrorMessage = strings.TrimSpace(result.ErrorMessage + "; cleanup failed: " + deleteErr.Error())
		}
	}

	switch {
	case execErr != nil && deleteErr != nil:
		return result, errors.Join(execErr, deleteErr)
	case execErr != nil:
		return result, execErr
	case deleteErr != nil:
		return result, deleteErr
	default:
		return result, nil
	}
}

func (m *Manager) Delete(sessionID string) error {
	s, err := m.store.Get(sessionID)
	if err != nil {
		return err
	}
	if err := m.ensureProvider(s); err != nil {
		return err
	}

	runtime, err := m.runtimeForSession(s)
	if err != nil {
		return err
	}

	if _, err := m.StopOpenClaude(sessionID); err != nil &&
		!errors.Is(err, ErrOpenClaudeNotConfigured) &&
		!errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := runtime.Stop(s.VMID); err != nil {
		return err
	}

	return m.store.Delete(sessionID)
}

func (m *Manager) List() ([]*model.Session, error) {
	items, err := m.store.List()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := m.ensureProvider(item); err != nil {
			return nil, err
		}
		runtime, err := m.runtimeForSession(item)
		if err != nil {
			return nil, err
		}
		info, err := runtime.Inspect(item.ID)
		if err != nil {
			return nil, err
		}
		if err := m.syncSessionState(item, info); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (m *Manager) GC(opts GCOptions) (*GCResult, error) {
	items, err := m.store.List()
	if err != nil {
		return nil, err
	}

	result := &GCResult{
		Items: make([]GCItem, 0, len(items)),
	}
	known := make(map[string]struct{}, len(items))

	for _, item := range items {
		known[item.ID] = struct{}{}
		if err := m.ensureProvider(item); err != nil {
			return nil, err
		}
		runtime, err := m.runtimeForSession(item)
		if err != nil {
			return nil, err
		}
		info, err := runtime.Inspect(item.ID)
		if err != nil {
			return nil, err
		}
		if err := m.syncSessionState(item, info); err != nil {
			return nil, err
		}

		entry := GCItem{
			SessionID: item.ID,
			Provider:  item.Provider,
			Status:    item.Status,
		}

		if item.Status == "running" && !opts.Force {
			entry.Reason = "session is still running"
			result.Skipped++
			result.Items = append(result.Items, entry)
			continue
		}

		entry.Removed = true
		if item.Status == "running" && opts.Force {
			entry.Reason = "force cleanup requested"
			if !opts.DryRun {
				if err := m.deleteSessionRuntime(runtime, item); err != nil {
					return nil, err
				}
			}
		} else if info == nil || !info.Exists {
			entry.Reason = "runtime artifacts are already missing"
			if !opts.DryRun {
				if err := m.store.Delete(item.ID); err != nil {
					return nil, err
				}
			}
		} else {
			entry.Reason = "session is stopped"
			if !opts.DryRun {
				if err := m.cleanupStoppedSession(runtime, item, info); err != nil {
					return nil, err
				}
			}
		}
		result.Removed++
		result.Items = append(result.Items, entry)
	}

	orphanItems, err := m.gcOrphanRuntimeSessions(known, opts)
	if err != nil {
		return nil, err
	}
	result.Items = append(result.Items, orphanItems...)

	processItems, err := m.gcOrphanFirecrackerProcesses(known, opts)
	if err != nil {
		return nil, err
	}
	result.Items = append(result.Items, processItems...)
	result.Checked = len(result.Items)

	for _, item := range append(orphanItems, processItems...) {
		if item.Removed {
			result.Removed++
		} else {
			result.Skipped++
		}
	}

	return result, nil
}

type InspectResult struct {
	Session *model.Session  `json:"session"`
	Runtime *vm.InspectInfo `json:"runtime"`
}

func (m *Manager) Inspect(sessionID string) (*InspectResult, error) {
	s, err := m.store.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if err := m.ensureProvider(s); err != nil {
		return nil, err
	}

	runtime, err := m.runtimeForSession(s)
	if err != nil {
		return nil, err
	}

	info, err := runtime.Inspect(sessionID)
	if err != nil {
		return nil, err
	}
	if err := m.syncSessionState(s, info); err != nil {
		return nil, err
	}

	return &InspectResult{
		Session: s,
		Runtime: info,
	}, nil
}

func (m *Manager) ConsolePath(sessionID string) (string, error) {
	inspect, err := m.Inspect(sessionID)
	if err != nil {
		return "", err
	}
	if inspect.Runtime.ConsolePath == "" {
		return "", errors.New("session provider does not expose a console log")
	}
	return inspect.Runtime.ConsolePath, nil
}

func (m *Manager) EventsPath(sessionID string) (string, error) {
	inspect, err := m.Inspect(sessionID)
	if err != nil {
		return "", err
	}
	if inspect.Runtime.EventsPath == "" {
		return "", errors.New("session provider does not expose an events log")
	}
	return inspect.Runtime.EventsPath, nil
}

func (m *Manager) ExportWorkspace(sessionID, outputPath string, force bool) (*ExportWorkspaceResult, error) {
	if outputPath == "" {
		return nil, errors.New("output path must not be empty")
	}

	inspect, err := m.Inspect(sessionID)
	if err != nil {
		return nil, err
	}
	if inspect.Session.Status != "running" {
		return nil, errors.New("session is not running")
	}
	if inspect.Session.Provider == "firecracker" && inspect.Runtime.WorkspaceImagePath == "" {
		return nil, errors.New("firecracker session does not have a workspace attached")
	}

	outputPath, err = filepath.Abs(outputPath)
	if err != nil {
		return nil, err
	}
	if err := prepareExportOutputPath(outputPath, force); err != nil {
		return nil, err
	}

	result, err := m.Exec(sessionID, "tar -czf - . | base64 | tr -d '\\n'")
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(result.Stderr)
		if stderr == "" {
			stderr = fmt.Sprintf("export command exited with code %d", result.ExitCode)
		}
		return nil, errors.New(stderr)
	}

	archiveBody, err := base64.StdEncoding.DecodeString(strings.TrimSpace(result.Stdout))
	if err != nil {
		return nil, fmt.Errorf("decode workspace archive: %w", err)
	}
	if err := extractTarGz(bytes.NewReader(archiveBody), outputPath); err != nil {
		return nil, fmt.Errorf("extract workspace archive: %w", err)
	}

	return &ExportWorkspaceResult{
		SessionID:  sessionID,
		Provider:   inspect.Session.Provider,
		OutputPath: outputPath,
	}, nil
}

func (m *Manager) runtimeForSession(s *model.Session) (vm.Runtime, error) {
	runtime, _, err := m.runtimeForProvider(s.Provider)
	return runtime, err
}

func (m *Manager) ensureProvider(s *model.Session) error {
	if s.Provider != "" {
		return nil
	}

	switch {
	case dirExists(filepath.Join(m.runtimeRoot, "firecracker", s.ID)):
		s.Provider = "firecracker"
	case dirExists(filepath.Join(m.runtimeRoot, "local", s.ID)):
		s.Provider = "local"
	default:
		s.Provider = m.provider
	}

	return m.store.Save(s)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (m *Manager) resolveProvider(provider string) string {
	if provider != "" {
		return provider
	}
	return m.provider
}

func classifyRunErrorType(stage string, err error) RunErrorType {
	switch stage {
	case "create":
		return RunErrorTypeStartup
	case "exec":
		if errors.Is(err, vm.ErrGuestAgentNotReady) {
			return RunErrorTypeTransport
		}
		return RunErrorTypeExec
	case "delete":
		return RunErrorTypeCleanup
	default:
		return RunErrorTypeExec
	}
}

func (m *Manager) cleanupStoppedSession(runtime vm.Runtime, s *model.Session, info *vm.InspectInfo) error {
	if info != nil && info.Exists {
		if err := m.deleteSessionRuntime(runtime, s); err != nil {
			if info.RootPath == "" {
				return err
			}
			return err
		}
	}
	return m.store.Delete(s.ID)
}

func (m *Manager) deleteSessionRuntime(runtime vm.Runtime, s *model.Session) error {
	if _, err := m.StopOpenClaude(s.ID); err != nil &&
		!errors.Is(err, ErrOpenClaudeNotConfigured) &&
		!errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := runtime.Stop(s.VMID); err != nil {
		return err
	}
	return m.store.Delete(s.ID)
}

func (m *Manager) gcOrphanRuntimeSessions(known map[string]struct{}, opts GCOptions) ([]GCItem, error) {
	providers := []string{"local", "firecracker"}
	items := make([]GCItem, 0)

	for _, provider := range providers {
		base := filepath.Join(m.runtimeRoot, provider)
		entries, err := os.ReadDir(base)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		runtime, _, err := m.runtimeForProvider(provider)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			sessionID := entry.Name()
			if _, ok := known[sessionID]; ok {
				continue
			}

			info, err := runtime.Inspect(sessionID)
			if err != nil {
				return nil, err
			}

			item := GCItem{
				SessionID: sessionID,
				Provider:  provider,
				Status:    "orphaned",
			}

			if info != nil && info.Running && !opts.Force {
				item.Reason = "orphan runtime is still running"
				items = append(items, item)
				continue
			}

			item.Removed = true
			if info == nil || !info.Exists {
				item.Reason = "orphan runtime artifacts are already missing"
			} else if info.Running && opts.Force {
				item.Reason = "force cleanup requested for orphan runtime"
				if !opts.DryRun {
					if err := runtime.Stop(sessionID); err != nil {
						return nil, err
					}
				}
			} else {
				item.Reason = "orphan runtime directory found"
				if !opts.DryRun {
					if err := runtime.Stop(sessionID); err != nil {
						return nil, err
					}
				}
			}

			items = append(items, item)
		}
	}

	return items, nil
}

func defaultKillPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func (m *Manager) gcOrphanFirecrackerProcesses(known map[string]struct{}, opts GCOptions) ([]GCItem, error) {
	processes, err := listProcessCmdlines()
	if err != nil {
		return nil, err
	}

	firecrackerRoot := filepath.Join(m.runtimeRoot, "firecracker") + string(os.PathSeparator)
	type processGroup struct {
		firecrackerPID int
		egressPID      int
	}

	groups := make(map[string]*processGroup)
	for _, proc := range processes {
		sessionID, procType := matchFirecrackerRuntimeProcess(proc.Args, firecrackerRoot)
		if sessionID == "" {
			continue
		}
		if _, ok := known[sessionID]; ok {
			continue
		}
		if dirExists(filepath.Join(m.runtimeRoot, "firecracker", sessionID)) {
			continue
		}
		group := groups[sessionID]
		if group == nil {
			group = &processGroup{}
			groups[sessionID] = group
		}
		if procType == "firecracker" {
			group.firecrackerPID = proc.PID
		}
		if procType == "egress-proxy" {
			group.egressPID = proc.PID
		}
	}

	items := make([]GCItem, 0, len(groups))
	for sessionID, group := range groups {
		item := GCItem{
			SessionID: sessionID,
			Provider:  "firecracker",
			Status:    "orphaned",
		}
		if !opts.Force {
			item.Reason = "orphan firecracker processes are still running"
			items = append(items, item)
			continue
		}

		item.Removed = true
		item.Reason = "force cleanup requested for orphan firecracker processes"
		if !opts.DryRun {
			if group.egressPID > 0 {
				if err := killPID(group.egressPID); err != nil {
					return nil, err
				}
			}
			if group.firecrackerPID > 0 {
				if err := killPID(group.firecrackerPID); err != nil {
					return nil, err
				}
			}
			_ = os.RemoveAll(filepath.Join(m.runtimeRoot, "firecracker", sessionID))
		}
		items = append(items, item)
	}

	return items, nil
}

func matchFirecrackerRuntimeProcess(args []string, firecrackerRoot string) (string, string) {
	for _, arg := range args {
		idx := strings.Index(arg, firecrackerRoot)
		if idx < 0 {
			continue
		}
		rel := strings.TrimPrefix(arg[idx:], firecrackerRoot)
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		sessionID := parts[0]
		if strings.Contains(arg, "firecracker.sock") {
			return sessionID, "firecracker"
		}
		if strings.Contains(arg, "firecracker.vsock_") {
			return sessionID, "egress-proxy"
		}
	}
	return "", ""
}

func defaultListProcessCmdlines() ([]processCmdline, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	items := make([]processCmdline, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue
		}
		parts := strings.Split(string(raw), "\x00")
		args := make([]string, 0, len(parts))
		for _, part := range parts {
			if part != "" {
				args = append(args, part)
			}
		}
		if len(args) == 0 {
			continue
		}
		items = append(items, processCmdline{PID: pid, Args: args})
	}
	return items, nil
}

func (m *Manager) syncSessionState(s *model.Session, info *vm.InspectInfo) error {
	next := "stopped"
	if info != nil && info.Exists && info.Running {
		next = "running"
	}

	if s.Status == next {
		return nil
	}

	s.Status = next
	return m.store.Save(s)
}

func (m *Manager) runtimeForProvider(provider string) (vm.Runtime, string, error) {
	return m.runtimeForCreateOptions(CreateOptions{Provider: provider})
}

func (m *Manager) runtimeForCreateOptions(opts CreateOptions) (vm.Runtime, string, error) {
	cfg, resolvedProvider := m.configForCreateOptions(opts)
	if resolvedProvider == m.provider &&
		normalizeSessionNetwork(opts.Network) == normalizeSessionNetwork(m.runtimeConfig.Network) &&
		cfg.MemoryMiB == m.runtimeConfig.MemoryMiB &&
		cfg.VCPUCount == m.runtimeConfig.VCPUCount &&
		cfg.StorageMiB == m.runtimeConfig.StorageMiB {
		return m.vm, resolvedProvider, nil
	}

	runtime, err := vm.NewWithConfig(cfg)
	if err != nil {
		return nil, "", err
	}
	return runtime, resolvedProvider, nil
}

func (m *Manager) configForCreateOptions(opts CreateOptions) (vm.Config, string) {
	cfg := m.runtimeConfig
	cfg.Root = m.runtimeRoot

	resolvedProvider := opts.Provider
	if resolvedProvider == "" {
		resolvedProvider = m.provider
	}
	cfg.Provider = resolvedProvider
	cfg.Network = normalizeSessionNetwork(opts.Network)
	if opts.MemoryMiB > 0 {
		cfg.MemoryMiB = opts.MemoryMiB
	}
	if opts.VCPUCount > 0 {
		cfg.VCPUCount = opts.VCPUCount
	}
	if opts.StorageMiB > 0 {
		cfg.StorageMiB = opts.StorageMiB
	}

	return cfg, resolvedProvider
}

func validateRunOptions(opts RunOptions) error {
	if err := vm.ValidateNetworkMode(opts.Network); err != nil {
		return err
	}
	if opts.Timeout < 0 {
		return errors.New("timeout must be greater than 0")
	}
	if opts.MemoryMiB < 0 {
		return errors.New("memory-mib must be greater than 0")
	}
	if opts.VCPUCount < 0 {
		return errors.New("vcpu-count must be greater than 0")
	}
	if opts.StorageMiB < 0 {
		return errors.New("storage-mib must be greater than 0")
	}
	if opts.WorkspacePath != "" {
		info, err := os.Stat(opts.WorkspacePath)
		if err != nil {
			return fmt.Errorf("workspace is unavailable: %w", err)
		}
		if !info.IsDir() {
			return errors.New("workspace must be a directory")
		}
	}
	return nil
}

func normalizeSessionNetwork(network string) string {
	if strings.TrimSpace(network) == "" {
		return vm.DefaultNetworkMode()
	}
	return network
}

func prepareExportOutputPath(path string, force bool) error {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return os.MkdirAll(path, 0o755)
	case err != nil:
		return err
	case !info.IsDir():
		return errors.New("output path must be a directory")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if !force {
		return errors.New("output directory is not empty; use --force to overwrite")
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func extractTarGz(reader io.Reader, outputPath string) error {
	gzr, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		targetPath := filepath.Join(outputPath, header.Name)
		cleanOutput := filepath.Clean(outputPath)
		cleanTarget := filepath.Clean(targetPath)
		if cleanTarget != cleanOutput && !strings.HasPrefix(cleanTarget, cleanOutput+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry escapes output directory: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanTarget, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, cleanTarget); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry type: %d", header.Typeflag)
		}
	}
}
