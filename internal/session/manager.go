package session

import (
	"errors"
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
	store *store.Store
	vm    vm.Runtime
}

func NewManager() (*Manager, error) {
	return NewManagerWithPaths("data/sessions.json", "runtime/sessions")
}

func NewManagerWithPaths(storePath, runtimeRoot string) (*Manager, error) {
	s, err := store.New(storePath)
	if err != nil {
		return nil, err
	}

	r, err := vm.New(runtimeRoot)
	if err != nil {
		return nil, err
	}

	return &Manager{store: s, vm: r}, nil
}

func (m *Manager) Create() (*model.Session, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	vmid, err := m.vm.Start(id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	s := &model.Session{
		ID:         id,
		VMID:       vmid,
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
	if s.Status != "running" {
		return nil, errors.New("session is not running")
	}

	result, err := m.vm.Exec(sessionID, command, 30*time.Second)
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

	if err := m.vm.Stop(s.VMID); err != nil {
		return err
	}

	return m.store.Delete(sessionID)
}
