package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alandotcom/tailscale-forwarder/internal/config"
	"tailscale.com/ipn/ipnstate"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPSProxyRewritesForwardingHeaders(t *testing.T) {
	const hostname = "api.example.ts.net"
	proxy := newHTTPSProxy(config.ServiceMapping{
		Name: "api", TargetAddr: "::1", TargetPort: 8080,
	}, hostname)
	var outbound *http.Request
	proxy.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		outbound = r
		return &http.Response{
			StatusCode: http.StatusCreated, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader("backend response")), Request: r,
		}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "https://attacker.example/a%2Fb?key=value", nil)
	req.RemoteAddr = "100.64.0.1:12345"
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-IP"} {
		req.Header.Set(header, "spoofed")
	}
	req.Header.Set("Connection", "X-Forwarded-Proto, X-Real-IP")
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated || recorder.Body.String() != "backend response" {
		t.Fatalf("unexpected response: %d %q", recorder.Code, recorder.Body.String())
	}
	if got := outbound.URL.String(); got != "http://[::1]:8080/a%2Fb?key=value" {
		t.Fatalf("unexpected upstream URL: %q", got)
	}
	if outbound.Host != hostname {
		t.Fatalf("upstream Host = %q, want canonical hostname %q", outbound.Host, hostname)
	}
	for header, want := range map[string]string{
		"Forwarded": "", "X-Real-IP": "", "Connection": "",
		"X-Forwarded-For": "100.64.0.1", "X-Forwarded-Host": hostname,
		"X-Forwarded-Proto": "https",
	} {
		if got := outbound.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestHTTPSProxyBackendFailure(t *testing.T) {
	proxy := newHTTPSProxy(config.ServiceMapping{Name: "api", TargetAddr: "localhost", TargetPort: 8080}, "api.example.ts.net")
	proxy.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private upstream details")
	})
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "private upstream") {
		t.Fatalf("unexpected error response: %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestHTTPSRedirectUsesCanonicalHostname(t *testing.T) {
	for _, path := range []string{"/a%2Fb?key=value", "//attacker.example/path"} {
		req := httptest.NewRequest(http.MethodPost, "http://attacker.example"+path, nil)
		recorder := httptest.NewRecorder()
		httpsRedirect("api.example.ts.net").ServeHTTP(recorder, req)
		if recorder.Code != http.StatusPermanentRedirect {
			t.Fatalf("redirect must preserve request method: %d", recorder.Code)
		}
		if got := recorder.Header().Get("Location"); got != "https://api.example.ts.net"+path {
			t.Fatalf("unexpected redirect: %q", got)
		}
	}
}

func TestHTTPSHostname(t *testing.T) {
	tests := []struct {
		name   string
		status *ipnstate.Status
		want   string
	}{
		{name: "no status"},
		{name: "MagicDNS disabled", status: &ipnstate.Status{CurrentTailnet: &ipnstate.TailnetStatus{}}},
		{name: "certificates disabled", status: &ipnstate.Status{
			CurrentTailnet: &ipnstate.TailnetStatus{MagicDNSEnabled: true},
			Self:           &ipnstate.PeerStatus{DNSName: "api.example.ts.net."},
		}},
		{name: "canonical hostname", status: &ipnstate.Status{
			CurrentTailnet: &ipnstate.TailnetStatus{MagicDNSEnabled: true},
			Self:           &ipnstate.PeerStatus{DNSName: "api.example.ts.net."},
			CertDomains:    []string{"api.example.ts.net"},
		}, want: "api.example.ts.net"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := httpsHostname(tt.status)
			if got != tt.want || (err == nil) != (tt.want != "") {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
