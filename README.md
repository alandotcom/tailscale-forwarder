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

   You should see multiple machines in the Tailscale [dashboard](https://login.tailscale.com/admin/machines) with the format `<Service Name>.<Base Hostname>`.
   
   Each service gets its own descriptive hostname.

5. Use the service-specific hostname to connect.

   Example: `postgresql://postgres:<Postgres Password>@postgres.my-project-production.railway:5432/railway`

   Each service has a clear, descriptive hostname that tells you exactly what you're connecting to.

## Configuration

| Environment Variable | Required | Default Value | Description |
| -------------------- | :------: | ------------- | ----------- |
| `TS_AUTHKEY`         | Yes      | -             | Tailscale auth key. |
| `TS_HOSTNAME`        | Yes      | `${{RAILWAY_PROJECT_NAME}}-${{RAILWAY_ENVIRONMENT_NAME}}.railway` | Base hostname domain for services. |
| `TS_EXTRA_ARGS`      | No       | -             | Additional Tailscale arguments (e.g., `--advertise-tags=tag:database,tag:production`). |
| `SERVICE_[n]`        | Yes      | -             | Service mapping in format: `servicename:sourceport:targethost:targetport` |

**Example Configuration:**
```bash
TS_AUTHKEY=tskey-auth-xxxxx
TS_HOSTNAME=my-project-production.railway
TS_EXTRA_ARGS=--advertise-tags=tag:database,tag:production
SERVICE_01=postgres:5432:${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:${{Postgres.PGPORT}}
SERVICE_02=redis:6379:${{Redis.RAILWAY_PRIVATE_DOMAIN}}:${{Redis.REDISPORT}}
SERVICE_03=api:80:${{WebServer.RAILWAY_PRIVATE_DOMAIN}}:${{WebServer.PORT}}
```

**Resulting Connection URLs:**
- PostgreSQL: `postgres.my-project-production.railway:5432`
- Redis: `redis.my-project-production.railway:6379`
- API: `api.my-project-production.railway:80`

## Examples

Each service gets its own descriptive hostname:

#### PostgreSQL
```bash
SERVICE_01=postgres:5432:${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:${{Postgres.PGPORT}}
```
Connect with: `postgresql://postgres:<password>@postgres.my-project-production.railway:5432/railway`

#### Redis
```bash
SERVICE_02=redis:6379:${{Redis.RAILWAY_PRIVATE_DOMAIN}}:${{Redis.REDISPORT}}
```
Connect with: `redis://default:<password>@redis.my-project-production.railway:6379`

#### ClickHouse
```bash
SERVICE_03=clickhouse:8123:${{ClickHouse.RAILWAY_PRIVATE_DOMAIN}}:${{ClickHouse.PORT}}
```
Connect with: `http://clickhouse:<password>@clickhouse.my-project-production.railway:8123/railway`

#### Multiple Services
```bash
SERVICE_01=postgres:5432:${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:${{Postgres.PGPORT}}
SERVICE_02=redis:6379:${{Redis.RAILWAY_PRIVATE_DOMAIN}}:${{Redis.REDISPORT}}
SERVICE_03=api:80:${{WebServer.RAILWAY_PRIVATE_DOMAIN}}:${{WebServer.PORT}}
SERVICE_04=clickhouse:8123:${{ClickHouse.RAILWAY_PRIVATE_DOMAIN}}:${{ClickHouse.PORT}}
```

Then you can connect to each service using its descriptive hostname:

- **PostgreSQL**: `postgresql://postgres:<password>@postgres.my-project-production.railway:5432/railway`
- **Redis**: `redis://default:<password>@redis.my-project-production.railway:6379`
- **API**: `http://api.my-project-production.railway:80`
- **ClickHouse**: `http://clickhouse:<password>@clickhouse.my-project-production.railway:8123/railway`

Each service gets its own clear, descriptive hostname that immediately tells you what you're connecting to!

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