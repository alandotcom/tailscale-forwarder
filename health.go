package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/alandotcom/tailscale-forwarder/internal/logger"
)

// readiness tracks bound services and their serving contexts. A failed service
// becomes unready as soon as its context is cancelled, before connections drain.
type readiness struct {
	total    int
	mu       sync.RWMutex
	services []context.Context
}

func newReadiness(total int) *readiness {
	return &readiness{total: total}
}

func (r *readiness) markReady(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services = append(r.services, ctx)
}

func (r *readiness) isReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.services) != r.total {
		return false
	}
	for _, ctx := range r.services {
		if ctx.Err() != nil {
			return false
		}
	}
	return true
}

func healthHandler(ctx context.Context, r *readiness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ctx.Err() == nil && r.isReady() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
	})
	return mux
}

func serveHealth(ctx context.Context, listener net.Listener, r *readiness) error {
	srv := &http.Server{
		Handler:           healthHandler(ctx, r),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	defer srv.Close()

	servingDone := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
		case <-servingDone:
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			srv.Close()
		}
	}()

	logger.Stdout.Info("starting health server", slog.String("addr", listener.Addr().String()))
	err := srv.Serve(listener)
	close(servingDone)
	<-shutdownDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("health server failed: %w", err)
	}
	return nil
}
