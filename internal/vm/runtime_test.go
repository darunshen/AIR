package vm

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestNewDefaultsToLocalRuntime(t *testing.T) {
	t.Helper()

	t.Setenv("AIR_VM_RUNTIME", "")
	t.Setenv("AIR_FIRECRACKER_BIN", "")
	t.Setenv("AIR_FIRECRACKER_KERNEL", "")
	t.Setenv("AIR_FIRECRACKER_ROOTFS", "")
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
