//go:build !unix

package vm

import "errors"

func syscallKill(pid int, sig int) error {
	return errors.New("process signals are not supported on this platform")
}

func signalIndicatesProcessExists(pid int) bool {
	_ = pid
	return false
}
