//go:build unix

package vm

import "syscall"

func syscallKill(pid int, sig int) error {
	return syscall.Kill(pid, syscall.Signal(sig))
}

func signalIndicatesProcessExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
