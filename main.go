package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"main/internal/config"
	"main/internal/logger"
	"main/internal/util"

	"golang.org/x/sync/errgroup"
	"tailscale.com/tsnet"
)

func main() {
	logger.Stdout.Info("🚀 Starting tailscale_fwdr",
		slog.Any("service-mappings", config.Cfg.ServiceMappings),
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan
		logger.Stdout.Info("received shutdown signal, initiating graceful shutdown",
			slog.String("signal", sig.String()),
		)
		cancel()
	}()

	var servers []*tsnet.Server
	defer func() {
		for _, srv := range servers {
			srv.Close()
		}
	}()

	// Use context-aware errgroup
	g, gCtx := errgroup.WithContext(ctx)

	for _, serviceMapping := range config.Cfg.ServiceMappings {
		serviceMapping := serviceMapping // Capture by value to avoid race condition

		// Create unique hostname for each service
		serviceHostname := fmt.Sprintf("%s.%s", serviceMapping.Name, config.Cfg.TSHostname)
		sanitizedHostname := util.SanitizeString(serviceHostname)

		// Create unique directory for each service
		var serviceDir string
		if config.Cfg.TSStateDir != "" {
			// Use persistent state directory
			serviceDir = filepath.Join(config.Cfg.TSStateDir, serviceMapping.Name)
		} else {
			// Fallback to temporary directory for backward compatibility
			baseDir := filepath.Join(os.TempDir(), "tailscale-forwarder")
			serviceDir = filepath.Join(baseDir, serviceMapping.Name)
		}

		// Handle directory creation error
		if err := os.MkdirAll(serviceDir, 0700); err != nil {
			logger.Stderr.Error("failed to create service directory",
				slog.String("service", serviceMapping.Name),
				slog.String("dir", serviceDir),
				logger.ErrAttr(err))
			os.Exit(1)
		}

		ts := &tsnet.Server{
			Hostname:     sanitizedHostname,
			AuthKey:      config.Cfg.TSAuthKey,
			RunWebClient: false,
			Ephemeral:    true,
			Dir:          serviceDir,
			UserLogf: func(format string, v ...any) {
				logger.Stdout.Info(fmt.Sprintf("[%s] %s", serviceMapping.Name, fmt.Sprintf(format, v...)))
			},
		}

		// Apply extra args if configured (via environment)
		if config.Cfg.TSExtraArgs != "" {
			extraArgs := config.ParseExtraArgs(config.Cfg.TSExtraArgs)
			logger.Stdout.Info("extra tailscale args configured",
				slog.String("service", serviceMapping.Name),
				slog.Any("args", extraArgs),
			)
			// Note: TS_EXTRA_ARGS should be set globally via environment variables
			// before starting the application, not per-service to avoid race conditions
		}

		servers = append(servers, ts)

		// Start each service in a separate goroutine
		g.Go(func() error {
			return runServiceMapping(ts, serviceMapping, gCtx)
		})
	}

	if err := g.Wait(); err != nil && err != context.Canceled {
		logger.Stderr.Error("service mapping failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	logger.Stdout.Info("application shutdown complete")
}

func runServiceMapping(ts *tsnet.Server, serviceMapping config.ServiceMapping, ctx context.Context) error {

	if _, err := ts.Up(ctx); err != nil {
		return fmt.Errorf("failed to connect %s to tailscale: %w", serviceMapping.Name, err)
	}

	if err := ts.Start(); err != nil {
		return fmt.Errorf("failed to start tailscale network server for %s: %w", serviceMapping.Name, err)
	}

	// Start HTTPS proxy if enabled
	if err := startHTTPSProxy(ctx, ts, serviceMapping); err != nil {
		return fmt.Errorf("failed to start HTTPS proxy for %s: %w", serviceMapping.Name, err)
	}

	listener, err := ts.Listen("tcp", fmt.Sprintf(":%d", serviceMapping.SourcePort))
	if err != nil {
		return fmt.Errorf("failed to start listener for %s on port %d: %w", serviceMapping.Name, serviceMapping.SourcePort, err)
	}

	// Setup graceful shutdown for TCP listener
	go func() {
		<-ctx.Done()
		logger.Stdout.Info("shutting down TCP listener",
			slog.String("service", serviceMapping.Name),
			slog.Int("port", serviceMapping.SourcePort),
		)

		// Close listener to stop accepting new connections
		listener.Close()
	}()

	logArgs := []any{
		slog.String("service", serviceMapping.Name),
		slog.String("hostname", ts.Hostname),
		slog.Int("source_port", serviceMapping.SourcePort),
		slog.String("target_addr", serviceMapping.TargetAddr),
		slog.Int("target_port", serviceMapping.TargetPort),
	}

	if config.Cfg.TSEnableHTTPS {
		logArgs = append(logArgs, slog.String("https_url", fmt.Sprintf("https://%s/", ts.Hostname)))
	}

	logger.Stdout.Info("service ready", logArgs...)

	for {
		select {
		case <-ctx.Done():
			logger.Stdout.Info("TCP service shutting down",
				slog.String("service", serviceMapping.Name),
			)
			return ctx.Err()
		default:
			sourceConn, err := listener.Accept()
			if err != nil {
				// Check if error is due to context cancellation
				if ctx.Err() != nil {
					return ctx.Err()
				}
				logger.Stderr.Error("failed to accept connection",
					slog.String("service", serviceMapping.Name),
					slog.Int("source_port", serviceMapping.SourcePort),
					logger.ErrAttr(err),
				)
				continue
			}

			go func() {
				defer sourceConn.Close()
				if err := fwdTCP(ctx, sourceConn, serviceMapping.TargetAddr, serviceMapping.TargetPort); err != nil {
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
	}
}
