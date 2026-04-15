package vm

import (
	"bufio"
	"encoding/json"
	"errors"
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

	vmid, err := rt.Start("sess_firecracker")
	if err != nil {
		t.Fatalf("start firecracker runtime: %v", err)
	}
	if vmid != "sess_firecracker" {
		t.Fatalf("unexpected vmid: %s", vmid)
	}

	base := filepath.Join(root, "runtime", "firecracker", "sess_firecracker")
	for _, path := range []string{
		filepath.Join(base, "firecracker.sock"),
		filepath.Join(base, "firecracker.pid"),
		filepath.Join(base, "console.log"),
		filepath.Join(base, "metrics.log"),
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

	if err := rt.Stop(vmid); err != nil {
		t.Fatalf("stop firecracker runtime: %v", err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("expected runtime directory to be removed, got err=%v", err)
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
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", result.ExitCode)
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
	for _, path := range []string{paths.pidPath, paths.socketPath, paths.consolePath, paths.metricsPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
}
