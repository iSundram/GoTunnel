GoTunnel — prompt.md

> Comprehensive product prompt & spec for GoTunnel (backend-only tunnel server in Go). This file is intended to be a single-source spec you can paste into a repo, prompt an LLM, or use as a developer-facing reference. No roadmap included.




---

Overview

GoTunnel is a backend-first reverse-tunnel service written in Go. It exposes local services to the public Internet by creating persistent, authenticated, and encrypted reverse tunnels between a client binary and a cluster of gateway servers. GoTunnel supports HTTP(S) and raw TCP, custom domains, config-driven operation, advanced security controls, and production-grade telemetry.

Primary goals:

Simple config-driven operation (/etc/gotunnel/config.yml, env vars, CLI flags)

High-concurrency performance using idiomatic Go

Secure (TLS, token auth, optional mTLS)

Rich runtime settings (rate limits, bandwidth caps, TTLs)

Extensible storage/backing (in-memory, Redis, Postgres)

Minimal backend only: no UI/dashboard required



---

Key Terms

Client — local process that opens a persistent outbound connection to GoTunnel gateway.

Gateway — public-facing server(s) that accept incoming HTTP/TCP and forward to Clients.

Tunnel — an active mapping between a public endpoint (subdomain or port) and a client local port.

Registrar — service or module that allocates/validates subdomains and stores tunnel metadata.

Broker — module that matches incoming public requests to the appropriate client connection.

Control channel — control frames/messages exchanged between client and gateway for setup, heartbeat, and management.

Data channel — bidirectional streams carrying proxied traffic.



---

Full Feature List

Core features (MUST)

Persistent outbound connections from client → gateway (TLS + optional mTLS)

Subdomain allocation and DNS mapping (random & custom)

HTTP(S) reverse-proxying (Host header routing)

Raw TCP forwarding (public port → client local port)

Config file support (YAML, TOML, JSON) and env var overrides

CLI flags for quick overrides and transient tunnels

Heartbeat + reconnect logic

In-memory tunnel registry for MVP; pluggable Redis/Postgres backends for production

Automatic Let's Encrypt integration for *.domain or ACME via certificate manager

Graceful shutdown and cleanup


Authentication & access control

API tokens / API keys (revocable)

Optional JWT-based authentication

Optional mutual TLS (client certs)

Per-tunnel password/basic-auth

IP allowlist / denylist

Rate limiting (connections and requests)

Per-user and per-tunnel bandwidth limiting and connection caps

Token scopes (create, list, revoke, admin)


Security & privacy

TLS everywhere (client ↔ gateway; gateway ↔ end user)

HMAC signature validation for client registration frames (anti-replay)

Per-tunnel encryption for payloads (optional layered encryption)

Abuse detection (connection/frequency anomalies)

Blacklisting and auto-block for suspicious clients

Audit logging for control actions


Protocol & transport

Control channel: WebSocket over TLS (recommended) or framed TCP over TLS

Data channels: multiplexed streams (using simple framing or HTTP/2 / gRPC streams)

Framing options: length-prefixed protobuf frames or custom binary frames

Heartbeat frames + TTL-based session expiration


Developer & operational features

Config file (/etc/gotunnel/config.yml) with hierarchical overrides

Environment variable configuration

Full CLI: start, register, status, list, stop, token management

JSON output and machine-readable logs

Prometheus-compatible /metrics endpoint

Structured logs (JSON)

Health check endpoints and readiness/liveness

Docker image and systemd unit examples

Integration hooks: webhooks for tunnel events (create, delete, error)

SDKs: minimal client library in Go (and optional Node/Python wrappers)

Plugin hooks for storage, auth, and certificate management


Performance & scale

Zero-copy streaming where possible, efficient buffer reuse (sync.Pool)

High-concurrency support with goroutines

Horizontal scale: multiple gateways with shared Redis for state

Sticky routing or session affinity when necessary for stateful clients

Connection pooling and keep-alive tuning

Optional QUIC/HTTP3 experimental support


Observability & troubleshooting

Request/response logging per-tunnel (configurable retention)

Live request inspection (for debugging, via control channel)

Per-tunnel metrics: requests, bytes in/out, active streams, errors

Audit logs: token usage, admin actions

Debug endpoints for active connections and registry state


