package agent

import (
	"context"
	"encoding/json"
	"io"
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

func roundTrip(t *testing.T, conn net.Conn, req guestapi.ExecRequest, result *guestapi.ExecResult) {
	t.Helper()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	if err := json.NewDecoder(conn).Decode(result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
}
