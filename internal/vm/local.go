package vm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type localRuntime struct {
	root string
}

func newLocalRuntime(cfg Config) (Runtime, error) {
	return &localRuntime{root: filepath.Join(cfg.Root, "local")}, nil
}

func (r *localRuntime) Start(sessionID string) (string, error) {
	base := sessionRoot(r.root, sessionID)
	workspace := filepath.Join(base, "workspace")
	taskDir := filepath.Join(base, "task")
	eventsPath := filepath.Join(base, "events.jsonl")

	for _, dir := range []string{workspace, taskDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	_ = appendRuntimeEvent(eventsPath, "local", sessionID, "session_started", map[string]any{
		"workspace_path": workspace,
		"task_path":      taskDir,
	})

	return sessionID, nil
}

func (r *localRuntime) Exec(sessionID, command string, timeout time.Duration) (*ExecResult, error) {
	base := sessionRoot(r.root, sessionID)
	workspace := filepath.Join(base, "workspace")
	taskDir := filepath.Join(base, "task")
	eventsPath := filepath.Join(base, "events.jsonl")
	cmdPath := filepath.Join(taskDir, "cmd.sh")
	resultPath := filepath.Join(taskDir, "result.txt")
	stderrPath := filepath.Join(taskDir, "stderr.txt")
	requestID := fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
	startedAt := time.Now()

	if err := os.WriteFile(cmdPath, []byte(command+"\n"), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(resultPath)
	_ = os.Remove(stderrPath)
	_ = appendRuntimeEvent(eventsPath, "local", sessionID, "exec_started", map[string]any{
		"request_id": requestID,
		"command":    command,
		"timeout_ms": timeout.Milliseconds(),
	})

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

	duration := time.Since(startedAt)
	_ = appendRuntimeEvent(eventsPath, "local", sessionID, "exec_completed", map[string]any{
		"request_id":  requestID,
		"command":     command,
		"duration_ms": duration.Milliseconds(),
		"exit_code":   exitCode,
	})

	return &ExecResult{
		RequestID: requestID,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode,
		TimedOut:  timedOut,
		Duration:  duration,
	}, nil
}

func (r *localRuntime) Stop(vmid string) error {
	base := sessionRoot(r.root, vmid)
	if _, err := os.Stat(base); err != nil {
		return fmt.Errorf("runtime not found: %w", err)
	}
	_ = appendRuntimeEvent(filepath.Join(base, "events.jsonl"), "local", vmid, "session_stopped", nil)
	return os.RemoveAll(base)
}

func (r *localRuntime) Inspect(sessionID string) (*InspectInfo, error) {
	base := sessionRoot(r.root, sessionID)
	workspace := filepath.Join(base, "workspace")
	taskDir := filepath.Join(base, "task")

	_, err := os.Stat(base)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return &InspectInfo{
		Provider:      "local",
		SessionID:     sessionID,
		RootPath:      base,
		Exists:        exists,
		Running:       exists,
		WorkspacePath: workspace,
		TaskPath:      taskDir,
		EventsPath:    filepath.Join(base, "events.jsonl"),
	}, nil
}

func (r *localRuntime) DialTCP(sessionID, address string, timeout time.Duration) (net.Conn, error) {
	_, err := r.Inspect(sessionID)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return net.DialTimeout("tcp", address, timeout)
}
