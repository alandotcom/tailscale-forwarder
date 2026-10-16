package main

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func tcpPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	client, err := net.DialTCP("tcp", nil, ln.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	server, err := ln.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	client.SetDeadline(time.Now().Add(3 * time.Second))
	server.SetDeadline(time.Now().Add(3 * time.Second))
	return client, server
}

func TestFwdTCPPreservesHalfClose(t *testing.T) {
	for _, closeFirst := range []string{"client", "target"} {
		t.Run(closeFirst, func(t *testing.T) {
			target, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			host, portString, _ := net.SplitHostPort(target.Addr().String())
			port, _ := strconv.Atoi(portString)
			client, source := tcpPair(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- fwdTCP(ctx, source, host, port) }()
			backend, err := target.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer backend.Close()
			backend.SetDeadline(time.Now().Add(3 * time.Second))
			first, second := client, backend.(*net.TCPConn)
			if closeFirst == "target" {
				first, second = second, first
			}
			if err := first.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			if data, err := io.ReadAll(second); err != nil || len(data) != 0 {
				t.Fatalf("read half-close: %q, %v", data, err)
			}
			if _, err := second.Write([]byte("response after EOF")); err != nil {
				t.Fatal(err)
			}
			if err := second.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			if data, err := io.ReadAll(first); err != nil || string(data) != "response after EOF" {
				t.Fatalf("response lost after half-close: %q, %v", data, err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("forward did not finish")
			}
		})
	}
}
