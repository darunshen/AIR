package vm

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const defaultWorkspaceUpperSize = 512 * 1024 * 1024

func defaultWorkspaceExcludes() map[string]struct{} {
	return map[string]struct{}{
		".air":         {},
		".git":         {},
		".hg":          {},
		".svn":         {},
		"node_modules": {},
		"target":       {},
		"dist":         {},
		"build":        {},
		".cache":       {},
	}
}

func buildWorkspaceImage(outputPath, sourcePath string) error {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("workspace path must be a directory: %s", sourcePath)
	}
	if _, err := exec.LookPath("mkfs.ext4"); err != nil {
		return fmt.Errorf("mkfs.ext4 is required to build workspace image: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "air-workspace-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	stageRoot := filepath.Join(tmpDir, "workspace")
	if err := copyDir(stageRoot, sourcePath, defaultWorkspaceExcludes()); err != nil {
		return err
	}

	size, inodeCount, err := dirUsage(stageRoot)
	if err != nil {
		return err
	}
	targetSize := alignUp(size+256*1024*1024, 64*1024*1024)
	inodeTarget := inodeCount + inodeCount/2 + 16384
	if inodeTarget < 32768 {
		inodeTarget = 32768
	}

	return createExt4FromDir(outputPath, stageRoot, targetSize, inodeTarget)
}

func createEmptyExt4(outputPath string, size int64, inodeCount int64) error {
	if _, err := exec.LookPath("mkfs.ext4"); err != nil {
		return fmt.Errorf("mkfs.ext4 is required to create ext4 image: %w", err)
	}
	if inodeCount <= 0 {
		inodeCount = 32768
	}
	if err := os.RemoveAll(outputPath); err != nil {
		return err
	}
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if err := file.Truncate(size); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	cmd := exec.Command("mkfs.ext4", "-q", "-F", "-N", fmt.Sprintf("%d", inodeCount), outputPath)
	return cmd.Run()
}

func createExt4FromDir(outputPath, sourceDir string, size, inodeCount int64) error {
	if err := os.RemoveAll(outputPath); err != nil {
		return err
	}
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if err := file.Truncate(size); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	cmd := exec.Command("mkfs.ext4", "-q", "-F", "-N", fmt.Sprintf("%d", inodeCount), "-d", sourceDir, outputPath)
	return cmd.Run()
}

func copyDir(dst, src string, excludes map[string]struct{}) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == src {
			return nil
		}
		name := entry.Name()
		if _, ok := excludes[name]; ok {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		switch {
		case mode.Type()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		case entry.IsDir():
			return os.MkdirAll(target, mode.Perm())
		case mode.IsRegular():
			return copyRegularFile(target, path, mode.Perm())
		default:
			return nil
		}
	})
}

func copyRegularFile(dst, src string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer target.Close()
	_, err = io.Copy(target, source)
	return err
}

func dirUsage(root string) (int64, int64, error) {
	var size int64
	var count int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		count++
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	return size, count, err
}

func alignUp(value, align int64) int64 {
	if align <= 0 {
		return value
	}
	return ((value + align - 1) / align) * align
}
