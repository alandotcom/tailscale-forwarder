# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Tailscale Forwarder is a TCP proxy service that allows secure connections through Tailscale to configured target addresses and ports. It creates multiple Tailscale machines, each with service-specific hostnames for database connections, APIs, and other services that need to be accessed through a private Tailscale network.

## Build and Run Commands

```bash
# Build the application
go build -o tailscale_fwdr .

# Run the application (requires environment variables)
./tailscale_fwdr

# Build with Docker
docker build -t tailscale-forwarder .

# Run with Docker
docker run -e TS_AUTHKEY=your-key -e TS_HOSTNAME=my-project -e SERVICE_01=postgres:5432:target.host:5432 tailscale-forwarder
```

## Architecture

### Core Components

- **main.go**: Entry point. Loads config via `config.Load()`, then `run()` creates one `tsnet.Server` per service mapping. Each service runs under its own errgroup covering the HTTPS proxy (if enabled) and the TCP accept loop, with graceful connection draining on shutdown.
- **tcp.go**: Contains `fwdTCP`, which dials the target and copies bytes bidirectionally with `io.Copy`. It returns when either direction closes or the context is cancelled, keeping live goroutines bounded by concurrent (not total) connections.
- **https.go**: Optional HTTPS reverse proxy (enabled by `TS_ENABLE_HTTPS`) with tsnet-provisioned TLS certificates and an HTTP→HTTPS redirect.
- **health.go**: Optional `/healthz` and `/readyz` HTTP probes, served only when `HEALTH_ADDR` is set.
- **internal/config/**: Configuration via `Load() (*Config, error)` (no `init()` side effects, no `os.Exit`); pure mapping parser is unit-tested.
- **internal/logger/**: Structured logging using `log/slog` with JSON output to stdout/stderr; level set from `LOG_LEVEL`.
- **internal/util/**: Utility functions for string sanitization.

### Key Dependencies

- `tailscale.com/tsnet`: Core Tailscale networking library for creating ephemeral Tailscale machines
- `github.com/caarlos0/env/v10`: Environment variable parsing
- `golang.org/x/sync/errgroup`: Concurrent goroutine management

### Service Architecture

Each service mapping creates:
1. A unique ephemeral Tailscale machine with hostname format: `{service-name}-{base-hostname}`
2. A TCP listener on the specified source port
3. Connection forwarding to the target host:port pair
4. Dedicated temporary directory for Tailscale state

### Configuration

Required environment variables:
- `TS_AUTHKEY`: Tailscale authentication key (must be reusable; attach any desired tags to the key)
- `TS_HOSTNAME`: Base hostname for services
- `TS_STATE_DIR`: Persistent state directory (required; prevents duplicate machines on restart, and stores TLS certs when HTTPS is enabled)
- `SERVICE_[n]`: Service mappings in format `servicename:sourceport:targethost:targetport`

Optional environment variables:
- `TS_ENABLE_HTTPS`: Enable the HTTPS reverse proxy with automatic TLS certificates
- `LOG_LEVEL`: `debug`, `info` (default), `warn`, or `error`
- `HEALTH_ADDR`: If set (e.g. `:8080`), serve `/healthz` and `/readyz` probes on this address

Example:
```
TS_AUTHKEY=tskey-auth-xxxxx
TS_HOSTNAME=my-project-production
TS_STATE_DIR=/app/data
SERVICE_01=postgres:5432:postgres.internal:5432
SERVICE_02=redis:6379:redis.internal:6379
```

## Development Notes

- Tests cover the pure logic (`internal/util` sanitization, `internal/config` mapping parsing, and `fwdTCP` forwarding). Run with `go test ./...`.
- The application uses structured logging with JSON format
- All Tailscale machines are ephemeral and cleaned up on shutdown (each service closes its own `tsnet.Server`)
- Tailscale state lives under `TS_STATE_DIR/{sanitized-service-name}/`
- Tags are inherited from the auth key; the app has no per-service tag setting