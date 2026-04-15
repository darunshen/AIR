package agent

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

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

func roundTrip(t *testing.T, conn net.Conn, req guestapi.ExecRequest, result *guestapi.ExecResult) {
	t.Helper()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	if err := json.NewDecoder(conn).Decode(result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
}
