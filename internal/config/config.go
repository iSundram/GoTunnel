// Package config provides configuration loading for the GoTunnel server and client.
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds all server-side configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"   json:"server"`
	Auth     AuthConfig     `yaml:"auth"     json:"auth"`
	Storage  StorageConfig  `yaml:"storage"  json:"storage"`
	Registry RegistryConfig `yaml:"registry" json:"registry"`
	Limits   LimitsConfig   `yaml:"limits"   json:"limits"`
	Logging  LoggingConfig  `yaml:"logging"  json:"logging"`
	Metrics  MetricsConfig  `yaml:"metrics"  json:"metrics"`
	Security SecurityConfig `yaml:"security" json:"security"`
}

// ServerConfig holds listener and TLS settings.
type ServerConfig struct {
	ListenHTTP     string   `yaml:"listen_http"      json:"listen_http"`
	ListenHTTPS    string   `yaml:"listen_https"     json:"listen_https"`
	GatewayHost    string   `yaml:"gateway_host"     json:"gateway_host"`
	PublicHostname string   `yaml:"public_hostname"  json:"public_hostname"`
	AllowedDomains []string `yaml:"allowed_domains"  json:"allowed_domains"`
	TLS            TLSConfig `yaml:"tls"             json:"tls"`
}

// TLSConfig holds TLS and ACME settings.
type TLSConfig struct {
	Enabled         bool       `yaml:"enabled"           json:"enabled"`
	AutoLetsEncrypt bool       `yaml:"auto_lets_encrypt" json:"auto_lets_encrypt"`
	CertDir         string     `yaml:"cert_dir"          json:"cert_dir"`
	ACME            ACMEConfig `yaml:"acme"              json:"acme"`
}

// ACMEConfig holds ACME/Let's Encrypt settings.
type ACMEConfig struct {
	Email   string `yaml:"email"   json:"email"`
	Staging bool   `yaml:"staging" json:"staging"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	TokenSigningKey string `yaml:"token_signing_key" json:"token_signing_key"`
	TokenTTLMinutes int    `yaml:"token_ttl_minutes" json:"token_ttl_minutes"`
	EnableJWT       bool   `yaml:"enable_jwt"        json:"enable_jwt"`
	JWTIssuer       string `yaml:"jwt_issuer"        json:"jwt_issuer"`
	JWTAudience     string `yaml:"jwt_audience"      json:"jwt_audience"`
}

// StorageConfig selects and configures the backing store.
type StorageConfig struct {
	Type     string         `yaml:"type"     json:"type"`
	Redis    RedisConfig    `yaml:"redis"    json:"redis"`
	Postgres PostgresConfig `yaml:"postgres" json:"postgres"`
}

// RedisConfig holds Redis connection parameters.
type RedisConfig struct {
	Addr     string `yaml:"addr"     json:"addr"`
	DB       int    `yaml:"db"       json:"db"`
	Password string `yaml:"password" json:"password"`
}

// PostgresConfig holds Postgres connection parameters.
type PostgresConfig struct {
	DSN string `yaml:"dsn" json:"dsn"`
}

// RegistryConfig holds subdomain registry settings.
type RegistryConfig struct {
	ReservedSubdomains []string `yaml:"reserved_subdomains" json:"reserved_subdomains"`
	DefaultTTLSeconds  int      `yaml:"default_ttl_seconds" json:"default_ttl_seconds"`
	MaxSubdomainLength int      `yaml:"max_subdomain_length" json:"max_subdomain_length"`
}

// LimitsConfig holds rate and resource limits.
type LimitsConfig struct {
	MaxTunnelsPerUser             int `yaml:"max_tunnels_per_user"               json:"max_tunnels_per_user"`
	MaxBandwidthPerTunnelMbps     int `yaml:"max_bandwidth_per_tunnel_mbps"      json:"max_bandwidth_per_tunnel_mbps"`
	MaxConcurrentStreamsPerTunnel  int `yaml:"max_concurrent_streams_per_tunnel"  json:"max_concurrent_streams_per_tunnel"`
	RequestRatePerIPPerMin        int `yaml:"request_rate_per_ip_per_min"        json:"request_rate_per_ip_per_min"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"  json:"level"`
	Format string `yaml:"format" json:"format"`
	File   string `yaml:"file"   json:"file"`
}

// MetricsConfig holds Prometheus metrics settings.
type MetricsConfig struct {
	Enabled  bool   `yaml:"enabled"   json:"enabled"`
	BindAddr string `yaml:"bind_addr" json:"bind_addr"`
}

// SecurityConfig holds IP filtering and mTLS settings.
type SecurityConfig struct {
	IPBlacklist []string   `yaml:"ip_blacklist" json:"ip_blacklist"`
	IPWhitelist []string   `yaml:"ip_whitelist" json:"ip_whitelist"`
	EnableMTLS  bool       `yaml:"enable_mtls"  json:"enable_mtls"`
	MTLS        MTLSConfig `yaml:"mtls"         json:"mtls"`
}

