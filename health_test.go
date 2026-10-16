package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthReadinessLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newReadiness(2)
	serviceCtx, stopService := context.WithCancel(ctx)
	defer stopService()
	handler := healthHandler(ctx, r)
	check := func(path string, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != want {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, want)
		}
	}
	check("/healthz", http.StatusOK)
	check("/readyz", http.StatusServiceUnavailable)
	r.markReady(serviceCtx)
	check("/readyz", http.StatusServiceUnavailable)
	r.markReady(ctx)
	check("/readyz", http.StatusOK)
	stopService()
	check("/readyz", http.StatusServiceUnavailable)
	stopService()
	cancel()
	check("/readyz", http.StatusServiceUnavailable)
	check("/healthz", http.StatusOK)
}

func TestServeHealthShutsDown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveHealth(ctx, listener, newReadiness(1)) }()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("health server did not stop")
	}
}
