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

- **main.go**: Entry point that creates multiple `tsnet.Server` instances, one per service mapping
- **tcp.go**: Contains `fwdTCP` function that handles bidirectional TCP forwarding using `io.Copy`
- **internal/config/**: Configuration management with environment variable parsing
- **internal/logger/**: Structured logging using `log/slog` with JSON output to stdout/stderr
- **internal/util/**: Utility functions for string sanitization

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
- `TS_AUTHKEY`: Tailscale authentication key (must be reusable)
- `TS_HOSTNAME`: Base hostname for services
- `SERVICE_[n]`: Service mappings in format `servicename:sourceport:targethost:targetport`

Example:
```
TS_AUTHKEY=tskey-auth-xxxxx
TS_HOSTNAME=my-project-production
SERVICE_01=postgres:5432:postgres.internal:5432
SERVICE_02=redis:6379:redis.internal:6379
```

## Development Notes

- No test files currently exist in the codebase
- The application uses structured logging with JSON format
- All Tailscale machines are ephemeral and cleaned up on shutdown
- Service directories are created in temporary filesystem locations
- Connection errors are handled gracefully with appropriate logging