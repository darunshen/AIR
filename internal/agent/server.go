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
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

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
			// A single client/proxy stream must not take down the entire guest agent.
			_ = s.handleConn(ctx, conn)
		}()
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
		streamReader := streamAfterJSONDecode(decoder, reader)
		return s.handleProxyStream(streamReader, rw, encoder, req)
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

func (s *Server) handleProxyStream(reader io.Reader, rw io.ReadWriter, encoder *json.Encoder, req guestapi.ExecRequest) error {
	network := req.Network
	if network == "" {
		network = "tcp"
	}
	if proxyTransportDebugEnabled() {
		fmt.Fprintf(os.Stderr, "air-agent: proxy start request_id=%s network=%s address=%s\n", req.RequestID, network, req.Address)
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
		if proxyTransportDebugEnabled() {
			fmt.Fprintf(os.Stderr, "air-agent: proxy dial failed request_id=%s address=%s err=%v\n", req.RequestID, req.Address, err)
		}
		return encoder.Encode(guestapi.ProxyResult{
			Type:      guestapi.MessageTypeProxy,
			RequestID: req.RequestID,
			Status:    "error",
			Error:     err.Error(),
		})
	}
	defer targetConn.Close()
	if proxyTransportDebugEnabled() {
		fmt.Fprintf(os.Stderr, "air-agent: proxy dial connected request_id=%s address=%s\n", req.RequestID, req.Address)
		reader = &loggedReader{Reader: reader, requestID: req.RequestID, name: "proxy-client"}
		targetConn = &loggedNetConn{Conn: targetConn, requestID: req.RequestID, name: "proxy-target"}
		if conn, ok := rw.(net.Conn); ok {
			rw = &loggedNetConn{Conn: conn, requestID: req.RequestID, name: "proxy-vsock"}
		}
	}

	if err := encoder.Encode(guestapi.ProxyResult{
		Type:      guestapi.MessageTypeProxy,
		RequestID: req.RequestID,
		Status:    "connected",
	}); err != nil {
		if proxyTransportDebugEnabled() {
			fmt.Fprintf(os.Stderr, "air-agent: proxy encode connected failed request_id=%s err=%v\n", req.RequestID, err)
		}
		return err
	}

	type proxyCopyResult struct {
		direction string
		bytes     int64
		err       error
	}
	copyErrCh := make(chan proxyCopyResult, 2)
	go func() {
		n, err := io.Copy(targetConn, reader)
		if tcpConn, ok := targetConn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
		copyErrCh <- proxyCopyResult{direction: "client->target", bytes: n, err: err}
	}()
	go func() {
		n, err := io.Copy(rw, targetConn)
		copyErrCh <- proxyCopyResult{direction: "target->client", bytes: n, err: err}
	}()

	for i := 0; i < 2; i++ {
		result := <-copyErrCh
		if result.err != nil && !errors.Is(result.err, net.ErrClosed) && !errors.Is(result.err, io.EOF) {
			if proxyTransportDebugEnabled() {
				fmt.Fprintf(os.Stderr, "air-agent: proxy copy request_id=%s direction=%s bytes=%d err=%v\n", req.RequestID, result.direction, result.bytes, result.err)
			}
			return result.err
		}
		if proxyTransportDebugEnabled() {
			fmt.Fprintf(os.Stderr, "air-agent: proxy copy request_id=%s direction=%s bytes=%d closed\n", req.RequestID, result.direction, result.bytes)
		}
	}
	if proxyTransportDebugEnabled() {
		fmt.Fprintf(os.Stderr, "air-agent: proxy done request_id=%s\n", req.RequestID)
	}
	return nil
}

type loggedReader struct {
	io.Reader
	requestID string
	name      string
}

func (r *loggedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 || err != nil {
		fmt.Fprintf(os.Stderr, "air-agent: io request_id=%s name=%s op=read bytes=%d err=%v\n", r.requestID, r.name, n, err)
	}
	return n, err
}

type loggedNetConn struct {
	net.Conn
	requestID string
	name      string
}

func (c *loggedNetConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 || err != nil {
		fmt.Fprintf(os.Stderr, "air-agent: io request_id=%s name=%s op=read bytes=%d err=%v\n", c.requestID, c.name, n, err)
	}
	return n, err
}

func (c *loggedNetConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 || err != nil {
		fmt.Fprintf(os.Stderr, "air-agent: io request_id=%s name=%s op=write bytes=%d err=%v\n", c.requestID, c.name, n, err)
	}
	return n, err
}

func proxyTransportDebugEnabled() bool {
	return strings.TrimSpace(os.Getenv("AIR_DEBUG_TRANSPORT")) == "1"
}

func streamAfterJSONDecode(decoder *json.Decoder, reader io.Reader) io.Reader {
	streamReader := reader
	if buffered := decoder.Buffered(); buffered != nil {
		streamReader = io.MultiReader(buffered, reader)
	}
	return &leadingWhitespaceTrimmingReader{
		reader:   streamReader,
		trimming: true,
	}
}

type leadingWhitespaceTrimmingReader struct {
	reader   io.Reader
	trimming bool
}

func (r *leadingWhitespaceTrimmingReader) Read(p []byte) (int, error) {
	for {
		n, err := r.reader.Read(p)
		if !r.trimming || n == 0 {
			return n, err
		}
		i := 0
		for i < n && unicode.IsSpace(rune(p[i])) {
			i++
		}
		if i == 0 {
			r.trimming = false
			return n, err
		}
		if i < n {
			copy(p, p[i:n])
			r.trimming = false
			return n - i, err
		}
		if err != nil {
			return 0, err
		}
	}
}

func defaultExec(ctx context.Context, command string) (*guestapi.ExecResult, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	if info, err := os.Stat("/workspace"); err == nil && info.IsDir() {
		cmd.Dir = "/workspace"
	}
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
