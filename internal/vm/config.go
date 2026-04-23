package vm

import (
	"os"
	"path/filepath"
)

const (
	defaultRuntimeRoot                      = "runtime/sessions"
	defaultFirecrackerBinary                = "firecracker"
	defaultFirecrackerAssetDir              = "assets/firecracker"
	defaultInstalledFirecrackerDir          = "/usr/lib/air/firecracker"
	defaultLocalFirecrackerDir              = "/usr/local/lib/air/firecracker"
	defaultBundledKernelImage               = "hello-vmlinux.bin"
	defaultBundledRootfsImage               = "hello-rootfs.ext4"
	defaultBundledPatchedRootfsImage        = "hello-rootfs-air.ext4"
	defaultBundledFirecracker               = "firecracker"
	defaultFirecrackerBootArgs              = "console=ttyS0 reboot=k panic=1 pci=off"
	defaultKVMDevice                        = "/dev/kvm"
	defaultFirecrackerMemoryMiB             = 256
	defaultFirecrackerVCPUCount             = 1
	defaultVSockCIDBase              uint32 = 100
)

func ResolveConfig(root string) Config {
	cwd, _ := os.Getwd()

	return Config{
		Root:              root,
		Provider:          getenvDefault("AIR_VM_RUNTIME", "local"),
		FirecrackerBinary: resolveFirecrackerBinary(cwd),
		KernelImage:       resolveFirecrackerKernel(cwd),
		RootfsImage:       resolveFirecrackerRootfs(cwd),
		BootArgs:          getenvDefault("AIR_FIRECRACKER_BOOT_ARGS", defaultFirecrackerBootArgs),
		KVMDevice:         getenvDefault("AIR_KVM_DEVICE", defaultKVMDevice),
		MemoryMiB:         defaultFirecrackerMemoryMiB,
		VCPUCount:         defaultFirecrackerVCPUCount,
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
	for _, dir := range firecrackerAssetDirs(cwd) {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func firecrackerAssetDirs(cwd string) []string {
	candidates := make([]string, 0, 4)
	seen := map[string]struct{}{}
	appendCandidate := func(path string) {
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}

	if cwd != "" {
		appendCandidate(filepath.Join(cwd, defaultFirecrackerAssetDir))
	}
	if exe, err := os.Executable(); err == nil {
		appendCandidate(filepath.Join(filepath.Dir(exe), "..", "lib", "air", "firecracker"))
	}
	appendCandidate(defaultInstalledFirecrackerDir)
	appendCandidate(defaultLocalFirecrackerDir)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		appendCandidate(filepath.Join(home, ".local", "share", "air", "firecracker"))
	}

	return candidates
}
