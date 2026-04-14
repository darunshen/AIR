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
	return NewWithConfig(Config{
		Root:              root,
		Provider:          getenvDefault("AIR_VM_RUNTIME", "local"),
		FirecrackerBinary: getenvDefault("AIR_FIRECRACKER_BIN", "firecracker"),
		KernelImage:       os.Getenv("AIR_FIRECRACKER_KERNEL"),
		RootfsImage:       os.Getenv("AIR_FIRECRACKER_ROOTFS"),
		KVMDevice:         getenvDefault("AIR_KVM_DEVICE", "/dev/kvm"),
		VSockCIDBase:      100,
	})
}

func NewWithConfig(cfg Config) (Runtime, error) {
	if cfg.Root == "" {
		cfg.Root = "runtime/sessions"
	}
	if cfg.Provider == "" {
		cfg.Provider = "local"
	}
	if cfg.VSockCIDBase == 0 {
		cfg.VSockCIDBase = 100
	}
	if cfg.KVMDevice == "" {
		cfg.KVMDevice = "/dev/kvm"
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