Admin & multi-tenant

Multi-user support (users, teams)

Per-user plan/limit enforcement

Revocation endpoints for tokens and tunnels

Reserved subdomains list (global)

Admin API endpoints for management



---

Configuration (etcs)

GoTunnel supports config precedence: CLI flags > environment variables > config file.

Default config file path: /etc/gotunnel/config.yml

Example YAML (complete)

# /etc/gotunnel/config.yml
server:
  listen_http: ":80"               # optional, used only for ACME redirect
  listen_https: ":443"
  gateway_host: "0.0.0.0"
  public_hostname: "gotunnel.example.com"
  allowed_domains:
    - "gotunnel.example.com"
  tls:
    enabled: true
    auto_lets_encrypt: true
    cert_dir: "/var/lib/gotunnel/certs"
    acme:
      email: "ops@example.com"
      staging: false

auth:
  token_signing_key: "base64-encoded-secret"
  token_ttl_minutes: 525600 # one year
  enable_jwt: true
  jwt_issuer: "gotunnel"
  jwt_audience: "gotunnel-clients"

storage:
  type: "redis"   # options: memory | redis | postgres
  redis:
    addr: "127.0.0.1:6379"
    db: 0
    password: ""
  postgres:
    dsn: "postgres://user:pass@127.0.0.1:5432/gotunnel?sslmode=disable"

registry:
  reserved_subdomains:
    - "admin"
    - "api"
  default_ttl_seconds: 3600
  max_subdomain_length: 32

limits:
  max_tunnels_per_user: 10
  max_bandwidth_per_tunnel_mbps: 100
  max_concurrent_streams_per_tunnel: 256
  request_rate_per_ip_per_min: 600

logging:
  level: "info"
  format: "json"
  file: "/var/log/gotunnel/gotunnel.log"

metrics:
  enabled: true
  bind_addr: ":9090"

security:
  ip_blacklist: []
  ip_whitelist: []
  enable_mtls: false
  mtls:
    ca_cert_file: "/etc/gotunnel/ca.pem"

Example TOML

[server]
listen_https = ":443"
public_hostname = "gotunnel.example.com"

[auth]
token_signing_key = "base64..."
enable_jwt = true

[storage]
type = "memory"

Example JSON

{
  "server": {"public_hostname": "gotunnel.example.com"},
  "storage": {"type": "redis", "redis": {"addr": "127.0.0.1:6379"}}
}


---

CLI interface & flags

gotunnel server --config /etc/gotunnel/config.yml — start server

gotunnel client --config ~/.gotunnel/client.yml — client mode

gotunnel token create --user-id USER --scopes create,list — create API token

gotunnel tunnel start --port 3000 --subdomain myapp — ad-hoc ephemeral registration

gotunnel tunnel list --format json — list active tunnels

gotunnel install --systemd — prints systemd unit


Common flags:

--config path to config

--log-level info|warn|debug|error

--env read config from environment

--daemon run in background

--output json machine parseable output



---

Environment variables

GOTUNNEL_CONFIG — path to config file

GOTUNNEL_LOG_LEVEL — override log level

GOTUNNEL_STORAGE_TYPE — memory|redis|postgres

GOTUNNEL_REDIS_ADDR — redis addr

GOTUNNEL_AUTH_KEY — token signing key

PORT — fallback server port



---

Client config (example: ~/.gotunnel/client.yml)

client:
  gateway_url: "wss://gateway.gotunnel.example.com/connect"
  token: "user-api-token"
  local_bind: "127.0.0.1"
  tunnels:
    - name: "app"
      port: 3000
      protocol: "http"
      subdomain: "myapp"
    - name: "ssh"
      port: 22
      protocol: "tcp"
  reconnect:
    max_retries: 0        # 0 = unlimited
    backoff_ms: 1000
  heartbeat_seconds: 10
  tls:
    skip_verify: false
    ca_file: "/etc/ssl/certs/ca-certificates.crt"

Default client behavior: on gotunnel client start, it registers each listed tunnel and then waits for incoming connections forwarded from the gateway. The client must implement auto-reconnect and local service health checks.


---

Protocol specification (control + data)

Transport recommendation: WebSocket over TLS using a binary protocol with protobuf frames (or JSON for simpler MVP). Use a small framing protocol:

