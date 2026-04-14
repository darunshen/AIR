package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type firecrackerRuntime struct {
	root              string
	binary            string
	kernelImage       string
	rootfsImage       string
	kvmDevice         string
	vsockCIDBase      uint32
	startupTimeout    time.Duration
	httpClientTimeout time.Duration
	waitForSocketFn   func(string, <-chan error) error
	newUnixClientFn   func(string) *http.Client
	putJSONFn         func(*http.Client, string, any) error
}

type firecrackerMachineConfig struct {
	VCPUCount  int  `json:"vcpu_count"`
	MemSizeMiB int  `json:"mem_size_mib"`
	Smt        bool `json:"smt"`
}

type firecrackerBootSource struct {
	KernelImagePath string `json:"kernel_image_path"`
	BootArgs        string `json:"boot_args"`
}

type firecrackerDrive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
}

type firecrackerVsock struct {
	GuestCID uint32 `json:"guest_cid"`
	UdsPath  string `json:"uds_path"`
	VsockID  string `json:"vsock_id"`
}

type firecrackerAction struct {
	ActionType string `json:"action_type"`
}

type firecrackerPaths struct {
	base              string
	socketPath        string
	consolePath       string
	metricsPath       string
	pidPath           string
	vsockPath         string
	configDir         string
	machineConfigPath string
	bootSourcePath    string
	rootfsConfigPath  string
	vsockConfigPath   string
}

func newFirecrackerRuntime(cfg Config) (Runtime, error) {
	return &firecrackerRuntime{
		root:              filepath.Join(cfg.Root, "firecracker"),
		binary:            cfg.FirecrackerBinary,
		kernelImage:       cfg.KernelImage,
		rootfsImage:       cfg.RootfsImage,
		kvmDevice:         cfg.KVMDevice,
		vsockCIDBase:      cfg.VSockCIDBase,
		startupTimeout:    5 * time.Second,
		httpClientTimeout: 3 * time.Second,
	}, nil
}

func (r *firecrackerRuntime) Start(sessionID string) (string, error) {
	if err := r.preflight(); err != nil {
		return "", err
	}

	paths := r.paths(sessionID)
	if err := os.MkdirAll(paths.configDir, 0o755); err != nil {
		return "", err
	}

	_ = os.Remove(paths.socketPath)
	_ = os.Remove(paths.vsockPath)

	consoleFile, err := os.OpenFile(paths.consolePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer consoleFile.Close()

	if err := touchFile(paths.metricsPath); err != nil {
		return "", err
	}

	payloads := r.payloads(sessionID, paths)
	if err := r.writeConfigSnapshot(paths, payloads); err != nil {
		return "", err
	}

	cmd := exec.Command(r.binary, "--api-sock", paths.socketPath)
	cmd.Stdout = consoleFile
	cmd.Stderr = consoleFile
	cmd.Dir = paths.base

	if err := cmd.Start(); err != nil {
		return "", err
	}

	if err := os.WriteFile(paths.pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		_ = cmd.Process.Kill()
		return "", err
	}

	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- cmd.Wait()
	}()

	waitForSocketFn := r.waitForSocketFn
	if waitForSocketFn == nil {
		waitForSocketFn = r.waitForSocket
	}
	if err := waitForSocketFn(paths.socketPath, waitErrCh); err != nil {
		_ = cmd.Process.Kill()
		return "", err
	}

	newUnixClientFn := r.newUnixClientFn
	if newUnixClientFn == nil {
		newUnixClientFn = r.newUnixClient
	}
	putJSONFn := r.putJSONFn
	if putJSONFn == nil {
		putJSONFn = r.putJSON
	}

	client := newUnixClientFn(paths.socketPath)
	if err := putJSONFn(client, "/machine-config", payloads.machineConfig); err != nil {
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("configure firecracker machine config: %w", err)
	}

	if err := putJSONFn(client, "/boot-source", payloads.bootSource); err != nil {
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("configure firecracker boot source: %w", err)
	}

	if err := putJSONFn(client, "/drives/rootfs", payloads.rootfsDrive); err != nil {
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("configure firecracker rootfs drive: %w", err)
	}

	if err := putJSONFn(client, "/vsock", payloads.vsockConfig); err != nil {
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("configure firecracker vsock: %w", err)
	}

	if err := putJSONFn(client, "/actions", firecrackerAction{
		ActionType: "InstanceStart",
	}); err != nil {
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("start firecracker instance: %w", err)
	}

	return sessionID, nil
}

func (r *firecrackerRuntime) Exec(sessionID, command string, timeout time.Duration) (*ExecResult, error) {
	_ = command
	_ = timeout

	paths := r.paths(sessionID)
	if _, err := os.Stat(paths.vsockPath); err != nil {
		return nil, errGuestAgentNotReady(sessionID, paths.vsockPath)
	}

	return nil, fmt.Errorf("%w for session %s: host/guest vsock exec protocol is not implemented yet", ErrGuestAgentNotReady, sessionID)
}

