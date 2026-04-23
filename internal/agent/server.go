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
	dialFn   func(string, string) (net.Conn, error)
}

func NewServer(listener net.Listener) *Server {
	return &Server{
		listener: listener,
		execFn:   defaultExec,
		dialFn:   net.Dial,
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

	if req.Type == guestapi.MessageTypeReady {
		return encoder.Encode(guestapi.ReadyResult{
			Type:      guestapi.MessageTypeReady,
			RequestID: req.RequestID,
			Status:    "ready",
		})
	}
	if req.Type == guestapi.MessageTypeProxy {
		return s.handleProxyStream(rw, encoder, req)
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

func (s *Server) handleProxyStream(rw io.ReadWriter, encoder *json.Encoder, req guestapi.ExecRequest) error {
	network := req.Network
	if network == "" {
		network = "tcp"
	}
	if req.Address == "" {
		return encoder.Encode(guestapi.ProxyResult{
			Type:      guestapi.MessageTypeProxy,
			RequestID: req.RequestID,
			Status:    "error",
			Error:     "proxy address must not be empty",
		})
	}

	dialFn := s.dialFn
	if dialFn == nil {
		dialFn = net.Dial
	}

	targetConn, err := dialFn(network, req.Address)
	if err != nil {
		return encoder.Encode(guestapi.ProxyResult{
			Type:      guestapi.MessageTypeProxy,
			RequestID: req.RequestID,
			Status:    "error",
			Error:     err.Error(),
		})
	}
	defer targetConn.Close()

	if err := encoder.Encode(guestapi.ProxyResult{
		Type:      guestapi.MessageTypeProxy,
		RequestID: req.RequestID,
		Status:    "connected",
	}); err != nil {
		return err
	}

	copyErrCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(targetConn, rw)
		if tcpConn, ok := targetConn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
		copyErrCh <- err
	}()
	go func() {
		_, err := io.Copy(rw, targetConn)
		copyErrCh <- err
	}()

	for i := 0; i < 2; i++ {
		err := <-copyErrCh
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
			return err
		}
	}
	return nil
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
			result.TimedOut = true
			if result.Stderr == "" {
				result.Stderr = "command timed out\n"
			}
		}
		return result, nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.TimedOut = true
		if result.Stderr == "" {
			result.Stderr = "command timed out\n"
		}
		return result, nil
	}

	return nil, err
}
