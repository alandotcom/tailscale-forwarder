package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/alandotcom/tailscale-forwarder/internal/config"
	"github.com/alandotcom/tailscale-forwarder/internal/logger"
	"github.com/alandotcom/tailscale-forwarder/internal/util"

	"golang.org/x/sync/errgroup"
	"tailscale.com/tsnet"
)

// shutdownGrace is how long a service waits for in-flight connections to finish
// after it stops accepting new ones, before forcing them closed.
const shutdownGrace = 30 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		logger.StderrWithSource.Error("configuration error(s) found", logger.ErrAttr(err))
		os.Exit(1)
	}
	logger.SetLevel(cfg.LogLevel)

	if err := run(cfg); err != nil {
		logger.Stderr.Error("fatal error", logger.ErrAttr(err))
		os.Exit(1)
	}
	logger.Stdout.Info("application shutdown complete")
}

func run(cfg *config.Config) error {
	logger.Stdout.Info("🚀 Starting tailscale_fwdr",
		slog.Any("service-mappings", cfg.ServiceMappings),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Translate SIGINT/SIGTERM into context cancellation for graceful shutdown.
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan
		logger.Stdout.Info("received shutdown signal, initiating graceful shutdown",
			slog.String("signal", sig.String()),
		)
		cancel()
	}()

	readiness := newReadiness(len(cfg.ServiceMappings))
	if cfg.HealthAddr != "" {
		go serveHealth(ctx, cfg.HealthAddr, readiness)
	}

	g, gCtx := errgroup.WithContext(ctx)

	for _, serviceMapping := range cfg.ServiceMappings {
		ts, err := newServiceServer(cfg, serviceMapping)
		if err != nil {
			return err
		}

		g.Go(func() error {
			return runServiceMapping(gCtx, ts, serviceMapping, cfg.TSEnableHTTPS, readiness)
		})
	}

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// newServiceServer builds the tsnet node for a service mapping. The sanitized
// service name is used for both the persistent state directory and the
// Tailscale hostname so the two always agree; the state directory is created
// before the node is returned.
func newServiceServer(cfg *config.Config, serviceMapping config.ServiceMapping) (*tsnet.Server, error) {
	serviceName := util.SanitizeHostname(serviceMapping.Name)
	serviceDir := filepath.Join(cfg.TSStateDir, serviceName)
	if err := os.MkdirAll(serviceDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create service directory for %s (%s): %w", serviceMapping.Name, serviceDir, err)
	}

	return &tsnet.Server{
		Hostname:     serviceName + "-" + cfg.TSHostname,
		AuthKey:      cfg.TSAuthKey,
		RunWebClient: false,
		Ephemeral:    true,
		Dir:          serviceDir,
		UserLogf: func(format string, v ...any) {
			logger.Stdout.Info(fmt.Sprintf(format, v...), slog.String("service", serviceMapping.Name))
		},
	}, nil
}

func runServiceMapping(ctx context.Context, ts *tsnet.Server, serviceMapping config.ServiceMapping, enableHTTPS bool, readiness *readiness) error {
	// Each service owns its tsnet node and closes it when the goroutine exits.
	defer ts.Close()

	// Up connects the node to the tailnet and blocks until it is usable; it
	// calls Start internally, so no separate Start call is needed.
	if _, err := ts.Up(ctx); err != nil {
		return fmt.Errorf("failed to connect %s to tailscale: %w", serviceMapping.Name, err)
	}

	// One errgroup owns both the HTTPS proxy (if enabled) and the TCP accept
	// loop, so a failure in either cancels the other and is reported upward.
	g, gCtx := errgroup.WithContext(ctx)

	// Open the TCP listener before registering any goroutines so a bind failure
	// returns cleanly without leaving group goroutines unwaited. ts.Close (the
	// deferred call above) tears down this listener on return.
	listener, err := ts.Listen("tcp", fmt.Sprintf(":%d", serviceMapping.SourcePort))
	if err != nil {
		return fmt.Errorf("failed to start listener for %s on port %d: %w", serviceMapping.Name, serviceMapping.SourcePort, err)
	}

	if enableHTTPS {
		if err := startHTTPSProxy(g, gCtx, ts, serviceMapping); err != nil {
			return fmt.Errorf("failed to start HTTPS proxy for %s: %w", serviceMapping.Name, err)
		}
	}

	g.Go(func() error {
		return acceptLoop(gCtx, listener, serviceMapping)
	})

	// The listener is bound; the service is ready to serve.
	readiness.markReady()

	logArgs := []any{
		slog.String("service", serviceMapping.Name),
		slog.String("hostname", ts.Hostname),
		slog.Int("source_port", serviceMapping.SourcePort),
		slog.String("target_addr", serviceMapping.TargetAddr),
		slog.Int("target_port", serviceMapping.TargetPort),
	}
	if enableHTTPS {
		logArgs = append(logArgs, slog.String("https_url", fmt.Sprintf("https://%s/", ts.Hostname)))
	}
	logger.Stdout.Info("service ready", logArgs...)

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// acceptLoop accepts connections and forwards each in its own goroutine. On
// shutdown it stops accepting, then gives in-flight connections up to
// shutdownGrace to finish before forcing them closed.
func acceptLoop(ctx context.Context, listener net.Listener, serviceMapping config.ServiceMapping) error {
	// connCtx is cancelled only when we give up waiting for connections to
	// drain; until then, in-flight forwards run to completion on their own.
	connCtx, forceClose := context.WithCancel(context.Background())
	defer forceClose()

	// Close the listener on shutdown to unblock Accept and stop new connections.
	go func() {
		<-ctx.Done()
		logger.Stdout.Info("shutting down TCP listener",
			slog.String("service", serviceMapping.Name),
			slog.Int("port", serviceMapping.SourcePort),
		)
		listener.Close()
	}()

	var wg sync.WaitGroup
	for {
		sourceConn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			logger.Stderr.Error("failed to accept connection",
				slog.String("service", serviceMapping.Name),
				slog.Int("source_port", serviceMapping.SourcePort),
				logger.ErrAttr(err),
			)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer sourceConn.Close()
			if err := fwdTCP(connCtx, sourceConn, serviceMapping.TargetAddr, serviceMapping.TargetPort); err != nil {
				logger.Stderr.Error("failed to forward connection",
					slog.String("service", serviceMapping.Name),
					slog.Int("source_port", serviceMapping.SourcePort),
					slog.String("target_addr", serviceMapping.TargetAddr),
					slog.Int("target_port", serviceMapping.TargetPort),
					logger.ErrAttr(err),
				)
			}
		}()
	}

	// Graceful drain: wait for active connections, force them closed on timeout.
	if drained := waitWithTimeout(&wg, shutdownGrace); !drained {
		logger.Stdout.Info("drain timeout reached, forcing connections closed",
			slog.String("service", serviceMapping.Name),
		)
		forceClose()
		wg.Wait()
	}

	logger.Stdout.Info("TCP service shutting down",
		slog.String("service", serviceMapping.Name),
	)
	return nil
}

// waitWithTimeout reports whether wg completed before the timeout elapsed.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
