package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Ensure env var doesn't interfere
	os.Unsetenv("GOTUNNEL_CONFIG")
	os.Unsetenv("GOTUNNEL_LOG_LEVEL")
	os.Unsetenv("GOTUNNEL_STORAGE_TYPE")
	os.Unsetenv("GOTUNNEL_REDIS_ADDR")
	os.Unsetenv("GOTUNNEL_AUTH_KEY")
	os.Unsetenv("PORT")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load defaults: %v", err)
	}

	if cfg.Server.ListenHTTP != ":80" {
		t.Fatalf("ListenHTTP: got %s, want :80", cfg.Server.ListenHTTP)
	}
	if cfg.Server.ListenHTTPS != ":443" {
		t.Fatalf("ListenHTTPS: got %s, want :443", cfg.Server.ListenHTTPS)
	}
	if cfg.Storage.Type != "memory" {
		t.Fatalf("Storage.Type: got %s, want memory", cfg.Storage.Type)
	}
	if cfg.Registry.DefaultTTLSeconds != 3600 {
		t.Fatalf("DefaultTTLSeconds: got %d, want 3600", cfg.Registry.DefaultTTLSeconds)
	}
	if cfg.Limits.MaxTunnelsPerUser != 10 {
		t.Fatalf("MaxTunnelsPerUser: got %d, want 10", cfg.Limits.MaxTunnelsPerUser)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("Logging.Level: got %s, want info", cfg.Logging.Level)
	}
	if !cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled should be true by default")
	}
	if cfg.Metrics.BindAddr != ":9090" {
		t.Fatalf("Metrics.BindAddr: got %s, want :9090", cfg.Metrics.BindAddr)
	}
	if cfg.Auth.TokenTTLMinutes != 525600 {
		t.Fatalf("TokenTTLMinutes: got %d, want 525600", cfg.Auth.TokenTTLMinutes)
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	os.Unsetenv("GOTUNNEL_CONFIG")
	os.Unsetenv("GOTUNNEL_LOG_LEVEL")
	os.Unsetenv("GOTUNNEL_STORAGE_TYPE")
	os.Unsetenv("GOTUNNEL_REDIS_ADDR")
	os.Unsetenv("GOTUNNEL_AUTH_KEY")
	os.Unsetenv("PORT")

	yamlContent := `
server:
  listen_http: ":8080"
  listen_https: ":8443"
  gateway_host: "localhost"
  public_hostname: "tunnel.example.com"
auth:
  token_signing_key: "my-secret-key"
  token_ttl_minutes: 120
storage:
  type: "redis"
  redis:
    addr: "redis:6379"
registry:
  reserved_subdomains:
    - api
    - admin
  default_ttl_seconds: 7200
  max_subdomain_length: 64
limits:
  max_tunnels_per_user: 5
logging:
  level: "debug"
  format: "text"
metrics:
  enabled: false
  bind_addr: ":9999"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.ListenHTTP != ":8080" {
		t.Fatalf("ListenHTTP: got %s, want :8080", cfg.Server.ListenHTTP)
	}
	if cfg.Server.ListenHTTPS != ":8443" {
		t.Fatalf("ListenHTTPS: got %s, want :8443", cfg.Server.ListenHTTPS)
	}
	if cfg.Server.PublicHostname != "tunnel.example.com" {
		t.Fatalf("PublicHostname: got %s", cfg.Server.PublicHostname)
	}
	if cfg.Auth.TokenSigningKey != "my-secret-key" {
		t.Fatalf("TokenSigningKey: got %s", cfg.Auth.TokenSigningKey)
	}
	if cfg.Auth.TokenTTLMinutes != 120 {
		t.Fatalf("TokenTTLMinutes: got %d", cfg.Auth.TokenTTLMinutes)
	}
	if cfg.Storage.Type != "redis" {
		t.Fatalf("Storage.Type: got %s", cfg.Storage.Type)
	}
	if cfg.Storage.Redis.Addr != "redis:6379" {
		t.Fatalf("Redis.Addr: got %s", cfg.Storage.Redis.Addr)
	}
	if len(cfg.Registry.ReservedSubdomains) != 2 {
		t.Fatalf("ReservedSubdomains: got %d", len(cfg.Registry.ReservedSubdomains))
	}
	if cfg.Registry.DefaultTTLSeconds != 7200 {
		t.Fatalf("DefaultTTLSeconds: got %d", cfg.Registry.DefaultTTLSeconds)
	}
	if cfg.Registry.MaxSubdomainLength != 64 {
		t.Fatalf("MaxSubdomainLength: got %d", cfg.Registry.MaxSubdomainLength)
	}
	if cfg.Limits.MaxTunnelsPerUser != 5 {
		t.Fatalf("MaxTunnelsPerUser: got %d", cfg.Limits.MaxTunnelsPerUser)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("Logging.Level: got %s", cfg.Logging.Level)
	}
	if cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled should be false")
	}
	if cfg.Metrics.BindAddr != ":9999" {
		t.Fatalf("Metrics.BindAddr: got %s", cfg.Metrics.BindAddr)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	os.Unsetenv("GOTUNNEL_CONFIG")
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	os.Unsetenv("GOTUNNEL_CONFIG")
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte("server:\n  listen_http: [invalid"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Unsetenv("GOTUNNEL_CONFIG")

	t.Setenv("GOTUNNEL_LOG_LEVEL", "debug")
	t.Setenv("GOTUNNEL_STORAGE_TYPE", "redis")
	t.Setenv("GOTUNNEL_REDIS_ADDR", "redis.local:6380")
	t.Setenv("GOTUNNEL_AUTH_KEY", "env-secret-key")
	t.Setenv("PORT", "9443")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Logging.Level != "debug" {
		t.Fatalf("Logging.Level: got %s, want debug", cfg.Logging.Level)
	}
	if cfg.Storage.Type != "redis" {
		t.Fatalf("Storage.Type: got %s, want redis", cfg.Storage.Type)
	}
	if cfg.Storage.Redis.Addr != "redis.local:6380" {
		t.Fatalf("Redis.Addr: got %s", cfg.Storage.Redis.Addr)
	}
	if cfg.Auth.TokenSigningKey != "env-secret-key" {
		t.Fatalf("TokenSigningKey: got %s", cfg.Auth.TokenSigningKey)
	}
	if cfg.Server.ListenHTTPS != ":9443" {
		t.Fatalf("ListenHTTPS: got %s, want :9443", cfg.Server.ListenHTTPS)
	}
}

func TestLoad_EnvOverridesOnTopOfFile(t *testing.T) {
	os.Unsetenv("GOTUNNEL_CONFIG")

	yamlContent := `
logging:
  level: "warn"
storage:
  type: "memory"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(yamlContent), 0644)

	t.Setenv("GOTUNNEL_LOG_LEVEL", "error")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Env should override file value
	if cfg.Logging.Level != "error" {
		t.Fatalf("Logging.Level: got %s, want error (env override)", cfg.Logging.Level)
	}
	// File value should be used when no env override
	if cfg.Storage.Type != "memory" {
		t.Fatalf("Storage.Type: got %s, want memory (from file)", cfg.Storage.Type)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	os.Unsetenv("GOTUNNEL_CONFIG")
	t.Setenv("PORT", "notanumber")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Invalid port should not change default
	if cfg.Server.ListenHTTPS != ":443" {
		t.Fatalf("ListenHTTPS should remain default: got %s", cfg.Server.ListenHTTPS)
	}
}

