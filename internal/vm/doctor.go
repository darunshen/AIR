package vm

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	doctorStatusOK   = "ok"
	doctorStatusFail = "fail"
)

type DoctorReport struct {
	Provider       string               `json:"provider"`
	Ready          bool                 `json:"ready"`
	RuntimeRoot    string               `json:"runtime_root,omitempty"`
	ResolvedConfig DoctorResolvedConfig `json:"resolved_config,omitempty"`
	Checks         []DoctorCheck        `json:"checks"`
}

type DoctorResolvedConfig struct {
	FirecrackerBinary string `json:"firecracker_binary,omitempty"`
	KernelImage       string `json:"kernel_image,omitempty"`
	RootfsImage       string `json:"rootfs_image,omitempty"`
	KVMDevice         string `json:"kvm_device,omitempty"`
}

type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func Diagnose(cfg Config) *DoctorReport {
	provider := cfg.Provider
	if provider == "" {
		provider = "local"
	}

	report := &DoctorReport{
		Provider:    provider,
		RuntimeRoot: cfg.Root,
	}

	switch provider {
	case "local":
		report.Checks = diagnoseLocal()
	case "firecracker":
		report.ResolvedConfig = DoctorResolvedConfig{
			FirecrackerBinary: cfg.FirecrackerBinary,
			KernelImage:       cfg.KernelImage,
			RootfsImage:       cfg.RootfsImage,
			KVMDevice:         cfg.KVMDevice,
		}
		report.Checks = diagnoseFirecracker(cfg)
	default:
		report.Checks = []DoctorCheck{{
			Name:    "provider",
			Status:  doctorStatusFail,
			Value:   provider,
			Message: fmt.Sprintf("unsupported vm provider: %s", provider),
			Hint:    "use `local` or `firecracker`",
		}}
	}

	report.Ready = true
	for _, check := range report.Checks {
		if check.Status != doctorStatusOK {
			report.Ready = false
			break
		}
	}

	return report
}

func diagnoseLocal() []DoctorCheck {
	path, err := exec.LookPath("sh")
	if err != nil {
		return []DoctorCheck{{
			Name:    "shell_binary",
			Status:  doctorStatusFail,
			Value:   "sh",
			Message: fmt.Sprintf("local provider shell is unavailable: %v", err),
			Hint:    "install a POSIX shell or ensure `sh` is in PATH",
		}}
	}

	return []DoctorCheck{{
		Name:    "shell_binary",
		Status:  doctorStatusOK,
		Value:   path,
		Message: "local provider can execute commands through `sh`",
	}}
}

func diagnoseFirecracker(cfg Config) []DoctorCheck {
	return []DoctorCheck{
		checkFirecrackerBinary(cfg.FirecrackerBinary),
		checkKernelImage(cfg.KernelImage),
		checkRootfsImage(cfg.RootfsImage),
		checkKVMDevice(cfg.KVMDevice),
	}
}

func checkFirecrackerBinary(binary string) DoctorCheck {
	path, err := exec.LookPath(binary)
	if err != nil {
		return DoctorCheck{
			Name:    "firecracker_binary",
			Status:  doctorStatusFail,
			Value:   binary,
			Message: fmt.Sprintf("firecracker binary is unavailable: %v", err),
			Hint:    "install the official Firecracker release into PATH or set `AIR_FIRECRACKER_BIN`",
		}
	}

	return DoctorCheck{
		Name:    "firecracker_binary",
		Status:  doctorStatusOK,
		Value:   path,
		Message: "firecracker binary is executable",
	}
}

func checkKernelImage(path string) DoctorCheck {
	if path == "" {
		return DoctorCheck{
			Name:    "kernel_image",
			Status:  doctorStatusFail,
			Message: "kernel image is not configured",
			Hint:    "set `AIR_FIRECRACKER_KERNEL` or place `hello-vmlinux.bin` under `assets/firecracker/` or `/usr/lib/air/firecracker/`",
		}
	}
	if _, err := os.Stat(path); err != nil {
		return DoctorCheck{
			Name:    "kernel_image",
			Status:  doctorStatusFail,
			Value:   path,
			Message: fmt.Sprintf("kernel image is unavailable: %v", err),
			Hint:    "point `AIR_FIRECRACKER_KERNEL` to a bootable `vmlinux` file",
		}
	}

	return DoctorCheck{
		Name:    "kernel_image",
		Status:  doctorStatusOK,
		Value:   path,
		Message: "kernel image is present",
	}
}

func checkRootfsImage(path string) DoctorCheck {
	if path == "" {
		return DoctorCheck{
			Name:    "rootfs_image",
			Status:  doctorStatusFail,
			Message: "rootfs image is not configured",
			Hint:    "set `AIR_FIRECRACKER_ROOTFS` or place `hello-rootfs-air.ext4` or `hello-rootfs.ext4` under `assets/firecracker/` or `/usr/lib/air/firecracker/`",
		}
	}
	if _, err := os.Stat(path); err != nil {
		return DoctorCheck{
			Name:    "rootfs_image",
			Status:  doctorStatusFail,
			Value:   path,
			Message: fmt.Sprintf("rootfs image is unavailable: %v", err),
			Hint:    "point `AIR_FIRECRACKER_ROOTFS` to an ext4 rootfs image",
		}
	}

	return DoctorCheck{
		Name:    "rootfs_image",
		Status:  doctorStatusOK,
		Value:   path,
		Message: "rootfs image is present",
	}
}

func checkKVMDevice(path string) DoctorCheck {
	if path == "" {
		path = defaultKVMDevice
	}

	if _, err := os.Stat(path); err != nil {
		return DoctorCheck{
			Name:    "kvm_device",
			Status:  doctorStatusFail,
			Value:   path,
			Message: fmt.Sprintf("KVM device is unavailable: %v", err),
			Hint:    "enable KVM in the host and ensure `/dev/kvm` exists",
		}
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return DoctorCheck{
			Name:    "kvm_device",
			Status:  doctorStatusFail,
			Value:   path,
			Message: fmt.Sprintf("KVM device is not readable and writable: %v", err),
			Hint:    "grant the current user read/write access to `/dev/kvm`",
		}
	}
	_ = file.Close()

	return DoctorCheck{
		Name:    "kvm_device",
		Status:  doctorStatusOK,
		Value:   path,
		Message: "KVM device is readable and writable",
	}
}
