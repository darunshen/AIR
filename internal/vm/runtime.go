package vm

import (
	"net"
	"os"
	"path/filepath"
	"time"
)

type Runtime interface {
	Start(sessionID string) (string, error)
	StartWithOptions(sessionID string, opts StartOptions) (string, error)
	Exec(sessionID, command string, timeout time.Duration) (*ExecResult, error)
	Stop(vmid string) error
	Inspect(sessionID string) (*InspectInfo, error)
}

type TCPDialer interface {
	DialTCP(sessionID, address string, timeout time.Duration) (net.Conn, error)
}

type StartOptions struct {
	WorkspacePath string
}

type ExecResult struct {
	RequestID string
	Stdout    string
	Stderr    string
	ExitCode  int
	TimedOut  bool
	Duration  time.Duration
}

type InspectInfo struct {
	Provider           string `json:"provider"`
	SessionID          string `json:"session_id"`
	RootPath           string `json:"root_path"`
	Exists             bool   `json:"exists"`
	Running            bool   `json:"running"`
	ConsolePath        string `json:"console_path,omitempty"`
	WorkspacePath      string `json:"workspace_path,omitempty"`
	TaskPath           string `json:"task_path,omitempty"`
	SocketPath         string `json:"socket_path,omitempty"`
	PIDPath            string `json:"pid_path,omitempty"`
	PID                int    `json:"pid,omitempty"`
	VSockPath          string `json:"vsock_path,omitempty"`
	MetricsPath        string `json:"metrics_path,omitempty"`
	ConfigPath         string `json:"config_path,omitempty"`
	EventsPath         string `json:"events_path,omitempty"`
	OverlayPath        string `json:"overlay_path,omitempty"`
	RootfsPath         string `json:"rootfs_path,omitempty"`
	WorkspaceImagePath string `json:"workspace_image_path,omitempty"`
	WorkspaceUpperPath string `json:"workspace_upper_path,omitempty"`
}

type Config struct {
	Root              string
	Provider          string
	FirecrackerBinary string
	KernelImage       string
	RootfsImage       string
	BootArgs          string
	KVMDevice         string
	MemoryMiB         int
	VCPUCount         int
	VSockCIDBase      uint32
}

func New(root string) (Runtime, error) {
	return NewWithConfig(ResolveConfig(root))
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
	if cfg.BootArgs == "" {
		cfg.BootArgs = defaultFirecrackerBootArgs
	}
	if cfg.MemoryMiB <= 0 {
		cfg.MemoryMiB = defaultFirecrackerMemoryMiB
	}
	if cfg.VCPUCount <= 0 {
		cfg.VCPUCount = defaultFirecrackerVCPUCount
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
