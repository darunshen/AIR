package vm

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/darunshen/AIR/internal/guestapi"
)

func TestFirecrackerStartStopWithFakeBinary(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	kernel := filepath.Join(root, "vmlinux.bin")
	rootfs := filepath.Join(root, "rootfs.ext4")
	kvm := filepath.Join(root, "kvm")
	bin := filepath.Join(root, "fake-firecracker")

	for _, path := range []string{kernel, rootfs, kvm} {
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}

	script := `#!/bin/sh
sock=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--api-sock" ]; then
    sock="$2"
    shift 2
    continue
  fi
  shift
done
if [ -z "$sock" ]; then
  exit 2
fi
touch "$sock"
trap 'rm -f "$sock"; exit 0' TERM INT
while true; do sleep 1; done
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake firecracker: %v", err)
	}

	rtAny, err := NewWithConfig(Config{
		Root:              filepath.Join(root, "runtime"),
		Provider:          "firecracker",
		FirecrackerBinary: bin,
		KernelImage:       kernel,
		RootfsImage:       rootfs,
		KVMDevice:         kvm,
		VSockCIDBase:      100,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	rt, ok := rtAny.(*firecrackerRuntime)
	if !ok {
		t.Fatal("expected firecracker runtime")
	}

	rt.waitForSocketFn = func(socketPath string, waitErrCh <-chan error) error {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(socketPath); err == nil {
				return nil
			}
			time.Sleep(25 * time.Millisecond)
		}
		return os.ErrNotExist
	}
	rt.newUnixClientFn = func(string) *http.Client {
		return &http.Client{}
	}
	rt.putJSONFn = func(_ *http.Client, _ string, _ any) error {
		return nil
	}
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	rt.dialVSockFn = func(string, uint32, time.Duration) (net.Conn, error) {
		return clientConn, nil
	}
	go func() {
		var req guestapi.ExecRequest
		if err := json.NewDecoder(serverConn).Decode(&req); err != nil {
			t.Errorf("decode ready request: %v", err)
			return
		}
		if err := json.NewEncoder(serverConn).Encode(guestapi.ReadyResult{
			Type:      guestapi.MessageTypeReady,
			RequestID: req.RequestID,
			Status:    "ready",
		}); err != nil {
			t.Errorf("encode ready response: %v", err)
		}
	}()

	vmid, err := rt.Start("sess_firecracker")
	if err != nil {
		t.Fatalf("start firecracker runtime: %v", err)
	}
	if vmid != "sess_firecracker" {
		t.Fatalf("unexpected vmid: %s", vmid)
	}

	base := filepath.Join(root, "runtime", "firecracker", "sess_firecracker")
	for _, path := range []string{
		filepath.Join(base, "rootfs.ext4"),
		filepath.Join(base, "firecracker.sock"),
		filepath.Join(base, "firecracker.pid"),
		filepath.Join(base, "console.log"),
		filepath.Join(base, "metrics.log"),
		filepath.Join(base, "events.jsonl"),
		filepath.Join(base, "config", "machine-config.json"),
		filepath.Join(base, "config", "boot-source.json"),
		filepath.Join(base, "config", "rootfs-drive.json"),
		filepath.Join(base, "config", "vsock.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}

	info, err := rt.Inspect(vmid)
	if err != nil {
		t.Fatalf("inspect firecracker runtime: %v", err)
	}
	if info.Provider != "firecracker" {
		t.Fatalf("unexpected provider: %s", info.Provider)
	}
	if !info.Exists || !info.Running {
		t.Fatalf("expected running firecracker runtime, got exists=%v running=%v", info.Exists, info.Running)
	}
	if info.ConsolePath == "" || info.SocketPath == "" || info.PIDPath == "" || info.ConfigPath == "" {
		t.Fatalf("expected populated firecracker inspect info, got %+v", info)
	}
	if info.OverlayPath != filepath.Join(base, "rootfs.ext4") {
		t.Fatalf("unexpected overlay path: %s", info.OverlayPath)
	}
	if info.EventsPath != filepath.Join(base, "events.jsonl") {
		t.Fatalf("unexpected events path: %s", info.EventsPath)
	}

	rootfsConfigBody, err := os.ReadFile(filepath.Join(base, "config", "rootfs-drive.json"))
	if err != nil {
		t.Fatalf("read rootfs config: %v", err)
	}
	if !strings.Contains(string(rootfsConfigBody), filepath.Join(base, "rootfs.ext4")) {
		t.Fatalf("expected rootfs config to point to overlay, got %s", string(rootfsConfigBody))
	}

	eventsBody, err := os.ReadFile(filepath.Join(base, "events.jsonl"))
	if err != nil {
		t.Fatalf("read events log: %v", err)
	}
	for _, marker := range []string{"session_starting", "firecracker_started", "guest_ready"} {
		if !strings.Contains(string(eventsBody), marker) {
			t.Fatalf("expected events log to contain %q, got %s", marker, string(eventsBody))
		}
	}

	if err := rt.Stop(vmid); err != nil {
		t.Fatalf("stop firecracker runtime: %v", err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("expected runtime directory to be removed, got err=%v", err)
	}
}

func TestFirecrackerDialTCPViaGuestProxy(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	rtAny, err := NewWithConfig(Config{
		Root:         filepath.Join(root, "runtime"),
		Provider:     "firecracker",
		VSockCIDBase: 100,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	rt, ok := rtAny.(*firecrackerRuntime)
	if !ok {
		t.Fatal("expected firecracker runtime")
	}

	clientConn, serverConn := net.Pipe()
	rt.dialVSockFn = func(string, uint32, time.Duration) (net.Conn, error) {
		return clientConn, nil
	}

	go func() {
		defer serverConn.Close()
		reader := bufio.NewReader(serverConn)

		var req guestapi.ExecRequest
		if err := json.NewDecoder(reader).Decode(&req); err != nil {
			t.Errorf("decode proxy request: %v", err)
			return
		}
		if req.Type != guestapi.MessageTypeProxy {
			t.Errorf("unexpected proxy request type: %s", req.Type)
			return
		}
		if req.Address != "127.0.0.1:50051" {
			t.Errorf("unexpected proxy address: %s", req.Address)
			return
		}
		if err := json.NewEncoder(serverConn).Encode(guestapi.ProxyResult{
			Type:      guestapi.MessageTypeProxy,
			RequestID: req.RequestID,
			Status:    "connected",
		}); err != nil {
			t.Errorf("encode proxy result: %v", err)
			return
		}
		buf := make([]byte, 4)
		if _, err := io.ReadFull(reader, buf); err != nil {
			t.Errorf("read proxied payload: %v", err)
			return
		}
		if string(buf) != "ping" {
			t.Errorf("unexpected proxied payload: %q", string(buf))
			return
		}
		if _, err := serverConn.Write([]byte("pong")); err != nil {
			t.Errorf("write proxied response: %v", err)
		}
	}()

	conn, err := rt.DialTCP("sess_proxy", "127.0.0.1:50051", 2*time.Second)
	if err != nil {
		t.Fatalf("dial tcp via guest proxy: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write to proxy conn: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read from proxy conn: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("unexpected proxy conn response: %q", string(buf))
	}
}

func TestFirecrackerPreflightRequiresAssets(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	kernel := filepath.Join(root, "vmlinux.bin")
	rootfs := filepath.Join(root, "rootfs.ext4")
	kvm := filepath.Join(root, "kvm")
	bin := filepath.Join(root, "fake-firecracker")

	for _, path := range []string{kernel, rootfs, kvm, bin} {
		if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}

	tests := []struct {
		name string
		cfg  Config
		want error
	}{
		{
			name: "missing binary",
			cfg: Config{
				Root:              root,
				Provider:          "firecracker",
				FirecrackerBinary: filepath.Join(root, "does-not-exist"),
				KernelImage:       kernel,
				RootfsImage:       rootfs,
				KVMDevice:         kvm,
			},
			want: ErrFirecrackerBinaryNotFound,
		},
		{
			name: "missing kernel env",
			cfg: Config{
				Root:              root,
				Provider:          "firecracker",
				FirecrackerBinary: bin,
				RootfsImage:       rootfs,
				KVMDevice:         kvm,
			},
			want: ErrFirecrackerKernelRequired,
		},
		{
			name: "missing kernel file",
			cfg: Config{
				Root:              root,
				Provider:          "firecracker",
				FirecrackerBinary: bin,
				KernelImage:       filepath.Join(root, "missing-kernel"),
				RootfsImage:       rootfs,
				KVMDevice:         kvm,
			},
			want: ErrFirecrackerKernelNotFound,
		},
		{
			name: "missing rootfs env",
			cfg: Config{
				Root:              root,
				Provider:          "firecracker",
				FirecrackerBinary: bin,
				KernelImage:       kernel,
				KVMDevice:         kvm,
			},
			want: ErrFirecrackerRootfsRequired,
		},
		{
			name: "missing rootfs file",
			cfg: Config{
				Root:              root,
				Provider:          "firecracker",
				FirecrackerBinary: bin,
				KernelImage:       kernel,
				RootfsImage:       filepath.Join(root, "missing-rootfs"),
				KVMDevice:         kvm,
			},
			want: ErrFirecrackerRootfsNotFound,
		},
		{
			name: "missing kvm device",
			cfg: Config{
				Root:              root,
				Provider:          "firecracker",
				FirecrackerBinary: bin,
				KernelImage:       kernel,
				RootfsImage:       rootfs,
				KVMDevice:         filepath.Join(root, "missing-kvm"),
			},
			want: ErrKVMDeviceNotAvailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rtAny, err := NewWithConfig(tt.cfg)
			if err != nil {
				t.Fatalf("new runtime: %v", err)
			}

			rt := rtAny.(*firecrackerRuntime)
			err = rt.preflight()
			if err == nil {
				t.Fatal("expected preflight failure")
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestFirecrackerPayloadsUseConfiguredResources(t *testing.T) {
	t.Helper()

	rtAny, err := NewWithConfig(Config{
		Root:      t.TempDir(),
		Provider:  "firecracker",
		MemoryMiB: 768,
		VCPUCount: 2,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	rt, ok := rtAny.(*firecrackerRuntime)
	if !ok {
		t.Fatalf("expected firecracker runtime, got %T", rtAny)
	}

	payloads := rt.payloads("sess_resources", rt.paths("sess_resources"))
	if payloads.machineConfig.MemSizeMiB != 768 {
		t.Fatalf("unexpected memory size: %d", payloads.machineConfig.MemSizeMiB)
	}
	if payloads.machineConfig.VCPUCount != 2 {
		t.Fatalf("unexpected vcpu count: %d", payloads.machineConfig.VCPUCount)
	}
	if payloads.bootSource.BootArgs != defaultFirecrackerBootArgs {
		t.Fatalf("unexpected boot args: %s", payloads.bootSource.BootArgs)
	}
}

func TestFirecrackerPayloadsUseConfiguredBootArgs(t *testing.T) {
	t.Helper()

	rtAny, err := NewWithConfig(Config{
		Root:     t.TempDir(),
		Provider: "firecracker",
		BootArgs: "console=ttyS0 init=/sbin/init",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	rt, ok := rtAny.(*firecrackerRuntime)
	if !ok {
		t.Fatalf("expected firecracker runtime, got %T", rtAny)
	}

	payloads := rt.payloads("sess_boot_args", rt.paths("sess_boot_args"))
	if payloads.bootSource.BootArgs != "console=ttyS0 init=/sbin/init" {
		t.Fatalf("unexpected boot args: %s", payloads.bootSource.BootArgs)
	}
}

func TestFirecrackerExecOverVSockBridge(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	rtAny, err := NewWithConfig(Config{
		Root:     root,
		Provider: "firecracker",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	rt, ok := rtAny.(*firecrackerRuntime)
	if !ok {
		t.Fatalf("expected firecracker runtime, got %T", rtAny)
	}

	sessionID := "sess_exec"
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	rt.dialVSockFn = func(string, uint32, time.Duration) (net.Conn, error) {
		return clientConn, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		reader := bufio.NewReader(serverConn)
		var req guestapi.ExecRequest
		if err := json.NewDecoder(reader).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if req.Type != guestapi.MessageTypeExec {
			t.Errorf("unexpected request type: %s", req.Type)
			return
		}
		if req.Command != "echo hello" {
			t.Errorf("unexpected command: %s", req.Command)
			return
		}

		if err := json.NewEncoder(serverConn).Encode(guestapi.ExecResult{
			Type:      guestapi.MessageTypeResult,
			RequestID: req.RequestID,
			Stdout:    "hello\n",
			ExitCode:  0,
		}); err != nil {
			t.Errorf("encode result: %v", err)
		}
	}()

	result, err := rt.Exec(sessionID, "echo hello", 3*time.Second)
	if err != nil {
		t.Fatalf("exec via vsock bridge: %v", err)
	}
	if result.RequestID == "" {
		t.Fatal("expected request id")
	}
	if result.Duration <= 0 {
		t.Fatalf("expected positive duration, got %s", result.Duration)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", result.ExitCode)
	}

	eventsBody, err := os.ReadFile(filepath.Join(root, "firecracker", sessionID, "events.jsonl"))
	if err != nil {
		t.Fatalf("read events log: %v", err)
	}
	if !strings.Contains(string(eventsBody), "exec_completed") || !strings.Contains(string(eventsBody), result.RequestID) {
		t.Fatalf("expected exec event in events log, got %s", string(eventsBody))
	}

	<-done
}

func TestFirecrackerExecReturnsGuestNotReady(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	rtAny, err := NewWithConfig(Config{
		Root:     root,
		Provider: "firecracker",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	rt, ok := rtAny.(*firecrackerRuntime)
	if !ok {
		t.Fatalf("expected firecracker runtime, got %T", rtAny)
	}

	_, err = rt.Exec("sess_missing", "echo hello", time.Second)
	if !errors.Is(err, ErrGuestAgentNotReady) {
		t.Fatalf("expected ErrGuestAgentNotReady, got %v", err)
	}
}

func TestFirecrackerWaitForGuestReady(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	rtAny, err := NewWithConfig(Config{
		Root:     root,
		Provider: "firecracker",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	rt, ok := rtAny.(*firecrackerRuntime)
	if !ok {
		t.Fatalf("expected firecracker runtime, got %T", rtAny)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	rt.dialVSockFn = func(string, uint32, time.Duration) (net.Conn, error) {
		return clientConn, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		reader := bufio.NewReader(serverConn)
		var req guestapi.ExecRequest
		if err := json.NewDecoder(reader).Decode(&req); err != nil {
			t.Errorf("decode ready request: %v", err)
			return
		}
		if req.Type != guestapi.MessageTypeReady {
			t.Errorf("unexpected request type: %s", req.Type)
			return
		}
		if err := json.NewEncoder(serverConn).Encode(guestapi.ReadyResult{
			Type:      guestapi.MessageTypeReady,
			RequestID: req.RequestID,
			Status:    "ready",
		}); err != nil {
			t.Errorf("encode ready result: %v", err)
		}
	}()

	if err := rt.waitForGuestReady("sess_ready", firecrackerPaths{
		vsockPath: filepath.Join(root, "firecracker.vsock"),
	}); err != nil {
		t.Fatalf("wait for guest ready: %v", err)
	}

	<-done
}

func TestPerformVSockHandshake(t *testing.T) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)

		reader := bufio.NewReader(serverConn)
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Errorf("read handshake: %v", err)
			return
		}
		if strings.TrimSpace(line) != "CONNECT 10789" {
			t.Errorf("unexpected handshake: %q", line)
			return
		}
		if _, err := serverConn.Write([]byte("OK\n")); err != nil {
			t.Errorf("write handshake response: %v", err)
			return
		}

		var req guestapi.ExecRequest
		if err := json.NewDecoder(reader).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if req.Command != "echo ready" {
			t.Errorf("unexpected command: %s", req.Command)
			return
		}
	}()

	conn, err := performVSockHandshake(clientConn, guestapi.DefaultVSockPort)
	if err != nil {
		t.Fatalf("perform handshake: %v", err)
	}

	if err := json.NewEncoder(conn).Encode(guestapi.ExecRequest{
		Type:    guestapi.MessageTypeExec,
		Command: "echo ready",
	}); err != nil {
		t.Fatalf("encode post-handshake request: %v", err)
	}

	<-done
}

func TestFirecrackerIntegrationLifecycle(t *testing.T) {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skip("firecracker integration test requires linux")
	}
	if os.Getenv("AIR_FIRECRACKER_INTEGRATION") != "1" {
		t.Skip("set AIR_FIRECRACKER_INTEGRATION=1 to run firecracker integration test")
	}

	kernel := os.Getenv("AIR_FIRECRACKER_KERNEL")
	if kernel == "" {
		t.Skip("AIR_FIRECRACKER_KERNEL is required")
	}
	rootfs := os.Getenv("AIR_FIRECRACKER_ROOTFS")
	if rootfs == "" {
		t.Skip("AIR_FIRECRACKER_ROOTFS is required")
	}
	binary := os.Getenv("AIR_FIRECRACKER_BIN")
	if binary == "" {
		binary = "firecracker"
	}
	kvm := os.Getenv("AIR_KVM_DEVICE")
	if kvm == "" {
		kvm = "/dev/kvm"
	}

	rtAny, err := NewWithConfig(Config{
		Root:              filepath.Join(t.TempDir(), "runtime"),
		Provider:          "firecracker",
		FirecrackerBinary: binary,
		KernelImage:       kernel,
		RootfsImage:       rootfs,
		KVMDevice:         kvm,
		VSockCIDBase:      1000,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	rt := rtAny.(*firecrackerRuntime)
	vmid, err := rt.Start("integration")
	if err != nil {
		t.Fatalf("start firecracker runtime: %v", err)
	}
	defer func() {
		if err := rt.Stop(vmid); err != nil {
			t.Fatalf("stop firecracker runtime: %v", err)
		}
	}()

	paths := rt.paths(vmid)
	for _, path := range []string{paths.pidPath, paths.socketPath, paths.consolePath, paths.metricsPath, paths.eventsPath, paths.rootfsPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
	if paths.rootfsPath == rootfs {
		t.Fatalf("expected session rootfs path, got shared rootfs path %s", paths.rootfsPath)
	}

	first, err := rt.Exec(vmid, "echo integration > /tmp/air-session.txt", 10*time.Second)
	if err != nil {
		t.Fatalf("exec write integration file: %v", err)
	}
	if first.RequestID == "" {
		t.Fatal("expected request id from first exec")
	}

	second, err := rt.Exec(vmid, "cat /root/air-session.txt", 10*time.Second)
	if err != nil {
		t.Fatalf("exec read integration file: %v", err)
	}
	if strings.TrimSpace(second.Stdout) != "integration" {
		t.Fatalf("unexpected exec stdout: %q", second.Stdout)
	}

	consoleBody, err := os.ReadFile(paths.consolePath)
	if err != nil {
		t.Fatalf("read console log: %v", err)
	}
	if len(consoleBody) == 0 {
		t.Fatal("expected non-empty console log")
	}

	eventsBody, err := os.ReadFile(paths.eventsPath)
	if err != nil {
		t.Fatalf("read events log: %v", err)
	}
	for _, marker := range []string{"guest_ready", "exec_started", "exec_completed"} {
		if !strings.Contains(string(eventsBody), marker) {
			t.Fatalf("expected %q in events log, got %s", marker, string(eventsBody))
		}
	}
}
