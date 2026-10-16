package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alandotcom/tailscale-forwarder/internal/config"
	"github.com/alandotcom/tailscale-forwarder/internal/logger"

	"golang.org/x/sync/errgroup"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

func httpsHostname(status *ipnstate.Status) (string, error) {
	if status == nil || status.CurrentTailnet == nil || !status.CurrentTailnet.MagicDNSEnabled || status.Self == nil {
		return "", errors.New("HTTPS requires MagicDNS to be enabled in the tailnet")
	}
	hostname := strings.TrimSuffix(status.Self.DNSName, ".")
	for _, domain := range status.CertDomains {
		if hostname != "" && strings.TrimSuffix(domain, ".") == hostname {
			return hostname, nil
		}
	}
	return "", errors.New("HTTPS certificates must be enabled for this node in the tailnet DNS settings")
}

func newHTTPSProxy(serviceMapping config.ServiceMapping, hostname string) *httputil.ReverseProxy {
	targetURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(serviceMapping.TargetAddr, strconv.Itoa(serviceMapping.TargetPort)),
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(targetURL)
			r.Out.Host = hostname
			r.Out.Header.Del("X-Real-Ip")
			r.SetXForwarded()
			r.Out.Header.Set("X-Forwarded-Host", hostname)
			r.Out.Header.Set("X-Forwarded-Proto", "https")
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			logger.Stderr.Error("HTTPS upstream request failed",
				slog.String("service", serviceMapping.Name), logger.ErrAttr(err))
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
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
	return proxy
}

func httpsRedirect(hostname string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := &url.URL{
			Scheme: "https", Host: hostname,
			Path: r.URL.Path, RawPath: r.URL.RawPath,
			RawQuery: r.URL.RawQuery, ForceQuery: r.URL.ForceQuery,
		}
		http.Redirect(w, r, target.String(), http.StatusPermanentRedirect)
	})
}

func startHTTPSProxy(g *errgroup.Group, ctx context.Context, ts *tsnet.Server, serviceMapping config.ServiceMapping, hostname string) error {
	proxy := newHTTPSProxy(serviceMapping, hostname)
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

	httpServer := newHTTPServer(httpsRedirect(hostname), nil)
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
			slog.String("target", net.JoinHostPort(serviceMapping.TargetAddr, strconv.Itoa(serviceMapping.TargetPort))),
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
			httpServer.Close()
			logger.Stderr.Error("HTTP server shutdown failed",
				slog.String("service", serviceMapping.Name),
				logger.ErrAttr(err),
			)
		}
		if err := httpsServer.Shutdown(shutdownCtx); err != nil {
			httpsServer.Close()
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
		slog.String("https_url", fmt.Sprintf("https://%s/", hostname)),
		slog.String("target", net.JoinHostPort(serviceMapping.TargetAddr, strconv.Itoa(serviceMapping.TargetPort))),
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
