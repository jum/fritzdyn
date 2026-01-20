# fritzdyn

Fritzdyn is go server for the AVM FritzBox dynamic DNS update protocol. The program needs to run
on a computer with a fixed IP so it is always reachable. It can be run as a CGI program (if you
have only a few fritzboxes), or as a permantly running server.

## Admin Interface

Fritzdyn includes a web-based administration interface accessible at `/admin/`. This interface allows you to:

*   View all configured hosts.
*   Add new hosts (auto-generating tokens).
*   Edit existing hosts.
*   Configure "Update Methods" for each host (e.g., triggering a `GET` request or updating Cloudflare DNS records).

## Database Structure

The application uses a SQL database (sqlite3 by default) with the following structure:

### `hosts` Table
Stores the Dynamic DNS records.
*   `token`: Unique identifier (UUID) used by the FritzBox to authenticate.
*   `name`: specific name for the host.
*   `domain`: The full domain name (e.g., `vpn.example.com`).
*   `zone`: The DNS zone (e.g., `example.com`).
*   `ip4addr`, `ip6addr`: The current IP addresses.

### `updates` Table
Stores actions to perform when a host's IP address changes.
*   `token`: Foreign key linking to the `hosts` table.
*   `cmd`: The action type. Supported values:
    *   `GET`: Performs an HTTP GET request to the URL specified in `args`.
    *   `cloudflare`: Updates Cloudflare DNS records (requires `api_key` to point to an environment variable containing the CF token).
    *   Shell command: Any other value is treated as a shell command to execute.
*   `args`: Arguments for the command or the URL for `GET`.
*   `api_key`: Name of the environment variable containing the API key (for Cloudflare).

## Security (Caddy & Basic Auth)

Since the `/admin` interface allows modifying your DNS configuration, it **must** be secured. Below is an example of how to configure Caddy to protect the `/admin` endpoint with Basic Authentication.

### Generating a Password Hash
First, generate a hashed password using `caddy hash-password`:
```bash
caddy hash-password --plaintext "your_secure_password"
```

### Docker Compose Example (with Caddy Labels)

This example uses `lucaslorentz/caddy-docker-proxy` labels to configure Caddy. It sets up Basic Auth for the `/admin` path.

```yaml
name: fritzdyn
services:
  fritzdyn:
    container_name: fritzdyn
    restart: always
    image: jumager/fritzdyn:master
    volumes:
      - /run/containers:/run/containers
      - ./data:/data
    environment:
      SQL_DRIVER: sqlite
      SQL_DSN: /data/fritzdyn.sqlite3?_journal_mode=WAL&_fk=true
      NODE_ENV: production
      PORT: /run/containers/fritzdyn.sock
      # Add your Cloudflare API Token here if using the cloudflare update method
      CF_API_TOKEN: "your_cloudflare_api_token"
    labels:
     caddy: fritzdyn.example.org
     caddy.tls: admin@example.org
     caddy.tls.dns: cloudflare {env.CF_API_KEY}
     caddy.import: norobots
     caddy.skip_log: /health
     
     # Secure /admin endpoint
     caddy.@admin.path: "/admin*"
     caddy.basicauth: "@admin"
     # User "admin" with hashed password (replace with your own hash)
     caddy.basicauth.admin: "JDJhJDE0JH...." 

     caddy.reverse_proxy: "unix//run/containers/fritzdyn.sock"
networks:
  default:
    name: caddy
    external: true
```

### Caddyfile Example (Manual Configuration)

If you are running Caddy manually with a `Caddyfile`:

```caddyfile
fritzdyn.example.org {
    # Protect /admin path
    basicauth /admin* {
        admin JDJhJDE0JH.... 
    }

    reverse_proxy unix//run/containers/fritzdyn.sock
}
```

## Running as CGI

```
		cgi /cgi-bin/fritzdyn.cgi /usr/lib/cgi-bin/fritzdyn.cgi {
			env NODE_ENV=development SQL_DRIVER=sqlite3 SQL_DSN=/var/lib/fritzdyn/fritzdyn.sqlite3?_fk=true&_journal=WAL
		}
```