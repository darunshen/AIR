//go:build !linux

package vm

import (
	"errors"
	"os"
)

func tryCloneFile(target *os.File, source *os.File) bool {
	return false
}

func copyFileSparse(target *os.File, source *os.File, size int64) error {
	return errors.New("sparse copy is unsupported on this platform")
}
