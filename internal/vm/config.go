package vm

import (
	"os"
	"path/filepath"
)

const (
	defaultRuntimeRoot                = "runtime/sessions"
	defaultFirecrackerBinary          = "firecracker"
	defaultFirecrackerAssetDir        = "assets/firecracker"
	defaultBundledKernelImage         = "hello-vmlinux.bin"
	defaultBundledRootfsImage         = "hello-rootfs.ext4"
	defaultBundledPatchedRootfsImage  = "hello-rootfs-air.ext4"
	defaultBundledFirecracker         = "firecracker"
	defaultKVMDevice                  = "/dev/kvm"
	defaultVSockCIDBase        uint32 = 100
)

func ResolveConfig(root string) Config {
	cwd, _ := os.Getwd()

	return Config{
		Root:              root,
		Provider:          getenvDefault("AIR_VM_RUNTIME", "local"),
		FirecrackerBinary: resolveFirecrackerBinary(cwd),
		KernelImage:       resolveFirecrackerKernel(cwd),
		RootfsImage:       resolveFirecrackerRootfs(cwd),
		KVMDevice:         getenvDefault("AIR_KVM_DEVICE", defaultKVMDevice),
		VSockCIDBase:      defaultVSockCIDBase,
	}
}

func resolveFirecrackerBinary(cwd string) string {
	if value := os.Getenv("AIR_FIRECRACKER_BIN"); value != "" {
		return value
	}
	if bundled := bundledFirecrackerAsset(cwd, defaultBundledFirecracker); bundled != "" {
		return bundled
	}
	return defaultFirecrackerBinary
}

func resolveFirecrackerKernel(cwd string) string {
	if value := os.Getenv("AIR_FIRECRACKER_KERNEL"); value != "" {
		return value
	}
	return bundledFirecrackerAsset(cwd, defaultBundledKernelImage)
}

func resolveFirecrackerRootfs(cwd string) string {
	if value := os.Getenv("AIR_FIRECRACKER_ROOTFS"); value != "" {
		return value
	}
	if bundled := bundledFirecrackerAsset(cwd, defaultBundledPatchedRootfsImage); bundled != "" {
		return bundled
	}
	return bundledFirecrackerAsset(cwd, defaultBundledRootfsImage)
}

func bundledFirecrackerAsset(cwd, name string) string {
	if cwd == "" {
		return ""
	}

	path := filepath.Join(cwd, defaultFirecrackerAssetDir, name)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}
