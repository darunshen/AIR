package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/darunshen/AIR/internal/guestapi"
)

func TestServerExecSuccess(t *testing.T) {
	t.Helper()

	server := &Server{execFn: defaultExec}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = server.handleStream(context.Background(), serverConn)
	}()

	var result guestapi.ExecResult
	roundTrip(t, clientConn, guestapi.ExecRequest{
		Type:      guestapi.MessageTypeExec,
		RequestID: "req_1",
		Command:   "echo hello",
		Timeout:   5,
	}, &result)
	if result.RequestID != "req_1" {
		t.Fatalf("unexpected request id: %s", result.RequestID)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", result.ExitCode)
	}
}

func TestServerExecFailure(t *testing.T) {
	t.Helper()

	server := &Server{execFn: defaultExec}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = server.handleStream(context.Background(), serverConn)
	}()

	var result guestapi.ExecResult
	roundTrip(t, clientConn, guestapi.ExecRequest{
		Type:      guestapi.MessageTypeExec,
		RequestID: "req_2",
		Command:   "sh -c 'echo boom >&2; exit 7'",
		Timeout:   5,
	}, &result)
	if result.ExitCode != 7 {
		t.Fatalf("unexpected exit code: %d", result.ExitCode)
	}
	if strings.TrimSpace(result.Stderr) != "boom" {
		t.Fatalf("unexpected stderr: %q", result.Stderr)
	}
}

func TestServerExecTimeout(t *testing.T) {
	t.Helper()

	server := &Server{execFn: defaultExec}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = server.handleStream(context.Background(), serverConn)
	}()

	var result guestapi.ExecResult
	roundTrip(t, clientConn, guestapi.ExecRequest{
		Type:      guestapi.MessageTypeExec,
		RequestID: "req_3",
		Command:   "sleep 1",
		Timeout:   1,
	}, &result)
	if result.ExitCode != 124 {
		t.Fatalf("expected timeout exit code 124, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "timed out") {
		t.Fatalf("expected timeout stderr, got %q", result.Stderr)
	}
}

func TestServerExecStreamingWorksWithoutPATH(t *testing.T) {
	t.Helper()

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", ""); err != nil {
		t.Fatalf("clear PATH: %v", err)
	}
	defer func() {
		_ = os.Setenv("PATH", originalPath)
	}()

	server := &Server{
		execFn:       defaultExec,
		execStreamFn: defaultExecStream,
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = server.handleStream(context.Background(), serverConn)
	}()

	if err := json.NewEncoder(clientConn).Encode(guestapi.ExecRequest{
		Type:      guestapi.MessageTypeExec,
		RequestID: "req_stream_pathless",
		Command:   "for i in 1 2 3; do echo tick-$i; sleep 0.01; done",
		Timeout:   5,
		Stream:    true,
	}); err != nil {
		t.Fatalf("encode exec request: %v", err)
	}

	decoder := json.NewDecoder(clientConn)
	var chunks []guestapi.ExecChunk
	var result guestapi.ExecResult
	for {
		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			t.Fatalf("decode stream response: %v", err)
		}
		msgType, _ := raw["type"].(string)
		switch msgType {
		case guestapi.MessageTypeChunk:
			body, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("marshal chunk: %v", err)
			}
			var chunk guestapi.ExecChunk
			if err := json.Unmarshal(body, &chunk); err != nil {
				t.Fatalf("unmarshal chunk: %v", err)
			}
			chunks = append(chunks, chunk)
		case guestapi.MessageTypeResult:
			body, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("marshal result: %v", err)
			}
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if result.ExitCode != 0 {
				t.Fatalf("unexpected exit code: %d stderr=%q", result.ExitCode, result.Stderr)
			}
			if !strings.Contains(result.Stdout, "tick-1") || !strings.Contains(result.Stdout, "tick-3") {
				t.Fatalf("unexpected stdout: %q", result.Stdout)
			}
			if len(chunks) == 0 {
				t.Fatal("expected streamed chunks")
			}
			return
		default:
			t.Fatalf("unexpected message type: %q", msgType)
		}
	}
}

