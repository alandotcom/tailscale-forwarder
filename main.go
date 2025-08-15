package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
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

	var servers []*tsnet.Server
	defer func() {
		for _, srv := range servers {
			srv.Close()
		}
	}()

	g := &errgroup.Group{}

	for _, serviceMapping := range config.Cfg.ServiceMappings {
		serviceMapping := serviceMapping // Capture by value to avoid race condition

		// Create unique hostname for each service
		serviceHostname := fmt.Sprintf("%s-%s", serviceMapping.Name, config.Cfg.TSHostname)
		sanitizedHostname := util.SanitizeString(serviceHostname)

		// Create unique directory for each service
		var serviceDir string
		if config.Cfg.TSHostname != "" {
			// Use a base directory with service-specific subdirectories
			baseDir := filepath.Join(os.TempDir(), "tailscale-forwarder")
			serviceDir = filepath.Join(baseDir, serviceMapping.Name)

			// Handle directory creation error
			if err := os.MkdirAll(serviceDir, 0700); err != nil {
				logger.Stderr.Error("failed to create service directory",
					slog.String("service", serviceMapping.Name),
					slog.String("dir", serviceDir),
					logger.ErrAttr(err))
				os.Exit(1)
			}
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

		servers = append(servers, ts)

		// Start each service in a separate goroutine
		g.Go(func() error {
			return runServiceMapping(ts, serviceMapping)
		})
	}

	if err := g.Wait(); err != nil {
		logger.Stderr.Error("service mapping failed", logger.ErrAttr(err))
		os.Exit(1)
	}
}

func runServiceMapping(ts *tsnet.Server, serviceMapping config.ServiceMapping) error {
	if _, err := ts.Up(context.Background()); err != nil {
		return fmt.Errorf("failed to connect %s to tailscale: %w", serviceMapping.Name, err)
	}

	if err := ts.Start(); err != nil {
		return fmt.Errorf("failed to start tailscale network server for %s: %w", serviceMapping.Name, err)
	}

	listener, err := ts.Listen("tcp", fmt.Sprintf(":%d", serviceMapping.SourcePort))
	if err != nil {
		return fmt.Errorf("failed to start listener for %s on port %d: %w", serviceMapping.Name, serviceMapping.SourcePort, err)
	}

	logger.Stdout.Info("service ready",
		slog.String("service", serviceMapping.Name),
		slog.String("hostname", ts.Hostname),
		slog.Int("source_port", serviceMapping.SourcePort),
		slog.String("target_addr", serviceMapping.TargetAddr),
		slog.Int("target_port", serviceMapping.TargetPort),
	)

	for {
		sourceConn, err := listener.Accept()
		if err != nil {
			logger.Stderr.Error("failed to accept connection",
				slog.String("service", serviceMapping.Name),
				slog.Int("source_port", serviceMapping.SourcePort),
				logger.ErrAttr(err),
			)
			continue
		}

		go func() {
			if err := fwdTCP(sourceConn, serviceMapping.TargetAddr, serviceMapping.TargetPort); err != nil {
				if errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET) {
					return
				}

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
