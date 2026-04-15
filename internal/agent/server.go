package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"time"

	"github.com/darunshen/AIR/internal/guestapi"
)

type Server struct {
	listener net.Listener
	execFn   func(context.Context, string) (*guestapi.ExecResult, error)
}

func NewServer(listener net.Listener) *Server {
	return &Server{
		listener: listener,
		execFn:   defaultExec,
	}
}

func (s *Server) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		_ = s.listener.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				continue
			}
			return err
		}

		go func() {
			if err := s.handleConn(ctx, conn); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()

		select {
		case err := <-errCh:
			return err
		default:
		}
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) error {
	defer conn.Close()
	return s.handleStream(ctx, conn)
}

func (s *Server) handleStream(ctx context.Context, rw io.ReadWriter) error {
	reader := bufio.NewReader(rw)
	decoder := json.NewDecoder(reader)
	encoder := json.NewEncoder(rw)

	var req guestapi.ExecRequest
	if err := decoder.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	result := &guestapi.ExecResult{
		Type:      guestapi.MessageTypeResult,
		RequestID: req.RequestID,
	}

	if req.Type != guestapi.MessageTypeExec {
		result.Error = fmt.Sprintf("unsupported request type: %s", req.Type)
		result.ExitCode = 1
		return encoder.Encode(result)
	}
	if req.Command == "" {
		result.Error = "command must not be empty"
		result.ExitCode = 1
		return encoder.Encode(result)
	}

	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	execResult, err := s.execFn(execCtx, req.Command)
	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
		return encoder.Encode(result)
	}

	execResult.Type = guestapi.MessageTypeResult
	execResult.RequestID = req.RequestID
	return encoder.Encode(execResult)
}

func defaultExec(ctx context.Context, command string) (*guestapi.ExecResult, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &guestapi.ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = 124
			if result.Stderr == "" {
				result.Stderr = "command timed out\n"
			}
		}
		return result, nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		if result.Stderr == "" {
			result.Stderr = "command timed out\n"
		}
		return result, nil
	}

	return nil, err
}