Frame layout (binary):

1 byte: frame type (1=register, 2=heartbeat, 3=open_stream, 4=stream_data, 5=close_stream, 6=control_response)

4 bytes: big-endian uint32 length of payload

N bytes: payload (protobuf/JSON)


Control messages (register example):

{
  "type": "register",
  "token": "api-token",
  "tunnels": [
    {"name": "app", "protocol": "http", "port": 3000, "subdomain": "myapp"}
  ],
  "client_id": "uuid-v4",
  "metadata": {"os":"linux","version":"0.1.0"}
}

Heartbeat:

{"type":"heartbeat", "timestamp": 1670000000}

Open_stream:

{
  "type": "open_stream",
  "stream_id": 42,
  "target_tunnel": "myapp",
  "metadata": {"method":"GET", "path":"/"}
}

Stream data frames carry raw bytes associated with stream_id. Both sides maintain stream_id mapping.

Error codes (sample):

1000 OK / normal close

2001 Unauthorized

2002 Invalid request

3001 Tunnel not found

3002 Stream limit reached

4001 Internal server error



---

Data models & sample Go structs

type Tunnel struct {
    ID              string    `json:"id"`
    OwnerID         string    `json:"owner_id"`
    Subdomain       string    `json:"subdomain"`
    Protocol        string    `json:"protocol"` // "http" | "tcp"
    TargetPort      int       `json:"target_port"`
    CreatedAt       time.Time `json:"created_at"`
    LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
    ConnID          string    `json:"conn_id"`
    TTL             int       `json:"ttl_seconds"`
    BandwidthLimit  int       `json:"bandwidth_limit_mbps"`
}

Redis schema suggestion:

gotunnel:tunnel:<id> → hash of tunnel meta

gotunnel:subdomain:<domain> → tunnel id

gotunnel:user:<user_id>:tunnels → set of tunnel ids

gotunnel:token:<token> → token meta (user_id, scopes, expiry)


Postgres schema suggestion: tunnels table with columns matching the Tunnel struct.


---

Storage & persistence

MVP: in-memory map protected by sync.RWMutex or sync.Map (fast but ephemeral).

Production: Redis for active session store; Postgres for durable records (users, tokens, audit logs).

Use TTL expiration in Redis for ephemeral tunnels.

Use optimistic locking or Lua scripts for atomic subdomain allocation in Redis.



---

TLS & certificate management

Option A: Use built-in ACME client to get wildcard or per-subdomain certificates (if DNS validation supported).

Option B: Integrate with an existing cert manager (cert-manager in k8s) and watch /var/lib/gotunnel/certs.

Option C: Provide tls.cert_file and tls.key_file config settings for pre-provisioned certs.

Support Let's Encrypt staging flag for testing.



---

Rate limiting & bandwidth controls

Defaults (configurable):

max_tunnels_per_user: 10

max_concurrent_streams_per_tunnel: 256

request_rate_per_ip_per_min: 600

max_bandwidth_per_tunnel_mbps: 100


Implementation:

Request rate: token-bucket (golang.org/x/time/rate) per IP and per tunnel

Bandwidth: simple per-tunnel counters + leaky-bucket enforcement; for heavy use, consider kernel-level shaping on edge nodes



---

Logging & metrics

Logging:

JSON structured logs, fields: timestamp, level, component, correlation_id, user_id, tunnel_id, message

Example: {"ts":"2026-03-02T12:00:00Z","level":"info","component":"proxy","tunnel_id":"abc","msg":"stream opened"}


Metrics (Prometheus names):

gotunnel_active_tunnels_total

gotunnel_streams_active

gotunnel_bytes_sent_total

gotunnel_bytes_recv_total

gotunnel_requests_total{code,method}

gotunnel_token_auth_failures_total

gotunnel_reconnects_total


Expose /metrics for Prometheus scraping.


---

Systemd & Docker snippets

systemd unit example

[Unit]
Description=GoTunnel Gateway
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/gotunnel server --config /etc/gotunnel/config.yml
Restart=on-failure
User=gotunnel
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target

docker-compose (minimal)

