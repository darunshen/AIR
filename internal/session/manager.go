package session

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/darunshen/AIR/internal/model"
	"github.com/darunshen/AIR/internal/store"
	"github.com/darunshen/AIR/internal/vm"
)

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
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
	runtime, resolvedProvider, err := m.runtimeForProvider(provider)
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

	result, err := runtime.Exec(sessionID, command, 30*time.Second)
	if err != nil {
		return nil, err
	}

	s.LastUsedAt = time.Now().UTC()
	if err := m.store.Save(s); err != nil {
		return nil, err
	}

	return &ExecResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, nil
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
	resolvedProvider := provider
	if resolvedProvider == "" {
		resolvedProvider = m.provider
	}
	if resolvedProvider == m.provider {
		return m.vm, resolvedProvider, nil
	}

	cfg := m.runtimeConfig
	cfg.Root = m.runtimeRoot
	cfg.Provider = resolvedProvider

	runtime, err := vm.NewWithConfig(cfg)
	if err != nil {
		return nil, "", err
	}
	return runtime, resolvedProvider, nil
}
