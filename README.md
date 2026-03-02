# GoTunnel

A backend-first reverse-tunnel service written in Go. Expose local services to the public internet by creating persistent, authenticated, and encrypted reverse tunnels between a client binary and gateway servers.

## Features

- **HTTP(S) and TCP tunneling** — reverse-proxy HTTP requests via subdomain routing, or forward raw TCP
- **Config-driven** — YAML config file with environment variable and CLI flag overrides
- **Secure** — TLS everywhere, HMAC-signed API tokens, optional mTLS
- **Observable** — Prometheus metrics, structured JSON logging, health endpoints
- **Scalable** — high-concurrency with goroutines; pluggable storage backends (in-memory, Redis, Postgres)
- **Admin API** — RESTful management of tokens and tunnels

## Quick Start

### Build

```bash
go build -o gotunnel ./cmd/gotunnel
```

### Start the Server

```bash
# Using config file
./gotunnel server --config config.example.yml

# Or with environment variables
GOTUNNEL_AUTH_KEY=my-secret ./gotunnel server
```

### Start a Client

```bash
# Using config file
./gotunnel client --config client.example.yml

# Or ad-hoc (requires a valid API token)
./gotunnel client --config client.example.yml
```

### Create an API Token

Use the admin API once the server is running:

```bash
curl -X POST http://localhost/api/v1/tokens \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user1", "scopes": ["create", "list"], "ttl_minutes": 525600}'
```

## Architecture

```
┌─────────┐     HTTPS/WSS     ┌──────────────┐     HTTP     ┌─────────────┐
│  Client  │ ◄──────────────► │   Gateway    │ ◄──────────► │  End Users  │
│ (local)  │   Control+Data   │   Server     │   Proxied    │  (public)   │
└─────────┘     Channels      └──────────────┘   Requests   └─────────────┘
                                     │
                              ┌──────┴──────┐
                              │  Registry   │
                              │  (Memory /  │
                              │  Redis /    │
                              │  Postgres)  │
                              └─────────────┘
```

### Components

| Component | Description |
|-----------|-------------|
| **Config** | YAML/env/CLI configuration with hierarchical overrides |
| **Protocol** | Binary framing protocol (1-byte type + 4-byte length + payload) |
| **Registry** | Tunnel metadata storage with subdomain allocation |
| **Auth** | HMAC-SHA256 signed API tokens with scopes |
| **Broker** | Routes incoming requests to client WebSocket connections |
| **Proxy** | HTTP reverse proxy with Host header subdomain routing |
| **Admin** | REST API for token and tunnel management |
| **Metrics** | Prometheus-compatible metrics endpoint |

## Configuration

### Server Config (`config.example.yml`)

See [`config.example.yml`](config.example.yml) for a complete example.

### Client Config (`client.example.yml`)

See [`client.example.yml`](client.example.yml) for a complete example.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `GOTUNNEL_CONFIG` | Path to config file |
| `GOTUNNEL_LOG_LEVEL` | Override log level (debug/info/warn/error) |
| `GOTUNNEL_STORAGE_TYPE` | Storage backend (memory/redis/postgres) |
| `GOTUNNEL_REDIS_ADDR` | Redis address |
| `GOTUNNEL_AUTH_KEY` | Token signing key |
| `PORT` | Fallback server port |

## CLI

```
gotunnel server  --config <path>  --log-level <level>   Start gateway server
gotunnel client  --config <path>  --log-level <level>   Start tunnel client
gotunnel token   create|list|revoke                      Token management (planned)
gotunnel tunnel  start|list|stop                         Tunnel management (planned)
gotunnel version                                         Print version
```

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | None | Health check |
| `GET` | `/metrics` | None | Prometheus metrics |
| `POST` | `/api/v1/tokens` | Admin | Create API token |
| `GET` | `/api/v1/tunnels` | Admin | List tunnels |
| `DELETE` | `/api/v1/tunnels/{id}` | Admin | Delete tunnel |
| `GET` | `/connect` | Token | WebSocket endpoint for clients |

## Protocol

Binary frame format:

```
┌──────────┬──────────────────┬─────────────────┐
│ Type (1B)│ Length (4B, BE)   │ Payload (N bytes)│
└──────────┴──────────────────┴─────────────────┘
```

Frame types: `1`=Register, `2`=Heartbeat, `3`=OpenStream, `4`=StreamData, `5`=CloseStream, `6`=ControlResponse

## Deployment

### Docker

```bash
docker-compose up -d
```

### Systemd

```bash
sudo cp gotunnel /usr/local/bin/
sudo cp gotunnel.service /etc/systemd/system/
sudo cp config.example.yml /etc/gotunnel/config.yml
sudo systemctl enable --now gotunnel
```

## Development

```bash
# Build
go build -o gotunnel ./cmd/gotunnel

# Test
go test ./...

# Vet
go vet ./...
```

## License

Apache-2.0