func TestServerRejectsInvalidRequest(t *testing.T) {
	t.Helper()

	server := &Server{execFn: defaultExec}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = server.handleStream(context.Background(), serverConn)
	}()

	var result guestapi.ExecResult
	roundTrip(t, clientConn, guestapi.ExecRequest{
		Type:      "ping",
		RequestID: "req_4",
	}, &result)
	if result.Error == "" {
		t.Fatal("expected error for unsupported request type")
	}
}

func TestServerReady(t *testing.T) {
	t.Helper()

	server := &Server{execFn: defaultExec}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = server.handleStream(context.Background(), serverConn)
	}()

	req := guestapi.ExecRequest{
		Type:      guestapi.MessageTypeReady,
		RequestID: "ready_1",
	}
	if err := json.NewEncoder(clientConn).Encode(req); err != nil {
		t.Fatalf("encode ready request: %v", err)
	}

	var result guestapi.ReadyResult
	if err := json.NewDecoder(clientConn).Decode(&result); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if result.Type != guestapi.MessageTypeReady {
		t.Fatalf("unexpected response type: %s", result.Type)
	}
	if result.Status != "ready" {
		t.Fatalf("unexpected ready status: %s", result.Status)
	}
}

func TestServerPTY(t *testing.T) {
	t.Helper()

	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("pty test requires /bin/bash")
	}

	server := &Server{execFn: defaultExec}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = server.handleStream(context.Background(), serverConn)
	}()

	if err := json.NewEncoder(clientConn).Encode(guestapi.ExecRequest{
		Type:      guestapi.MessageTypePTY,
		RequestID: "pty_1",
		Command:   "printf ' ready'",
		Rows:      30,
		Cols:      100,
	}); err != nil {
		t.Fatalf("encode pty request: %v", err)
	}

	var result guestapi.PTYResult
	if err := json.NewDecoder(clientConn).Decode(&result); err != nil {
		t.Fatalf("decode pty response: %v", err)
	}
	if result.Type != guestapi.MessageTypePTY {
		t.Fatalf("unexpected response type: %s", result.Type)
	}
	if result.Status != "connected" {
		t.Fatalf("unexpected pty status: %+v", result)
	}

	deadline := time.Now().Add(2 * time.Second)
	var output strings.Builder
	buf := make([]byte, 16)
	for time.Now().Before(deadline) {
		_ = clientConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := clientConn.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
			if strings.Contains(output.String(), " ready") {
				return
			}
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			t.Fatalf("read pty output: %v", err)
		}
	}
	t.Fatalf("unexpected pty output: %q", output.String())
}

func TestServerPTYReturnsAfterCommandExit(t *testing.T) {
	t.Helper()

	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("pty test requires /bin/bash")
	}

	server := &Server{execFn: defaultExec}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer serverConn.Close()
		_ = server.handleStream(context.Background(), serverConn)
	}()

	if err := json.NewEncoder(clientConn).Encode(guestapi.ExecRequest{
		Type:      guestapi.MessageTypePTY,
		RequestID: "pty_exit",
		Command:   "printf 'done'",
		Rows:      24,
		Cols:      80,
	}); err != nil {
		t.Fatalf("encode pty request: %v", err)
	}

	var result guestapi.PTYResult
	if err := json.NewDecoder(clientConn).Decode(&result); err != nil {
		t.Fatalf("decode pty response: %v", err)
	}
	if result.Status != "connected" {
		t.Fatalf("unexpected pty status: %+v", result)
	}

	deadline := time.Now().Add(2 * time.Second)
	var output strings.Builder
	buf := make([]byte, 16)
	for time.Now().Before(deadline) {
		_ = clientConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := clientConn.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
			if strings.Contains(output.String(), "done") {
				break
			}
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			t.Fatalf("read pty output: %v", err)
		}
	}
	if !strings.Contains(output.String(), "done") {
		t.Fatalf("unexpected pty output: %q", output.String())
	}

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("pty stream did not exit after command completion")
	}
}

