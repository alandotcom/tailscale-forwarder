package main

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

// startEchoTarget starts a loopback TCP server that echoes one connection, and
// returns its host and port.
func startEchoTarget(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi port: %v", err)
	}
	return host, port
}

func TestFwdTCPForwardsBidirectionally(t *testing.T) {
	host, port := startEchoTarget(t)

	client, server := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- fwdTCP(ctx, server, host, port) }()

	msg := []byte("hello world")
	go func() {
		_ = client.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, _ = client.Write(msg)
	}()

	buf := make([]byte, len(msg))
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != string(msg) {
		t.Fatalf("got %q, want %q", buf, msg)
	}

	// Closing the source must cause fwdTCP to return promptly rather than
	// parking until context cancellation (the leak this guards against).
	client.Close()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("fwdTCP returned error on normal close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fwdTCP did not return after the connection closed (goroutine leak)")
	}
}

func TestFwdTCPReturnsOnContextCancel(t *testing.T) {
	host, port := startEchoTarget(t)

	client, server := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- fwdTCP(ctx, server, host, port) }()

	// Drive one round-trip so the dial has completed and forwarding is active.
	go func() {
		_ = client.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, _ = client.Write([]byte("x"))
	}()
	buf := make([]byte, 1)
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}

	// Cancelling an established forward should unblock and return without error.
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("fwdTCP returned error on cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fwdTCP did not return after context cancellation")
	}
	client.Close()
}

func TestFwdTCPDialFailureClassified(t *testing.T) {
	_, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Port 1 on loopback should refuse quickly.
	err := fwdTCP(ctx, server, "127.0.0.1", 1)
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
}
