package vm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Runtime struct {
	root string
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func New(root string) (*Runtime, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Runtime{root: root}, nil
}

func (r *Runtime) Start(sessionID string) (string, error) {
	base := filepath.Join(r.root, sessionID)
	workspace := filepath.Join(base, "workspace")
	taskDir := filepath.Join(base, "task")

	for _, dir := range []string{workspace, taskDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}

	return sessionID, nil
}

func (r *Runtime) Exec(sessionID, command string, timeout time.Duration) (*ExecResult, error) {
	base := filepath.Join(r.root, sessionID)
	workspace := filepath.Join(base, "workspace")
	taskDir := filepath.Join(base, "task")
	cmdPath := filepath.Join(taskDir, "cmd.sh")
	resultPath := filepath.Join(taskDir, "result.txt")
	stderrPath := filepath.Join(taskDir, "stderr.txt")

	if err := os.WriteFile(cmdPath, []byte(command+"\n"), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(resultPath)
	_ = os.Remove(stderrPath)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workspace
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if err := os.WriteFile(resultPath, stdout.Bytes(), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(stderrPath, stderr.Bytes(), 0o644); err != nil {
		return nil, err
	}

	exitCode := 0
	timedOut := ctx.Err() == context.DeadlineExceeded
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			if timedOut {
				exitCode = 124
			} else {
				exitCode = exitErr.ExitCode()
			}
		} else {
			if timedOut {
				if stderr.Len() == 0 {
					stderr.WriteString("command timed out\n")
				}
				exitCode = 124
			} else {
				return nil, runErr
			}
		}
	}
	if timedOut && stderr.Len() == 0 {
		stderr.WriteString("command timed out\n")
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

func (r *Runtime) Stop(vmid string) error {
	base := filepath.Join(r.root, vmid)
	if _, err := os.Stat(base); err != nil {
		return fmt.Errorf("runtime not found: %w", err)
	}
	return os.RemoveAll(base)
}
