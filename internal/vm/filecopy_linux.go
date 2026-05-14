//go:build linux

package vm

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func tryCloneFile(target *os.File, source *os.File) bool {
	return unix.IoctlFileClone(int(target.Fd()), int(source.Fd())) == nil
}

func copyFileSparse(target *os.File, source *os.File, size int64) error {
	if size == 0 {
		return nil
	}
	offset := int64(0)
	for offset < size {
		dataOffset, err := unix.Seek(int(source.Fd()), offset, unix.SEEK_DATA)
		if err != nil {
			if errors.Is(err, unix.ENXIO) {
				return target.Truncate(size)
			}
			return err
		}
		if dataOffset > offset {
			if err := target.Truncate(dataOffset); err != nil {
				return err
			}
		}
		holeOffset, err := unix.Seek(int(source.Fd()), dataOffset, unix.SEEK_HOLE)
		if err != nil {
			if errors.Is(err, unix.ENXIO) {
				holeOffset = size
			} else {
				return err
			}
		}
		if holeOffset > size {
			holeOffset = size
		}
		if _, err := source.Seek(dataOffset, io.SeekStart); err != nil {
			return err
		}
		if _, err := target.Seek(dataOffset, io.SeekStart); err != nil {
			return err
		}
		if _, err := io.CopyN(target, source, holeOffset-dataOffset); err != nil {
			return err
		}
		offset = holeOffset
	}
	return target.Truncate(size)
}
