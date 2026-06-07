package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/alandotcom/tailscale-forwarder/internal/logger"
)

// readiness tracks how many service mappings have bound their listeners. It is
// reported by the /readyz endpoint so an orchestrator can gate traffic until
// every service is actually serving.
type readiness struct {
	total int64
	ready atomic.Int64
}

func newReadiness(total int) *readiness {
	return &readiness{total: int64(total)}
}

func (r *readiness) markReady() {
	r.ready.Add(1)
}

func (r *readiness) isReady() bool {
	return r.ready.Load() >= r.total
}

// serveHealth runs a plain (non-tsnet) HTTP server exposing liveness and
// readiness probes on addr. It is only started when HEALTH_ADDR is configured,
// and shuts down when ctx is cancelled.
func serveHealth(ctx context.Context, addr string, r *readiness) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if r.isReady() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Stdout.Info("starting health server", slog.String("addr", addr))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Stderr.Error("health server failed", slog.String("addr", addr), logger.ErrAttr(err))
	}
}