func TestResolveShellFallsBackToSh(t *testing.T) {
	t.Helper()

	shellPath, shellFlag := resolveShell()
	if shellPath == "" || shellFlag == "" {
		t.Fatalf("expected shell path and flag, got %q %q", shellPath, shellFlag)
	}
	if err := exec.Command(shellPath, shellFlag, ":").Run(); err != nil {
		t.Fatalf("resolved shell does not work: %v", err)
	}
}

func TestServerProxy(t *testing.T) {
	t.Helper()

	targetClient, targetServer := net.Pipe()
	defer targetClient.Close()
	defer targetServer.Close()

	server := &Server{
		execFn: defaultExec,
		dialFn: func(network, address string) (net.Conn, error) {
			if network != "tcp" {
				t.Fatalf("unexpected network: %s", network)
			}
			if address != "127.0.0.1:50051" {
				t.Fatalf("unexpected address: %s", address)
			}
			return targetClient, nil
		},
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = server.handleStream(context.Background(), serverConn)
	}()

	if err := json.NewEncoder(clientConn).Encode(guestapi.ExecRequest{
		Type:      guestapi.MessageTypeProxy,
		RequestID: "proxy_1",
		Network:   "tcp",
		Address:   "127.0.0.1:50051",
	}); err != nil {
		t.Fatalf("encode proxy request: %v", err)
	}

	var result guestapi.ProxyResult
	if err := json.NewDecoder(clientConn).Decode(&result); err != nil {
		t.Fatalf("decode proxy response: %v", err)
	}
	if result.Status != "connected" {
		t.Fatalf("unexpected proxy status: %+v", result)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 5)
		if _, err := io.ReadFull(targetServer, buf); err != nil {
			t.Errorf("read proxy payload at target: %v", err)
			return
		}
		if string(buf) != "hello" {
			t.Errorf("unexpected proxy payload: %q", string(buf))
			return
		}
		if _, err := targetServer.Write([]byte("world")); err != nil {
			t.Errorf("write proxy response at target: %v", err)
		}
	}()

	if _, err := clientConn.Write([]byte("hello")); err != nil {
		t.Fatalf("write client payload: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(clientConn, buf); err != nil {
		t.Fatalf("read proxied response: %v", err)
	}
	if string(buf) != "world" {
		t.Fatalf("unexpected proxied response: %q", string(buf))
	}
	_ = clientConn.Close()
	<-done
}

func TestServerContinuesAfterConnectionError(t *testing.T) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	server := NewServer(listener)
	server.execFn = func(ctx context.Context, command string) (*guestapi.ExecResult, error) {
		return &guestapi.ExecResult{Stdout: "ok\n"}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx)
	}()

	conn1, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial first conn: %v", err)
	}
	_ = conn1.Close()

	time.Sleep(100 * time.Millisecond)

	conn2, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial second conn: %v", err)
	}
	defer conn2.Close()

	var result guestapi.ExecResult
	roundTrip(t, conn2, guestapi.ExecRequest{
		Type:      guestapi.MessageTypeExec,
		RequestID: "req_after_error",
		Command:   "echo ok",
		Timeout:   5,
	}, &result)
	if strings.TrimSpace(result.Stdout) != "ok" {
		t.Fatalf("unexpected stdout after prior connection error: %q", result.Stdout)
	}

	cancel()
	select {
	case serveErr := <-done:
		if serveErr != nil && !errors.Is(serveErr, net.ErrClosed) {
			t.Fatalf("unexpected serve error: %v", serveErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after cancel")
	}
}

func roundTrip(t *testing.T, conn net.Conn, req guestapi.ExecRequest, result *guestapi.ExecResult) {
	t.Helper()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	if err := json.NewDecoder(conn).Decode(result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
}
