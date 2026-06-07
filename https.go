package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/alandotcom/tailscale-forwarder/internal/config"
	"github.com/alandotcom/tailscale-forwarder/internal/logger"

	"golang.org/x/sync/errgroup"
	"tailscale.com/tsnet"
)

// startHTTPSProxy registers an HTTP→HTTPS redirect server and an HTTPS reverse
// proxy (with tsnet-provisioned certificates) on the provided errgroup. By
// sharing the caller's group, a server failure propagates up and tears the
// service down rather than being silently logged in a detached goroutine.
func startHTTPSProxy(g *errgroup.Group, ctx context.Context, ts *tsnet.Server, serviceMapping config.ServiceMapping) error {
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d", serviceMapping.TargetAddr, serviceMapping.TargetPort))
	if err != nil {
		return fmt.Errorf("failed to parse target URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Strip client-supplied forwarding headers before the proxy appends its
	// own, so a tailnet client cannot spoof its apparent origin to the backend.
	baseDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		r.Header.Del("X-Forwarded-For")
		r.Header.Del("X-Forwarded-Host")
		r.Header.Del("X-Forwarded-Proto")
		r.Header.Del("X-Real-Ip")
		baseDirector(r)
		r.Header.Set("X-Forwarded-Proto", "https")
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		logger.Stdout.Info("HTTPS proxy response",
			slog.String("service", serviceMapping.Name),
			slog.String("method", resp.Request.Method),
			slog.String("path", resp.Request.URL.Path),
			slog.Int("status", resp.StatusCode),
		)
		return nil
	}

	// Redirect to the node's own hostname rather than reflecting the inbound
	// Host header, which removes the open-redirect ingredient.
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpsURL := fmt.Sprintf("https://%s%s", ts.Hostname, r.URL.RequestURI())
		http.Redirect(w, r, httpsURL, http.StatusMovedPermanently)
	})

	httpsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Stdout.Info("HTTPS proxy request",
			slog.String("service", serviceMapping.Name),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
		)
		proxy.ServeHTTP(w, r)
	})

	localClient, err := ts.LocalClient()
	if err != nil {
		return fmt.Errorf("failed to get LocalClient for %s: %w", serviceMapping.Name, err)
	}
	tlsConfig := &tls.Config{
		GetCertificate: localClient.GetCertificate,
	}

	httpServer := newHTTPServer(httpHandler, nil)
	httpsServer := newHTTPServer(httpsHandler, tlsConfig)

	httpListener, err := ts.Listen("tcp", ":80")
	if err != nil {
		return fmt.Errorf("failed to start HTTP listener for %s: %w", serviceMapping.Name, err)
	}

	httpsListener, err := ts.Listen("tcp", ":443")
	if err != nil {
		httpListener.Close()
		return fmt.Errorf("failed to start HTTPS listener for %s: %w", serviceMapping.Name, err)
	}

	g.Go(func() error {
		logger.Stdout.Info("starting HTTP redirect server",
			slog.String("service", serviceMapping.Name),
			slog.String("addr", httpListener.Addr().String()),
		)
		if err := httpServer.Serve(httpListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP redirect server failed for %s: %w", serviceMapping.Name, err)
		}
		return nil
	})

	g.Go(func() error {
		logger.Stdout.Info("starting HTTPS proxy server",
			slog.String("service", serviceMapping.Name),
			slog.String("addr", httpsListener.Addr().String()),
			slog.String("target", targetURL.String()),
		)
		if err := httpsServer.ServeTLS(httpsListener, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTPS proxy server failed for %s: %w", serviceMapping.Name, err)
		}
		return nil
	})

	// On shutdown, drain both servers gracefully. Returning nil keeps a clean
	// shutdown from being reported as a service error.
	g.Go(func() error {
		<-ctx.Done()
		logger.Stdout.Info("shutting down HTTPS proxy servers",
			slog.String("service", serviceMapping.Name),
		)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Stderr.Error("HTTP server shutdown failed",
				slog.String("service", serviceMapping.Name),
				logger.ErrAttr(err),
			)
		}
		if err := httpsServer.Shutdown(shutdownCtx); err != nil {
			logger.Stderr.Error("HTTPS server shutdown failed",
				slog.String("service", serviceMapping.Name),
				logger.ErrAttr(err),
			)
		}
		logger.Stdout.Info("HTTPS proxy servers stopped",
			slog.String("service", serviceMapping.Name),
		)
		return nil
	})

	logger.Stdout.Info("HTTPS proxy started",
		slog.String("service", serviceMapping.Name),
		slog.String("https_url", fmt.Sprintf("https://%s/", ts.Hostname)),
		slog.String("target", targetURL.String()),
	)

	return nil
}

// newHTTPServer builds an http.Server with the project's standard timeouts.
func newHTTPServer(handler http.Handler, tlsConfig *tls.Config) *http.Server {
	return &http.Server{
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
