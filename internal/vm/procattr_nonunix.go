//go:build !unix

package vm

import "os/exec"

func setDetachedProcessGroup(cmd *exec.Cmd) {}
