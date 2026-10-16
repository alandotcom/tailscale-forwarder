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
docker run -v "$PWD/data:/app/data" -e TS_STATE_DIR=/app/data -e TS_AUTHKEY=your-key -e TS_HOSTNAME=my-project -e SERVICE_01=postgres:5432:target.host:5432 tailscale-forwarder
```

## Architecture

### Core Components

- **main.go**: Entry point. Loads config via `config.Load()`, then `run()` creates one `tsnet.Server` per service mapping. Each service runs under its own errgroup covering the HTTPS proxy (if enabled) and the TCP accept loop, with graceful connection draining on shutdown.
- **tcp.go**: Contains `fwdTCP`, which preserves TCP half-closes and waits for both copy directions. Cancellation closes both connections and waits for the copies to stop.
- **https.go**: HTTPS reverse proxy using `Rewrite`, tsnet-provisioned certificates, and the node's actual FQDN. `TS_HTTPS_SERVICES` selects HTTP backends; `TS_ENABLE_HTTPS` preserves all-service compatibility when the list is unset.
- **health.go**: Optional `/healthz` and `/readyz` HTTP probes, served only when `HEALTH_ADDR` is set.
- **internal/config/**: Configuration via `Load() (*Config, error)` (no `init()` side effects, no `os.Exit`); pure mapping parser is unit-tested.
- **internal/logger/**: Structured logging using `log/slog` with JSON output to stdout/stderr; level set from `LOG_LEVEL`.
- **internal/util/**: Utility functions for string sanitization.

### Key Dependencies

- `tailscale.com/tsnet`: Core Tailscale networking library for creating ephemeral Tailscale machines
- `github.com/caarlos0/env/v11`: Environment variable parsing
- `golang.org/x/sync/errgroup`: Concurrent goroutine management

### Service Architecture

Each service mapping creates:
1. A unique ephemeral Tailscale machine with hostname format: `{service-name}-{base-hostname}`
2. A TCP listener on the specified source port
3. Connection forwarding to the target host:port pair
4. Dedicated persistent directory for Tailscale state

### Configuration

Required environment variables:
- `TS_AUTHKEY`: Tailscale authentication key (must be reusable; attach any desired tags to the key)
- `TS_HOSTNAME`: Base hostname for services
- `TS_STATE_DIR`: Persistent state directory (required; prevents duplicate machines on restart, and stores TLS certs when HTTPS is enabled)
- `SERVICE_[n]`: Service mappings in format `servicename:sourceport:targethost:targetport`

Optional environment variables:
- `TS_ENABLE_HTTPS`: Enable the HTTPS reverse proxy with automatic TLS certificates
- `TS_HTTPS_SERVICES`: Comma-separated service names; a nonempty list overrides the global HTTPS flag
- `MAX_CONNECTIONS`: Concurrent TCP forwards per service, default 256
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

- Tests cover configuration, TCP half-closes, connection limits, shutdown, HTTPS rewriting, redirects, and health probes. Run with `go test -race ./...`.
- Keep the runtime container running as root for Railway's root-owned state volume. See README.md for volume permissions and deployment checks.
- The application uses structured logging with JSON format
- All Tailscale machines are ephemeral and cleaned up on shutdown (each service closes its own `tsnet.Server`)
- Tailscale state lives under `TS_STATE_DIR/{sanitized-service-name}/`
- Tags are inherited from the auth key; the app has no per-service tag setting
