package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"main/internal/config"
	"main/internal/logger"

	"golang.org/x/sync/errgroup"
	"tailscale.com/tsnet"
)

func startHTTPSProxy(ctx context.Context, ts *tsnet.Server, serviceMapping config.ServiceMapping) error {
	if !config.Cfg.TSEnableHTTPS {
		return nil // HTTPS not enabled
	}

	// Create target URL for reverse proxy
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d", serviceMapping.TargetAddr, serviceMapping.TargetPort))
	if err != nil {
		return fmt.Errorf("failed to parse target URL: %w", err)
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Add logging to proxy
	proxy.ModifyResponse = func(resp *http.Response) error {
		logger.Stdout.Info("HTTPS proxy response",
			slog.String("service", serviceMapping.Name),
			slog.String("method", resp.Request.Method),
			slog.String("path", resp.Request.URL.Path),
			slog.Int("status", resp.StatusCode),
		)
		return nil
	}

	// Create HTTP handler that redirects to HTTPS
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpsURL := fmt.Sprintf("https://%s%s", r.Host, r.RequestURI)
		http.Redirect(w, r, httpsURL, http.StatusMovedPermanently)
	})

	// Create HTTPS handler
	httpsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Stdout.Info("HTTPS proxy request",
			slog.String("service", serviceMapping.Name),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
		)
		proxy.ServeHTTP(w, r)
	})

	// Configure TLS with automatic certificate provisioning using tsnet's LocalClient
	localClient, err := ts.LocalClient()
	if err != nil {
		return fmt.Errorf("failed to get LocalClient for %s: %w", serviceMapping.Name, err)
	}
	tlsConfig := &tls.Config{
		GetCertificate: localClient.GetCertificate,
	}

	// Create HTTP server for redirect with timeouts
	httpServer := &http.Server{
		Handler:      httpHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Create HTTPS server with timeouts
	httpsServer := &http.Server{
		Handler:      httpsHandler,
		TLSConfig:    tlsConfig,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP listener (for redirects)
	httpListener, err := ts.Listen("tcp", ":80")
	if err != nil {
		return fmt.Errorf("failed to start HTTP listener for %s: %w", serviceMapping.Name, err)
	}

	// Start HTTPS listener
	httpsListener, err := ts.Listen("tcp", ":443")
	if err != nil {
		httpListener.Close()
		return fmt.Errorf("failed to start HTTPS listener for %s: %w", serviceMapping.Name, err)
	}

	// Use errgroup to manage HTTP/HTTPS servers with proper error handling
	g, gCtx := errgroup.WithContext(ctx)

	// Start HTTP server (for redirects)
	g.Go(func() error {
		logger.Stdout.Info("starting HTTP redirect server",
			slog.String("service", serviceMapping.Name),
			slog.String("addr", httpListener.Addr().String()),
		)

		if err := httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("HTTP redirect server failed: %w", err)
		}
		return nil
	})

	// Start HTTPS server
	g.Go(func() error {
		logger.Stdout.Info("starting HTTPS proxy server",
			slog.String("service", serviceMapping.Name),
			slog.String("addr", httpsListener.Addr().String()),
			slog.String("target", targetURL.String()),
		)

		if err := httpsServer.ServeTLS(httpsListener, "", ""); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("HTTPS proxy server failed: %w", err)
		}
		return nil
	})

	// Start graceful shutdown handler
	g.Go(func() error {
		<-gCtx.Done()
		logger.Stdout.Info("shutting down HTTPS proxy servers",
			slog.String("service", serviceMapping.Name),
		)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Shutdown HTTP server
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Stderr.Error("HTTP server shutdown failed",
				slog.String("service", serviceMapping.Name),
				logger.ErrAttr(err),
			)
		}

		// Shutdown HTTPS server
		if err := httpsServer.Shutdown(shutdownCtx); err != nil {
			logger.Stderr.Error("HTTPS server shutdown failed",
				slog.String("service", serviceMapping.Name),
				logger.ErrAttr(err),
			)
		}

		logger.Stdout.Info("HTTPS proxy servers stopped",
			slog.String("service", serviceMapping.Name),
		)
		return gCtx.Err()
	})

	logger.Stdout.Info("HTTPS proxy started",
		slog.String("service", serviceMapping.Name),
		slog.String("https_url", fmt.Sprintf("https://%s/", ts.Hostname)),
		slog.String("target", targetURL.String()),
	)

	// Simple background management - let servers start naturally
	go func() {
		if err := g.Wait(); err != nil && err != context.Canceled {
			logger.Stderr.Error("HTTPS proxy group error",
				slog.String("service", serviceMapping.Name),
				logger.ErrAttr(err),
			)
		}
	}()

	return nil
}