version: "3.7"
services:
  gotunnel:
    image: yourrepo/gotunnel:latest
    restart: unless-stopped
    volumes:
      - /etc/gotunnel:/etc/gotunnel
      - /var/lib/gotunnel/certs:/certs
    environment:
      - GOTUNNEL_CONFIG=/etc/gotunnel/config.yml
    ports:
      - "80:80"
      - "443:443"
      - "9090:9090" # metrics


---

Admin & management API (REST)

Auth

All admin endpoints require bearer token with admin scope.


Endpoints (examples)

POST /api/v1/tokens — create token

GET  /api/v1/tunnels — list tunnels (query by user)

DELETE /api/v1/tunnels/{id} — delete tunnel

GET /api/v1/health — service health

GET /metrics — Prometheus metrics


All responses: JSON with code, message, data.


---

Error handling & response codes

Client should handle control-channel errors gracefully and attempt reconnect when recoverable.

Use standard HTTP codes for public HTTP forwarding.

Provide explicit control-channel error frames for fatal errors (e.g., unauthorized, quota_exceeded, invalid_subdomain).



---

Testing & QA

Unit tests for registry, auth, and proxy routing.

Integration tests: client ↔ gateway ↔ local server (end-to-end).

Load tests: thousands of concurrent tunnels; measure memory and goroutine count.

Security tests: token revocation, replay protections, TLS validation.

Chaos tests: simulate network partition, client crashes, and reconnect behavior.



---

Troubleshooting & common issues

Client cannot register: check token, gateway URL, TLS certs, and firewall (outbound).

Requests failing to reach local service: confirm client is up, registered, correct port mapping, and no local firewall blocking.

Subdomain collision: reserved subdomains list or TTL expiry not cleared — clear registry entry.

Certificate errors: ensure certs exist and are readable; check ACME logs.

High latency: check network path to gateway, CPU saturation, goroutine leaks.



---

Example usage snippets

Start server (system):

sudo gotunnel server --config /etc/gotunnel/config.yml

Start client (ad-hoc):

gotunnel client --gateway wss://gateway.gotunnel.example.com/connect \
  --token "USER_TOKEN" \
  --tunnel-port 3000 --tunnel-subdomain "myapp"

Register multiple tunnels from client config:

gotunnel client --config ~/.gotunnel/client.yml

Programmatic registration (control frame example in Go pseudocode):

reg := Register{
  Token: "user-token",
  Tunnels: []TunnelRequest{
    {Name: "app", Protocol: "http", Port: 3000, Subdomain: "myapp"},
  },
}
sendControlFrame(conn, FrameTypeRegister, reg)


---

Security checklist (minimum)

Use TLS with valid certs for client ↔ gateway

Issue per-user API tokens and rotate them regularly

Enforce per-user/tunnel limits

Log and monitor authentication failures

Use Redis/Postgres with authentication and restricted network access

Run gateways behind a firewall or in VPC with minimal ingress paths

Periodically audit reserved subdomains and token usage



---

Extensibility notes (no roadmap content)

Auth, storage, and certificate managers are pluggable interfaces — implementers can swap Redis for Memcached or Postgres for any SQL store.

Protocol frames designed to be forward-compatible: include version field in control frames to allow protocol upgrades.

Add-ons: plugin hooks for custom authentication (OAuth), custom event listeners (webhooks), or specialized load-balancing hooks.



---

Packaging & release notes (packaging checklist)

Provide a static-linked Linux binary (CGO disabled where possible)

Provide container image with minimal base (scratch or distroless)

Provide deb/rpm packaging for system integration

Provide public API docs and OpenAPI spec for management endpoints

Publish Prometheus metrics schema and sample Grafana dashboard JSON



---

License & compliance

Recommend using a permissive license (Apache-2.0 or MIT) for the core library and binaries.

Document third-party libraries and their licenses.

For enterprise features (if later added), consider dual licensing or separate enterprise repo.



---

Epilogue — single-file prompt usage

Use this prompt.md as the authoritative product brief for:

generating code prototypes,

writing API docs,

building tests,

prompting other contributors or LLMs to implement components of GoTunnel.


If you want, I can now:

produce a minimal Go prototype for the server registration + WebSocket control channel and a matching client,

or generate OpenAPI spec for the admin endpoints,

or give the exact protobuf schema for control/data frames.


(You asked for no roadmap — none included.)

