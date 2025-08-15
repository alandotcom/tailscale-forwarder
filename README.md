## Tailscale Forwarder

Tailscale Forwarder is a TCP proxy that allows you to connect through a Tailscale machine to the configured target address and port pair.

This allows you to connect to Railway services that are not accessible from the internet, for example, locking down access to your database to only those who are on your Tailscale network.

This also solves for the issue that you can only run one Tailscale subnet router per Tailscale account.

## Usage

1. Generate a Tailscale [auth key](https://tailscale.com/kb/1085/auth-keys).

   Make sure `Reusable` is enabled.

2. Enable [MagicDNS](https://tailscale.com/kb/1081/magicdns) for your Tailscale account.

   This is required so that your computer can resolve the Tailscale Forwarder machine's short hostname to the correct IP address.   

3. Deploy the Tailscale Forwarder service into your pre-existing Railway project.

   Set the `TS_AUTHKEY` environment variable to the auth key you generated in step 1.

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

| Environment Variable | Required | Default Value | Description |
| -------------------- | :------: | ------------- | ----------- |
| `TS_AUTHKEY`         | Yes      | -             | Tailscale auth key. |
| `TS_HOSTNAME`        | Yes      | `${{RAILWAY_PROJECT_NAME}}-${{RAILWAY_ENVIRONMENT_NAME}}.railway` | Base hostname for services. Note: dots will be converted to hyphens in final hostname. |
| `TS_EXTRA_ARGS`      | No       | -             | Additional Tailscale arguments (e.g., `--advertise-tags=tag:database,tag:production`). |
| `TS_STATE_DIR`       | No*      | -             | Persistent storage directory. Required when `TS_ENABLE_HTTPS=true`. |
| `TS_ENABLE_HTTPS`    | No       | `false`       | Enable HTTPS proxy with automatic TLS certificates. |
| `SERVICE_[n]`        | Yes      | -             | Service mapping in format: `servicename:sourceport:targethost:targetport` |

**Example Configuration (TCP Only):**
```bash
TS_AUTHKEY=tskey-auth-xxxxx
TS_HOSTNAME=my-project-production.railway
TS_EXTRA_ARGS=--advertise-tags=tag:database,tag:production
SERVICE_01=postgres:5432:${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:${{Postgres.PGPORT}}
SERVICE_02=redis:6379:${{Redis.RAILWAY_PRIVATE_DOMAIN}}:${{Redis.REDISPORT}}
SERVICE_03=api:80:${{WebServer.RAILWAY_PRIVATE_DOMAIN}}:${{WebServer.PORT}}
```

**Example Configuration (With HTTPS):**
```bash
TS_AUTHKEY=tskey-auth-xxxxx
TS_HOSTNAME=my-project-production.railway
TS_STATE_DIR=/app/data
TS_ENABLE_HTTPS=true
TS_EXTRA_ARGS=--advertise-tags=tag:web,tag:production
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
TS_ENABLE_HTTPS=true
TS_STATE_DIR=/app/data
SERVICE_01=bullboard:3000:${{Bullboard.RAILWAY_PRIVATE_DOMAIN}}:${{Bullboard.PORT}}
```
Access via:
- **HTTPS**: `https://bullboard-my-project-production-railway.{your-tailnet}.ts.net/`
- **TCP**: `bullboard-my-project-production-railway:3000`

#### API Service (HTTPS Enabled)
```bash
TS_ENABLE_HTTPS=true
TS_STATE_DIR=/app/data
SERVICE_02=api:8080:${{API.RAILWAY_PRIVATE_DOMAIN}}:${{API.PORT}}
```
Access via: `https://api-my-project-production-railway.{your-tailnet}.ts.net/`

#### Mixed TCP and HTTPS Services
```bash
# Enable HTTPS for web services
TS_ENABLE_HTTPS=true
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

**Note**: When HTTPS is enabled, web services get both TCP and HTTPS access, while database services remain TCP-only.

Each service gets its own clear, descriptive hostname that immediately tells you what you're connecting to!

## HTTPS with Automatic TLS Certificates

Enable HTTPS proxy mode to automatically provision TLS certificates for your services using Let's Encrypt via Tailscale.

### Features:
- **Automatic certificate provisioning**: Uses Tailscale + Let's Encrypt integration
- **90-day certificate renewal**: Handled automatically by Tailscale
- **HTTP to HTTPS redirect**: HTTP requests automatically redirect to HTTPS
- **Reverse proxy**: Forwards HTTPS requests to your internal HTTP services
- **Browser-trusted certificates**: Valid certificates for all modern browsers

### Requirements:
- **Persistent storage**: Set `TS_STATE_DIR` to a Railway volume mount path (e.g., `/app/data`)
- **Railway volume**: Mount a volume to persist Tailscale state and certificates

### Configuration:
```bash
# Enable HTTPS with persistent storage
TS_STATE_DIR=/app/data          # Railway volume mount path
TS_ENABLE_HTTPS=true            # Enable HTTPS proxy

# Service will be available at both:
# - TCP: service-name.tailnet.ts.net:3000
# - HTTPS: https://service-name.tailnet.ts.net/
```

### Railway Volume Setup:
1. **Add volume** to your Railway service (e.g., mount at `/app/data`)
2. **Set `TS_STATE_DIR=/app/data`** in your environment variables
3. **Enable HTTPS** with `TS_ENABLE_HTTPS=true`

### Certificate Storage:
- **Certificates**: Stored in `TS_STATE_DIR/{service-name}/`
- **Tailscale state**: Node keys and network configuration
- **Automatic renewal**: Certificates renewed ~30 days before expiry

**Note**: Without persistent storage, certificates will be re-requested on every restart, which may hit Let's Encrypt rate limits.

## Tags and ACLs

You can use Tailscale tags to organize your services and apply ACL policies. Tags help you:

- **Group related services**: Tag all database services with `tag:database`
- **Apply environment-specific rules**: Use `tag:production` or `tag:staging`  
- **Control access**: Set up ACLs to allow specific users/devices to access tagged services
- **Auto-approve routes**: Configure ACLs to automatically approve subnet routes for tagged nodes

**Example tag configurations:**

```bash
# Tag all services as databases in production
TS_EXTRA_ARGS=--advertise-tags=tag:database,tag:production

# Tag services by type and environment  
TS_EXTRA_ARGS=--advertise-tags=tag:cache,tag:staging

# Multiple arguments supported
TS_EXTRA_ARGS=--advertise-tags=tag:web,tag:frontend --accept-routes
```

**Note**: You must be listed as a "TagOwner" in your Tailscale ACL to apply tags. See [Tailscale ACL documentation](https://tailscale.com/kb/1337/acl-syntax) for more details.