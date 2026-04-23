package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Provider  string
	Timeout   time.Duration
	MemoryMiB int
	VCPUCount int
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
	Provider  string
	MemoryMiB int
	VCPUCount int
}

type Manager struct {
	store         *store.Store
	runtimeRoot   string
	runtimeConfig vm.Config
	provider      string
	vm            vm.Runtime
}

func NewManager() (*Manager, error) {
	return NewManagerWithPaths("data/sessions.json", "runtime/sessions")
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

	vmid, err := runtime.Start(id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	s := &model.Session{
		ID:         id,
		VMID:       vmid,
		Provider:   resolvedProvider,
		Status:     "running",
		CreatedAt:  now,
		LastUsedAt: now,
	}

	if err := m.store.Save(s); err != nil {
		return nil, err
	}

	return s, nil
}

func (m *Manager) Exec(sessionID, command string) (*ExecResult, error) {
	return m.ExecWithTimeout(sessionID, command, 30*time.Second)
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
		Provider:  opts.Provider,
		MemoryMiB: opts.MemoryMiB,
		VCPUCount: opts.VCPUCount,
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
		cfg.MemoryMiB == m.runtimeConfig.MemoryMiB &&
		cfg.VCPUCount == m.runtimeConfig.VCPUCount {
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
	if opts.MemoryMiB > 0 {
		cfg.MemoryMiB = opts.MemoryMiB
	}
	if opts.VCPUCount > 0 {
		cfg.VCPUCount = opts.VCPUCount
	}

	return cfg, resolvedProvider
}

func validateRunOptions(opts RunOptions) error {
	if opts.Timeout < 0 {
		return errors.New("timeout must be greater than 0")
	}
	if opts.MemoryMiB < 0 {
		return errors.New("memory-mib must be greater than 0")
	}
	if opts.VCPUCount < 0 {
		return errors.New("vcpu-count must be greater than 0")
	}
	return nil
}
