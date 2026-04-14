package vm

import (
	"os"
	"path/filepath"
	"time"
)

type Runtime interface {
	Start(sessionID string) (string, error)
	Exec(sessionID, command string, timeout time.Duration) (*ExecResult, error)
	Stop(vmid string) error
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Config struct {
	Root              string
	Provider          string
	FirecrackerBinary string
	KernelImage       string
	RootfsImage       string
	KVMDevice         string
	VSockCIDBase      uint32
}

func New(root string) (Runtime, error) {
	return NewWithConfig(resolveRuntimeConfig(root))
}

func NewWithConfig(cfg Config) (Runtime, error) {
	if cfg.Root == "" {
		cfg.Root = defaultRuntimeRoot
	}
	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	cfg.Root = absRoot
	if cfg.Provider == "" {
		cfg.Provider = "local"
	}
	if cfg.VSockCIDBase == 0 {
		cfg.VSockCIDBase = defaultVSockCIDBase
	}
	if cfg.KVMDevice == "" {
		cfg.KVMDevice = defaultKVMDevice
	}

	if err := os.MkdirAll(cfg.Root, 0o755); err != nil {
		return nil, err
	}

	switch cfg.Provider {
	case "local":
		return newLocalRuntime(cfg)
	case "firecracker":
		return newFirecrackerRuntime(cfg)
	default:
		return nil, ErrUnsupportedProvider(cfg.Provider)
	}
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func sessionRoot(root, sessionID string) string {
	return filepath.Join(root, sessionID)
}
