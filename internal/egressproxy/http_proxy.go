package egressproxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func ServeUnixHTTPProxy(ctx context.Context, socketPath string) error {
	if err := os.RemoveAll(socketPath); err != nil {
		return err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()

	server := &http.Server{
		Handler:           http.HandlerFunc(handleProxyHTTP),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	go func() {
		errCh <- server.Serve(listener)
	}()

	err = <-errCh
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return err
}

func handleProxyHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect {
		handleConnect(w, req)
		return
	}
	handleForward(w, req)
}

func handleConnect(w http.ResponseWriter, req *http.Request) {
	targetConn, err := net.DialTimeout("tcp", req.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		targetConn.Close()
		http.Error(w, "proxy hijacking is not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		targetConn.Close()
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	bridgeConns(clientConn, targetConn)
}

func handleForward(w http.ResponseWriter, req *http.Request) {
	outReq := req.Clone(req.Context())
	outReq.RequestURI = ""
	outReq.URL.Scheme = normalizeScheme(outReq.URL.Scheme)
	if outReq.URL.Host == "" {
		outReq.URL.Host = req.Host
	}

	resp, err := http.DefaultTransport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func normalizeScheme(scheme string) string {
	switch strings.ToLower(scheme) {
	case "", "http", "https":
		if scheme == "" {
			return "http"
		}
		return strings.ToLower(scheme)
	default:
		return scheme
	}
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func bridgeConns(left, right net.Conn) {
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(left, right)
		done <- err
	}()
	go func() {
		_, err := io.Copy(right, left)
		done <- err
	}()
	<-done
	_ = left.Close()
	_ = right.Close()
	<-done
}

func DefaultGuestHTTPProxyURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", DefaultGuestHTTPProxyPort)
}

const DefaultGuestHTTPProxyPort uint32 = 18080