func TestLoad_GotunnelConfigEnv(t *testing.T) {
	yamlContent := `
server:
  listen_http: ":3000"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "env-config.yaml")
	os.WriteFile(path, []byte(yamlContent), 0644)

	t.Setenv("GOTUNNEL_CONFIG", path)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.ListenHTTP != ":3000" {
		t.Fatalf("ListenHTTP: got %s, want :3000", cfg.Server.ListenHTTP)
	}
}

func TestLoadClientConfig_Defaults(t *testing.T) {
	os.Unsetenv("GOTUNNEL_CONFIG")

	cfg, err := LoadClientConfig("")
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}

	if cfg.Client.LocalBind != "127.0.0.1" {
		t.Fatalf("LocalBind: got %s, want 127.0.0.1", cfg.Client.LocalBind)
	}
	if cfg.Client.HeartbeatSeconds != 10 {
		t.Fatalf("HeartbeatSeconds: got %d, want 10", cfg.Client.HeartbeatSeconds)
	}
	if cfg.Client.Reconnect.BackoffMs != 1000 {
		t.Fatalf("BackoffMs: got %d, want 1000", cfg.Client.Reconnect.BackoffMs)
	}
}

func TestLoadClientConfig_FromFile(t *testing.T) {
	os.Unsetenv("GOTUNNEL_CONFIG")

	yamlContent := `
client:
  gateway_url: "wss://tunnel.example.com/ws"
  token: "my-token"
  tunnels:
    - name: web
      port: 3000
      protocol: http
      subdomain: mysite
  heartbeat_seconds: 30
`
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	os.WriteFile(path, []byte(yamlContent), 0644)

	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}

	if cfg.Client.GatewayURL != "wss://tunnel.example.com/ws" {
		t.Fatalf("GatewayURL: got %s", cfg.Client.GatewayURL)
	}
	if cfg.Client.Token != "my-token" {
		t.Fatalf("Token: got %s", cfg.Client.Token)
	}
	if len(cfg.Client.Tunnels) != 1 {
		t.Fatalf("Tunnels count: got %d", len(cfg.Client.Tunnels))
	}
	if cfg.Client.Tunnels[0].Name != "web" || cfg.Client.Tunnels[0].Port != 3000 {
		t.Fatalf("Tunnel mismatch: %+v", cfg.Client.Tunnels[0])
	}
	if cfg.Client.HeartbeatSeconds != 30 {
		t.Fatalf("HeartbeatSeconds: got %d", cfg.Client.HeartbeatSeconds)
	}
}
