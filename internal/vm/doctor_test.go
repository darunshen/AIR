package vm

import (
	"path/filepath"
	"testing"
)

func TestDiagnoseLocalRuntime(t *testing.T) {
	t.Helper()

	report := Diagnose(Config{
		Provider: "local",
		Root:     filepath.Join(t.TempDir(), "runtime"),
	})

	if !report.Ready {
		t.Fatalf("expected local doctor report to be ready: %+v", report)
	}
	if len(report.Checks) != 1 {
		t.Fatalf("expected 1 local check, got %d", len(report.Checks))
	}
	if report.Checks[0].Name != "shell_binary" {
		t.Fatalf("unexpected local check: %+v", report.Checks[0])
	}
}

func TestDiagnoseFirecrackerReportsMissingDependencies(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	report := Diagnose(Config{
		Provider:          "firecracker",
		Root:              filepath.Join(root, "runtime"),
		FirecrackerBinary: filepath.Join(root, "missing-firecracker"),
		KernelImage:       filepath.Join(root, "missing-vmlinux"),
		RootfsImage:       filepath.Join(root, "missing-rootfs"),
		KVMDevice:         filepath.Join(root, "missing-kvm"),
	})

	if report.Ready {
		t.Fatalf("expected firecracker doctor report to fail: %+v", report)
	}
	if len(report.Checks) != 4 {
		t.Fatalf("expected 4 firecracker checks, got %d", len(report.Checks))
	}
	for _, check := range report.Checks {
		if check.Status != doctorStatusFail {
			t.Fatalf("expected failing check, got %+v", check)
		}
		if check.Hint == "" {
			t.Fatalf("expected hint for failing check, got %+v", check)
		}
	}
}
