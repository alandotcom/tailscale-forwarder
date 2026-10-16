## Tailscale Forwarder

Tailscale Forwarder is a TCP proxy that allows you to connect through a Tailscale machine to the configured target address and port pair.

This allows you to connect to Railway services that are not accessible from the internet, for example, locking down access to your database to only those who are on your Tailscale network.

## Usage

1. Generate a Tailscale [auth key](https://tailscale.com/kb/1085/auth-keys).

   Make sure `Reusable` is enabled.

2. Enable [MagicDNS](https://tailscale.com/kb/1081/magicdns) for your Tailscale account.

   This is required so that your computer can resolve the Tailscale Forwarder machine's short hostname to the correct IP address.

3. Deploy the Tailscale Forwarder service into your pre-existing Railway project.

   Set the `TS_AUTHKEY` environment variable to the auth key you generated in step 1.

   **Important**: Set `TS_STATE_DIR=/app/data` and mount a Railway volume at `/app/data` to prevent duplicate Tailscale machines on restart.

   Set your first service mapping, example:

   `SERVICE_01=postgres:5432:${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:${{Postgres.PGPORT}}`

   The format is `<Service Name>:<Source Port>:<Target Host>:<Target Port>`.

   Note: You can set multiple service mappings by incrementing the `SERVICE_` prefix.

4. Get the service hostnames.

   You should see multiple machines in the Tailscale [dashboard](https://login.tailscale.com/admin/machines) with the format `<Service Name>-<Base Hostname>` (dots in hostname are converted to hyphens).

   Each service gets its own descriptive hostname.

5. Use the service-specific hostname to connect.

   Example: `postgresql://postgres:<Postgres Password>@postgres-my-project-production-railway:5432/railway`

   Each service has a clear, descriptive hostname that tells you exactly what you're connecting to.

## Configuration

| Environment Variable | Required | Default Value                                                     | Description                                                                                |
| -------------------- | :------: | ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `TS_AUTHKEY`         |   Yes    | -                                                                 | Tailscale auth key.                                                                        |
| `TS_HOSTNAME`        |   Yes    | - | Base hostname for services. Dots are converted to hyphens. For Railway, you can use `${{RAILWAY_PROJECT_NAME}}-${{RAILWAY_ENVIRONMENT_NAME}}.railway`. |
| `TS_STATE_DIR`       |   Yes    | -                                                                 | Persistent storage directory. Required to prevent duplicate Tailscale machines on restart. |
| `TS_ENABLE_HTTPS`    |    No    | `false`                                                           | Enable HTTPS for every mapping when `TS_HTTPS_SERVICES` is unset. Use only with HTTP backends. |
| `TS_HTTPS_SERVICES`  |    No    | -                                                                 | Comma-separated service names to enable HTTPS. A nonempty list overrides `TS_ENABLE_HTTPS`. |
| `MAX_CONNECTIONS`    |    No    | `256`                                                             | Maximum concurrent TCP forwards per service. Must be greater than zero. Excess connections are closed. |
| `LOG_LEVEL`          |    No    | `info`                                                            | Log verbosity: `debug`, `info`, `warn`, or `error`.                                        |
| `HEALTH_ADDR`        |    No    | -                                                                 | If set (e.g. `:8080`), serves `/healthz` and `/readyz` probes on this address.             |
| `SERVICE_[n]`        |   Yes    | -                                                                 | Service mapping in format: `servicename:sourceport:targethost:targetport`                  |

Required values must be nonempty. `TS_STATE_DIR` must point to an existing directory.
Services can share a source port because each service has its own Tailscale node.
Services selected for HTTPS must use a TCP source port other than 80 or 443, which the HTTP redirect and HTTPS proxy reserve.

**Example Configuration (TCP Only):**

```bash
TS_AUTHKEY=tskey-auth-xxxxx
TS_HOSTNAME=my-project-production.railway
TS_STATE_DIR=/app/data
SERVICE_01=postgres:5432:${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:${{Postgres.PGPORT}}
SERVICE_02=redis:6379:${{Redis.RAILWAY_PRIVATE_DOMAIN}}:${{Redis.REDISPORT}}
SERVICE_03=api:80:${{WebServer.RAILWAY_PRIVATE_DOMAIN}}:${{WebServer.PORT}}
```

**Example Configuration (With HTTPS):**

```bash
TS_AUTHKEY=tskey-auth-xxxxx
TS_HOSTNAME=my-project-production.railway
TS_STATE_DIR=/app/data
TS_HTTPS_SERVICES=bullboard,api
SERVICE_01=bullboard:3000:${{Bullboard.RAILWAY_PRIVATE_DOMAIN}}:${{Bullboard.PORT}}
SERVICE_02=api:8080:${{API.RAILWAY_PRIVATE_DOMAIN}}:${{API.PORT}}
```

**Resulting Connection URLs:**

- **TCP Only**: `postgres-my-project-production-railway:5432`
- **With HTTPS**:
  - TCP: `bullboard-my-project-production-railway:3000`
  - HTTPS: `https://bullboard-my-project-production-railway.{your-tailnet}.ts.net/`

## Examples

Each service gets its own descriptive hostname:

#### PostgreSQL (TCP Only)

```bash
SERVICE_01=postgres:5432:${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:${{Postgres.PGPORT}}
```

Connect with: `postgresql://postgres:<password>@postgres-my-project-production-railway:5432/railway`

#### Redis (TCP Only)

```bash
SERVICE_02=redis:6379:${{Redis.RAILWAY_PRIVATE_DOMAIN}}:${{Redis.REDISPORT}}
```

Connect with: `redis://default:<password>@redis-my-project-production-railway:6379`

#### Web Service (HTTPS Enabled)

```bash
TS_HTTPS_SERVICES=bullboard
TS_STATE_DIR=/app/data
SERVICE_01=bullboard:3000:${{Bullboard.RAILWAY_PRIVATE_DOMAIN}}:${{Bullboard.PORT}}
```

Access via:

- **HTTPS**: `https://bullboard-my-project-production-railway.{your-tailnet}.ts.net/`
- **TCP**: `bullboard-my-project-production-railway:3000`

#### API Service (HTTPS Enabled)

```bash
TS_HTTPS_SERVICES=api
TS_STATE_DIR=/app/data
SERVICE_02=api:8080:${{API.RAILWAY_PRIVATE_DOMAIN}}:${{API.PORT}}
```

Access via: `https://api-my-project-production-railway.{your-tailnet}.ts.net/`

#### Mixed TCP and HTTPS Services

```bash
# Enable HTTPS for web services
TS_HTTPS_SERVICES=api,dashboard
TS_STATE_DIR=/app/data

# Mix of TCP and HTTPS-capable services
SERVICE_01=postgres:5432:${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:${{Postgres.PGPORT}}
SERVICE_02=redis:6379:${{Redis.RAILWAY_PRIVATE_DOMAIN}}:${{Redis.REDISPORT}}
SERVICE_03=api:8080:${{WebServer.RAILWAY_PRIVATE_DOMAIN}}:${{WebServer.PORT}}
SERVICE_04=dashboard:3000:${{Dashboard.RAILWAY_PRIVATE_DOMAIN}}:${{Dashboard.PORT}}
```

**Connection Options:**

- **PostgreSQL** (TCP): `postgresql://postgres:<password>@postgres-my-project-production-railway:5432/railway`
- **Redis** (TCP): `redis://default:<password>@redis-my-project-production-railway:6379`
- **API** (HTTPS): `https://api-my-project-production-railway.{your-tailnet}.ts.net/`
- **Dashboard** (HTTPS): `https://dashboard-my-project-production-railway.{your-tailnet}.ts.net/`

In this example, `TS_HTTPS_SERVICES` selects only `api` and `dashboard`. PostgreSQL and Redis remain TCP-only.
The application does not detect whether a backend supports HTTP.

## HTTPS with Automatic TLS Certificates

Enable HTTPS proxy mode to automatically provision TLS certificates for your services using Let's Encrypt via Tailscale.

### Features

- **Automatic certificate provisioning**: Uses Tailscale + Let's Encrypt integration
- **90-day certificate renewal**: Handled automatically by Tailscale
- **HTTP to HTTPS redirect**: HTTP requests automatically redirect to HTTPS
- **Reverse proxy**: Forwards HTTPS requests to your internal HTTP services
- **Browser-trusted certificates**: Valid certificates for all modern browsers

### Requirements

- **Persistent storage**: Set `TS_STATE_DIR` to a Railway volume mount path (e.g., `/app/data`)
- **Railway volume**: Mount a volume to persist Tailscale state, machine identity, and certificates
- **Tailnet DNS settings**: Enable MagicDNS and HTTPS certificates in the Tailscale admin console.
- **Certificate hostname**: Use the full `<machine>.<tailnet>.ts.net` hostname. The short hostname does not match the certificate.

Tailscale publishes certificate hostnames in public Certificate Transparency logs. Avoid sensitive information in service names and `TS_HOSTNAME`.
The proxy sends the node's full certificate hostname in the upstream `Host` and `X-Forwarded-Host` headers.
If your backend validates allowed hostnames, include that hostname in its configuration.

### Configuration

```bash
# Enable HTTPS with persistent storage
TS_STATE_DIR=/app/data          # Railway volume mount path
TS_HTTPS_SERVICES=service-name  # Select HTTP backends by service name

# Service will be available at both:
# - TCP: service-name.tailnet.ts.net:3000
# - HTTPS: https://service-name.tailnet.ts.net/
```

### Railway Volume Setup

1. **Add volume** to your Railway service (e.g., mount at `/app/data`)
2. **Set `TS_STATE_DIR=/app/data`** in your environment variables
3. **Select HTTPS services** with `TS_HTTPS_SERVICES=api,dashboard`.

### Container user and volume permissions

The container runs as root to write Tailscale state to Railway volumes, which are mounted as root.
Changing the image to a nonroot user can prevent startup when the state volume is not writable.
For nonroot images with this issue, Railway documents the service variable `RAILWAY_RUN_UID=0`.
For more information, see [Railway volume permissions](https://docs.railway.com/volumes#permissions).

### Certificate Storage

- **Certificates**: Stored in `TS_STATE_DIR/{service-name}/`
- **Tailscale state**: Node keys and network configuration
- **Automatic renewal**: Certificates renewed ~30 days before expiry

**Note**: Without persistent storage, certificates will be re-requested on every restart, which may hit Let's Encrypt rate limits.

## Tags and ACLs

You can use Tailscale tags to organize your services and apply ACL policies. Tags help you:

- **Group related services**: Tag all database services with `tag:database`
- **Apply environment-specific rules**: Use `tag:production` or `tag:staging`
- **Control access**: Set up ACLs to allow specific users/devices to access tagged services

Because every machine here is brought up from a reusable auth key, **tags are attached to the auth key, not to this app**. Assign the tags you want when you generate the key (Tailscale admin console → Settings → Keys → "Add tags"), and every machine this forwarder creates with that key inherits them. There is no per-service tag setting in the app itself.

Authorization to reach a service is enforced entirely by your tailnet ACLs against those tags, so make sure the auth key's tags map to grants that only expose each service to the intended peers.

**Note**: You must be listed as a "TagOwner" in your Tailscale ACL to apply tags. See [Tailscale ACL documentation](https://tailscale.com/kb/1337/acl-syntax) for more details.

## Health and shutdown

Set `HEALTH_ADDR=:8080` to expose `/healthz` and `/readyz` on the container network.
Keep this port private or use a loopback address when external probes are unnecessary.
`/readyz` returns 200 after every service binds its listeners and returns 503 during shutdown.
Readiness does not test backend availability. Failed HTTPS upstream requests return 502.

On SIGINT or SIGTERM, the forwarder stops accepting TCP connections and gives existing connections up to 30 seconds to finish.
After that deadline, it closes the remaining connections. Configure the deployment platform's shutdown window to allow this drain period.
TCP half-closes preserve responses until both directions finish or shutdown forces closure.

## Development and release checks

Use Go 1.27.2 or later. The Go version and dependencies are declared in `go.mod`.

```bash
go test -race -count=1 -cover ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
go mod tidy -diff
docker build -t tailscale-forwarder .
```

CI runs these checks for pull requests, pushes to `main`, and a weekly schedule.
Release publishing depends on CI passing and includes build provenance and an SBOM.
Dependabot proposes updates for Go modules, GitHub Actions, and Docker base images.
The container includes dependency license notices in `/usr/share/doc/tailscale-forwarder/THIRD_PARTY_NOTICES.txt`.
