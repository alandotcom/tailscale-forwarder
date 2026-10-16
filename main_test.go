package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alandotcom/tailscale-forwarder/internal/config"
	"golang.org/x/sync/errgroup"
)

type observedListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *observedListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.closed) })
	return err
}

func startAcceptLoop(t *testing.T, grace time.Duration) (net.Conn, *observedListener, <-chan error, context.CancelFunc) {
	t.Helper()
	host, port := startEchoTarget(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedListener{Listener: listener, closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { observed.Close() })
	done := make(chan error, 1)
	go func() {
		done <- acceptLoop(ctx, observed, config.ServiceMapping{
			Name: "echo", TargetAddr: host, TargetPort: port,
		}, 1, grace)
	}()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	client.SetDeadline(time.Now().Add(3 * time.Second))
	checkEcho(t, client)
	return client, observed, done, cancel
}

func checkEcho(t *testing.T, conn net.Conn) {
	t.Helper()
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 1)
	if _, err := io.ReadFull(conn, data); err != nil || data[0] != 'x' {
		t.Fatalf("echo failed: %q, %v", data, err)
	}
}

func awaitLoop(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("accept loop did not finish")
	}
}

func TestAcceptLoopDrainsEstablishedConnections(t *testing.T) {
	client, listener, done, cancel := startAcceptLoop(t, time.Second)
	cancel()
	select {
	case <-listener.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not close on cancellation")
	}
	checkEcho(t, client)
	client.Close()
	awaitLoop(t, done)
}

func TestAcceptLoopForcesCloseAfterGrace(t *testing.T) {
	client, _, done, cancel := startAcceptLoop(t, 20*time.Millisecond)
	cancel()
	awaitLoop(t, done)
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection remained open after drain timeout")
	}
}

func TestAcceptLoopRejectsExcessConnections(t *testing.T) {
	client, listener, done, cancel := startAcceptLoop(t, time.Second)
	excess, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer excess.Close()
	excess.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := excess.Read(make([]byte, 1)); err == nil {
		t.Fatal("excess connection was not closed")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("excess connection timed out instead of being rejected")
	}
	checkEcho(t, client)
	cancel()
	client.Close()
	awaitLoop(t, done)
}

func TestAcceptLoopReusesConnectionSlots(t *testing.T) {
	first, listener, done, cancel := startAcceptLoop(t, time.Second)
	first.Close()
	deadline := time.Now().Add(time.Second)
	for {
		client, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		client.SetDeadline(time.Now().Add(200 * time.Millisecond))
		_, writeErr := client.Write([]byte("x"))
		data := make([]byte, 1)
		_, readErr := io.ReadFull(client, data)
		client.Close()
		if writeErr == nil && readErr == nil && data[0] == 'x' {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("connection slot was not released: write %v, read %v", writeErr, readErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	awaitLoop(t, done)
}

func TestServiceFailureInvalidatesReadinessBeforeDrain(t *testing.T) {
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g, serviceCtx := errgroup.WithContext(appCtx)
	host, port := startEchoTarget(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedListener{Listener: listener, closed: make(chan struct{})}
	defer observed.Close()
	g.Go(func() error {
		return acceptLoop(serviceCtx, observed, config.ServiceMapping{
			Name: "echo", TargetAddr: host, TargetPort: port,
		}, 1, time.Second)
	})
	fail := make(chan struct{})
	failure := errors.New("HTTPS listener failed")
	g.Go(func() error {
		select {
		case <-fail:
			return failure
		case <-appCtx.Done():
			return appCtx.Err()
		}
	})
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	checkEcho(t, client)
	r := newReadiness(1)
	r.markReady(serviceCtx)
	if !r.isReady() {
		t.Fatal("service was not initially ready")
	}
	done := make(chan error, 1)
	go func() { done <- g.Wait() }()
	close(fail)
	select {
	case <-observed.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("service listener did not stop")
	}
	checkEcho(t, client)
	select {
	case <-done:
		t.Fatal("service returned before its connection drained")
	default:
	}
	recorder := httptest.NewRecorder()
	healthHandler(appCtx, r).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if appCtx.Err() != nil || recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("local failure must invalidate readiness before app cancellation: %d, %v", recorder.Code, appCtx.Err())
	}
	client.Close()
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatalf("unexpected service failure: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service did not drain")
	}
}

type failedListener struct{ accepts int }

func (l *failedListener) Accept() (net.Conn, error) {
	l.accepts++
	return nil, errors.New("permanent listener failure")
}
func (*failedListener) Close() error   { return nil }
func (*failedListener) Addr() net.Addr { return &net.TCPAddr{} }

func TestAcceptLoopReturnsPermanentErrors(t *testing.T) {
	listener := &failedListener{}
	err := acceptLoop(context.Background(), listener, config.ServiceMapping{Name: "failed"}, 1, time.Second)
	if err == nil || listener.accepts != 1 {
		t.Fatalf("expected one failing accept, got %d accepts and %v", listener.accepts, err)
	}
}

func TestRunRejectsUnavailableHealthPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := run(&config.Config{HealthAddr: listener.Addr().String()}); err == nil {
		t.Fatal("expected health listener bind failure")
	}
}

func TestNewServiceServerCreatesPrivateStateDirectory(t *testing.T) {
	cfg := &config.Config{TSStateDir: t.TempDir(), TSHostname: "base", TSAuthKey: "test-key"}
	server, err := newServiceServer(cfg, config.ServiceMapping{Name: "My.Service"})
	if err != nil {
		t.Fatal(err)
	}
	if server.Hostname != "my-service-base" || server.AuthKey != cfg.TSAuthKey {
		t.Fatalf("unexpected server configuration: %s", server.Hostname)
	}
	info, err := os.Stat(server.Dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatalf("state directory must have mode 0700: %v, %v", info, err)
	}
}