func (r *firecrackerRuntime) Stop(vmid string) error {
	paths := r.paths(vmid)

	pidRaw, err := os.ReadFile(paths.pidPath)
	if err != nil {
		return fmt.Errorf("read firecracker pid: %w", err)
	}

	pid, err := strconv.Atoi(string(bytes.TrimSpace(pidRaw)))
	if err != nil {
		return fmt.Errorf("parse firecracker pid: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err == nil {
		_ = process.Kill()
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	return os.RemoveAll(paths.base)
}

func (r *firecrackerRuntime) Inspect(sessionID string) (*InspectInfo, error) {
	paths := r.paths(sessionID)

	_, err := os.Stat(paths.base)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	info := &InspectInfo{
		Provider:    "firecracker",
		SessionID:   sessionID,
		RootPath:    paths.base,
		Exists:      exists,
		ConsolePath: paths.consolePath,
		SocketPath:  paths.socketPath,
		PIDPath:     paths.pidPath,
		VSockPath:   paths.vsockPath,
		MetricsPath: paths.metricsPath,
		ConfigPath:  paths.configDir,
	}

	pidRaw, err := os.ReadFile(paths.pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return info, nil
		}
		return nil, fmt.Errorf("read firecracker pid: %w", err)
	}

	pid, err := strconv.Atoi(string(bytes.TrimSpace(pidRaw)))
	if err != nil {
		return nil, fmt.Errorf("parse firecracker pid: %w", err)
	}

	info.PID = pid
	info.Running = processExists(pid)
	return info, nil
}

func (r *firecrackerRuntime) preflight() error {
	if _, err := exec.LookPath(r.binary); err != nil {
		return errFirecrackerBinaryNotFound(r.binary, err)
	}
	if r.kernelImage == "" {
		return ErrFirecrackerKernelRequired
	}
	if _, err := os.Stat(r.kernelImage); err != nil {
		return errFirecrackerKernelNotFound(r.kernelImage, err)
	}
	if r.rootfsImage == "" {
		return ErrFirecrackerRootfsRequired
	}
	if _, err := os.Stat(r.rootfsImage); err != nil {
		return errFirecrackerRootfsNotFound(r.rootfsImage, err)
	}
	if _, err := os.Stat(r.kvmDevice); err != nil {
		return errKVMDeviceNotAvailable(r.kvmDevice, err)
	}
	return nil
}

func (r *firecrackerRuntime) waitForSocket(socketPath string, waitErrCh <-chan error) error {
	deadline := time.Now().Add(r.startupTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-waitErrCh:
			if err != nil {
				return fmt.Errorf("firecracker exited before socket was ready: %w", err)
			}
			return errors.New("firecracker exited before socket was ready")
		default:
		}

		if _, err := os.Stat(socketPath); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for firecracker socket: %s", socketPath)
}

func (r *firecrackerRuntime) newUnixClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: r.httpClientTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

func (r *firecrackerRuntime) putJSON(client *http.Client, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, "http://localhost"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("firecracker api %s returned status %d", path, resp.StatusCode)
	}
	return nil
}

func cidOffset(sessionID string) uint32 {
	var total uint32
	for _, ch := range []byte(sessionID) {
		total += uint32(ch)
	}
	return total % 10000
}

func processExists(pid int) bool {
	err := syscallKill(pid, 0)
	return err == nil
}

func (r *firecrackerRuntime) paths(sessionID string) firecrackerPaths {
	base := sessionRoot(r.root, sessionID)
	configDir := filepath.Join(base, "config")
	return firecrackerPaths{
		base:              base,
		socketPath:        filepath.Join(base, "firecracker.sock"),
		consolePath:       filepath.Join(base, "console.log"),
		metricsPath:       filepath.Join(base, "metrics.log"),
		pidPath:           filepath.Join(base, "firecracker.pid"),
		vsockPath:         filepath.Join(base, "firecracker.vsock"),
		configDir:         configDir,
		machineConfigPath: filepath.Join(configDir, "machine-config.json"),
		bootSourcePath:    filepath.Join(configDir, "boot-source.json"),
		rootfsConfigPath:  filepath.Join(configDir, "rootfs-drive.json"),
		vsockConfigPath:   filepath.Join(configDir, "vsock.json"),
	}
}

type firecrackerPayloads struct {
	machineConfig firecrackerMachineConfig
	bootSource    firecrackerBootSource
	rootfsDrive   firecrackerDrive
	vsockConfig   firecrackerVsock
}

func (r *firecrackerRuntime) payloads(sessionID string, paths firecrackerPaths) firecrackerPayloads {
	return firecrackerPayloads{
		machineConfig: firecrackerMachineConfig{
			VCPUCount:  1,
			MemSizeMiB: 256,
			Smt:        false,
		},
		bootSource: firecrackerBootSource{
			KernelImagePath: r.kernelImage,
			BootArgs:        "console=ttyS0 reboot=k panic=1 pci=off",
		},
		rootfsDrive: firecrackerDrive{
			DriveID:      "rootfs",
			PathOnHost:   r.rootfsImage,
			IsRootDevice: true,
			IsReadOnly:   false,
		},
		vsockConfig: firecrackerVsock{
			VsockID:  "root",
			GuestCID: r.vsockCIDBase + cidOffset(sessionID),
			UdsPath:  paths.vsockPath,
		},
	}
}

func (r *firecrackerRuntime) writeConfigSnapshot(paths firecrackerPaths, payloads firecrackerPayloads) error {
	for filePath, payload := range map[string]any{
		paths.machineConfigPath: payloads.machineConfig,
		paths.bootSourcePath:    payloads.bootSource,
		paths.rootfsConfigPath:  payloads.rootfsDrive,
		paths.vsockConfigPath:   payloads.vsockConfig,
	} {
		if err := writeJSONFile(filePath, payload); err != nil {
			return err
		}
	}
	return nil
}

func touchFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func writeJSONFile(path string, payload any) error {
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}
