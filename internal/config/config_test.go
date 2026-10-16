package config

import (
	"os"
	"strings"
	"testing"
)

func configureEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "SERVICE_") {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
	t.Setenv("TS_HOSTNAME", "review.example")
	t.Setenv("TS_AUTHKEY", "test-key")
	t.Setenv("TS_STATE_DIR", t.TempDir())
	t.Setenv("TS_ENABLE_HTTPS", "false")
	t.Setenv("TS_HTTPS_SERVICES", "")
	t.Setenv("MAX_CONNECTIONS", "256")
	t.Setenv("SERVICE_01", "postgres:5432:db.internal:5432")
	t.Setenv("SERVICE_02", "api:8080:api.internal:80")
}

func TestLoadConnectionLimit(t *testing.T) {
	for _, value := range []string{"0", "-1", "invalid", "16"} {
		t.Run(value, func(t *testing.T) {
			configureEnvironment(t)
			t.Setenv("MAX_CONNECTIONS", value)
			cfg, err := Load()
			if value == "16" {
				if err != nil || cfg.MaxConnections != 16 {
					t.Fatalf("unexpected connection limit: %v, %v", cfg, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "MAX_CONNECTIONS") {
				t.Fatalf("expected connection limit error, got %v", err)
			}
		})
	}
}

func TestLoadRequiredValues(t *testing.T) {
	for _, key := range []string{"TS_HOSTNAME", "TS_AUTHKEY", "TS_STATE_DIR"} {
		t.Run(key, func(t *testing.T) {
			configureEnvironment(t)
			t.Setenv(key, "")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("expected %s validation error, got %v", key, err)
			}
		})
	}
}

func TestLoadStateDirectory(t *testing.T) {
	for _, kind := range []string{"directory", "file", "missing"} {
		t.Run(kind, func(t *testing.T) {
			configureEnvironment(t)
			path := t.TempDir()
			if kind != "directory" {
				path += "/state"
			}
			if kind == "file" {
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("TS_STATE_DIR", path)
			_, err := Load()
			if (err != nil) != (kind != "directory") {
				t.Fatalf("unexpected directory validation: %v", err)
			}
		})
	}
}

func TestLoadHTTPSSelection(t *testing.T) {
	tests := []struct {
		name, global, services string
		port                   string
		wantHTTPS              int
		wantError              string
	}{
		{name: "disabled", global: "false"},
		{name: "global compatibility", global: "true", wantHTTPS: 2},
		{name: "explicit list", global: "false", services: " api ", wantHTTPS: 1},
		{name: "list overrides global", global: "true", services: "api", wantHTTPS: 1},
		{name: "unknown service", global: "false", services: "missing", wantError: "unknown service"},
		{name: "duplicate service", global: "false", services: "api,API", wantError: "duplicate service"},
		{name: "empty list entry", global: "false", services: "api,", wantError: "invalid service name"},
		{name: "port 80 conflict", global: "true", port: "80", wantError: "conflicts"},
		{name: "port 443 conflict", global: "false", services: "api", port: "443", wantError: "conflicts"},
		{name: "TCP port 80 remains available", global: "false", port: "80"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configureEnvironment(t)
			t.Setenv("TS_ENABLE_HTTPS", tt.global)
			t.Setenv("TS_HTTPS_SERVICES", tt.services)
			if tt.port != "" {
				t.Setenv("SERVICE_02", "api:"+tt.port+":api.internal:80")
			}
			cfg, err := Load()
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q error, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var enabled int
			for _, mapping := range cfg.ServiceMappings {
				if mapping.HTTPS {
					enabled++
					if tt.services != "" && mapping.Name != "api" {
						t.Fatalf("unexpected HTTPS service: %s", mapping.Name)
					}
				}
			}
			if enabled != tt.wantHTTPS {
				t.Fatalf("got %d HTTPS services, want %d", enabled, tt.wantHTTPS)
			}
		})
	}
}
