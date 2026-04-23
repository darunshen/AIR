package vm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefaultsToLocalRuntime(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_VM_RUNTIME", "")
	t.Setenv("AIR_FIRECRACKER_BIN", "")
	t.Setenv("AIR_FIRECRACKER_KERNEL", "")
	t.Setenv("AIR_FIRECRACKER_ROOTFS", "")
	t.Setenv("AIR_FIRECRACKER_BOOT_ARGS", "")
	t.Setenv("AIR_KVM_DEVICE", "")

	rt, err := New(filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if _, ok := rt.(*localRuntime); !ok {
		t.Fatalf("expected local runtime, got %T", rt)
	}
}

func TestNewUsesProviderFromEnv(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_VM_RUNTIME", "firecracker")
	t.Setenv("AIR_FIRECRACKER_BIN", "firecracker-custom")
	t.Setenv("AIR_FIRECRACKER_KERNEL", "/tmp/kernel")
	t.Setenv("AIR_FIRECRACKER_ROOTFS", "/tmp/rootfs")
	t.Setenv("AIR_FIRECRACKER_BOOT_ARGS", "console=ttyS0 init=/sbin/init")
	t.Setenv("AIR_KVM_DEVICE", "/tmp/kvm")

	rt, err := New(filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	firecracker, ok := rt.(*firecrackerRuntime)
	if !ok {
		t.Fatalf("expected firecracker runtime, got %T", rt)
	}
	if firecracker.binary != "firecracker-custom" {
		t.Fatalf("unexpected firecracker binary: %s", firecracker.binary)
	}
	if firecracker.kernelImage != "/tmp/kernel" {
		t.Fatalf("unexpected kernel image: %s", firecracker.kernelImage)
	}
	if firecracker.rootfsImage != "/tmp/rootfs" {
		t.Fatalf("unexpected rootfs image: %s", firecracker.rootfsImage)
	}
	if firecracker.bootArgs != "console=ttyS0 init=/sbin/init" {
		t.Fatalf("unexpected boot args: %s", firecracker.bootArgs)
	}
	if firecracker.kvmDevice != "/tmp/kvm" {
		t.Fatalf("unexpected kvm device: %s", firecracker.kvmDevice)
	}
}

func TestNewWithConfigRejectsUnsupportedProvider(t *testing.T) {
	t.Helper()

	_, err := NewWithConfig(Config{
		Root:     filepath.Join(t.TempDir(), "runtime"),
		Provider: "unknown",
	})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}

	var providerErr unsupportedProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected unsupportedProviderError, got %T", err)
	}
}

func TestNewUsesBundledFirecrackerAssetsWhenEnvMissing(t *testing.T) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	root := t.TempDir()
	assetsDir := filepath.Join(root, "assets", "firecracker")
	for _, path := range []string{
		filepath.Join(assetsDir, "firecracker"),
		filepath.Join(assetsDir, "hello-vmlinux.bin"),
		filepath.Join(assetsDir, "hello-rootfs.ext4"),
		filepath.Join(assetsDir, "hello-rootfs-air.ext4"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir assets: %v", err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
			t.Fatalf("write asset %s: %v", path, err)
		}
	}

	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to temp repo: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	t.Setenv("AIR_VM_RUNTIME", "firecracker")
	t.Setenv("AIR_FIRECRACKER_BIN", "")
	t.Setenv("AIR_FIRECRACKER_KERNEL", "")
	t.Setenv("AIR_FIRECRACKER_ROOTFS", "")
	t.Setenv("AIR_FIRECRACKER_BOOT_ARGS", "")
	t.Setenv("AIR_KVM_DEVICE", "")

	rt, err := New(filepath.Join(root, "runtime"))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	firecracker, ok := rt.(*firecrackerRuntime)
	if !ok {
		t.Fatalf("expected firecracker runtime, got %T", rt)
	}

	if firecracker.binary != filepath.Join(root, "assets", "firecracker", "firecracker") {
		t.Fatalf("unexpected bundled firecracker binary: %s", firecracker.binary)
	}
	if firecracker.kernelImage != filepath.Join(root, "assets", "firecracker", "hello-vmlinux.bin") {
		t.Fatalf("unexpected bundled kernel image: %s", firecracker.kernelImage)
	}
	if firecracker.rootfsImage != filepath.Join(root, "assets", "firecracker", "hello-rootfs-air.ext4") {
		t.Fatalf("unexpected bundled rootfs image: %s", firecracker.rootfsImage)
	}
}