// MTLSConfig holds mutual-TLS settings.
type MTLSConfig struct {
	CACertFile string `yaml:"ca_cert_file" json:"ca_cert_file"`
}

// ClientConfig holds all client-side configuration.
type ClientConfig struct {
	Client ClientSettings `yaml:"client" json:"client"`
}

// ClientSettings holds the inner client configuration block.
type ClientSettings struct {
	GatewayURL       string              `yaml:"gateway_url"       json:"gateway_url"`
	Token            string              `yaml:"token"             json:"token"`
	LocalBind        string              `yaml:"local_bind"        json:"local_bind"`
	Tunnels          []ClientTunnel      `yaml:"tunnels"           json:"tunnels"`
	Reconnect        ReconnectConfig     `yaml:"reconnect"         json:"reconnect"`
	HeartbeatSeconds int                 `yaml:"heartbeat_seconds" json:"heartbeat_seconds"`
	TLS              ClientTLSConfig     `yaml:"tls"               json:"tls"`
}

// ClientTunnel describes a single tunnel the client should establish.
type ClientTunnel struct {
	Name      string `yaml:"name"      json:"name"`
	Port      int    `yaml:"port"      json:"port"`
	Protocol  string `yaml:"protocol"  json:"protocol"`
	Subdomain string `yaml:"subdomain" json:"subdomain"`
}

// ReconnectConfig holds reconnection behaviour settings.
type ReconnectConfig struct {
	MaxRetries int `yaml:"max_retries" json:"max_retries"`
	BackoffMs  int `yaml:"backoff_ms"  json:"backoff_ms"`
}

// ClientTLSConfig holds client-side TLS verification settings.
type ClientTLSConfig struct {
	SkipVerify bool   `yaml:"skip_verify" json:"skip_verify"`
	CAFile     string `yaml:"ca_file"     json:"ca_file"`
}

// defaults returns a Config populated with sensible default values.
func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			ListenHTTP:  ":80",
			ListenHTTPS: ":443",
			GatewayHost: "0.0.0.0",
			TLS: TLSConfig{
				Enabled: true,
				CertDir: "/var/lib/gotunnel/certs",
			},
		},
		Auth: AuthConfig{
			TokenTTLMinutes: 525600, // ~1 year in minutes
			EnableJWT:       true,
			JWTIssuer:       "gotunnel",
			JWTAudience:     "gotunnel-clients",
		},
		Storage: StorageConfig{
			Type: "memory",
			Redis: RedisConfig{
				Addr: "127.0.0.1:6379",
				DB:   0,
			},
		},
		Registry: RegistryConfig{
			DefaultTTLSeconds:  3600,
			MaxSubdomainLength: 32,
		},
		Limits: LimitsConfig{
			MaxTunnelsPerUser:            10,
			MaxBandwidthPerTunnelMbps:    100,
			MaxConcurrentStreamsPerTunnel: 256,
			RequestRatePerIPPerMin:       600,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Metrics: MetricsConfig{
			Enabled:  true,
			BindAddr: ":9090",
		},
	}
}

// clientDefaults returns a ClientConfig populated with sensible default values.
func clientDefaults() *ClientConfig {
	return &ClientConfig{
		Client: ClientSettings{
			LocalBind:        "127.0.0.1",
			HeartbeatSeconds: 10,
			Reconnect: ReconnectConfig{
				MaxRetries: 0, // 0 = unlimited retries
				BackoffMs:  1000,
			},
			TLS: ClientTLSConfig{
				CAFile: "/etc/ssl/certs/ca-certificates.crt",
			},
		},
	}
}

// Load reads a YAML config file and returns a server Config.
// An empty path falls back to the GOTUNNEL_CONFIG env var.
// Environment variables override individual fields after file parsing.
func Load(path string) (*Config, error) {
	cfg := defaults()

	if path == "" {
		path = os.Getenv("GOTUNNEL_CONFIG")
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: read file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: parse yaml: %w", err)
		}
	}

	applyServerEnv(cfg)
	return cfg, nil
}

// LoadClientConfig reads a YAML config file and returns a ClientConfig.
func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg := clientDefaults()

	if path == "" {
		path = os.Getenv("GOTUNNEL_CONFIG")
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: read file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: parse yaml: %w", err)
		}
	}

	return cfg, nil
}

// applyServerEnv overrides config values from environment variables.
func applyServerEnv(cfg *Config) {
	if v := os.Getenv("GOTUNNEL_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("GOTUNNEL_STORAGE_TYPE"); v != "" {
		cfg.Storage.Type = v
	}
	if v := os.Getenv("GOTUNNEL_REDIS_ADDR"); v != "" {
		cfg.Storage.Redis.Addr = v
	}
	if v := os.Getenv("GOTUNNEL_AUTH_KEY"); v != "" {
		cfg.Auth.TokenSigningKey = v
	}
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p <= 65535 {
			cfg.Server.ListenHTTPS = ":" + v
		}
	}
}
