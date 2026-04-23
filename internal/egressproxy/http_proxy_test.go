package egressproxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServeUnixHTTPProxyForHTTP(t *testing.T) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	socketPath := filepath.Join(t.TempDir(), "proxy.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeUnixHTTPProxy(ctx, socketPath)
	}()
	waitForSocket(t, socketPath)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(mustParseProxyURL(t)),
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("proxy get: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("unexpected body: %q", string(body))
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("proxy returned error: %v", err)
	}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for socket: %s", path)
}

func mustParseProxyURL(t *testing.T) *url.URL {
	t.Helper()
	parsed, err := url.Parse("http://air-proxy")
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	return parsed
}
