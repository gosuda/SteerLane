// Package config provides typed configuration for the SteerLane server.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mode represents the deployment mode.
type Mode string

const (
	ModeSaaS       Mode = "saas"
	ModeSelfHosted Mode = "selfhosted"
)

// Config is the top-level configuration for the SteerLane server.
type Config struct { //nolint:govet // fieldalignment: readability over 8-byte saving
	HTTP      HTTPConfig
	Auth      AuthConfig
	Bootstrap BootstrapConfig
	Slack     SlackConfig
	Discord   DiscordConfig
	Telegram  TelegramConfig
	HITL      HITLConfig
	Email     EmailConfig
	Redis     RedisConfig
	CORS      CORSConfig
	Postgres  PostgresConfig
	Mode      Mode
	LogLevel  string
	RateLimit RateLimitConfig
}

// HTTPConfig holds HTTP server settings.
type HTTPConfig struct {
	Addr              string
	PublicBaseURL     string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// PostgresConfig holds database connection settings.
type PostgresConfig struct {
	DSN string
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	JWTSecret        string
	JWTIssuer        string
	JWTExpiry        time.Duration
	JWTRefreshExpiry time.Duration
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	Origins []string
	MaxAge  int
}

// RateLimitConfig holds global request rate limiting settings.
type RateLimitConfig struct {
	TrustedProxies    []string
	RequestsPerMinute int
	Enabled           bool
}

// HITLConfig holds HITL timeout escalation settings.
type HITLConfig struct {
	// ExtendedTimeout is the duration added after the initial timeout when
	// escalating an unanswered HITL question (default: 1 hour).
	ExtendedTimeout time.Duration
}

// EmailConfig holds optional SMTP email notification settings.
type EmailConfig struct { //nolint:govet // readability over field packing
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	FromAddress  string
	Enabled      bool
}

// IsEnabled returns true if email notification delivery is properly configured.
func (c EmailConfig) IsEnabled() bool {
	return c.Enabled && c.SMTPHost != "" && c.FromAddress != ""
}

// SlackConfig holds optional Slack integration settings.
// When SigningSecret is empty, Slack routes are still registered but signature
// verification falls back to a no-op verifier (development mode).
type SlackConfig struct {
	// SigningSecret is the Slack app signing secret for verifying webhook signatures.
	SigningSecret string
	// BotToken is the xoxb-* token for sending messages to Slack.
	BotToken string
}

// DiscordConfig holds optional Discord integration settings.
type DiscordConfig struct {
	ApplicationID string
	BotToken      string
	PublicKey     string
}

// Enabled returns true if the Discord integration has the minimum config set.
func (c DiscordConfig) Enabled() bool {
	return c.ApplicationID != "" && c.BotToken != "" && c.PublicKey != ""
}

// TelegramConfig holds optional Telegram integration settings.
type TelegramConfig struct {
	BotToken      string
	WebhookSecret string
}

// Enabled returns true if the Telegram integration has the minimum config set.
func (c TelegramConfig) Enabled() bool {
	return c.BotToken != "" && c.WebhookSecret != ""
}

// Enabled returns true if the Slack integration has the minimum config set.
func (c SlackConfig) Enabled() bool {
	return c.SigningSecret != "" && c.BotToken != ""
}

// BootstrapConfig holds self-hosted first-run admin creation settings.
type BootstrapConfig struct {
	AdminEmail    string
	AdminPassword string
	AdminName     string
}

// IsSelfHosted returns true if the server is running in self-hosted mode.
func (c Config) IsSelfHosted() bool {
	return c.Mode == ModeSelfHosted
}

// Validate checks all required configuration values.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeSaaS, ModeSelfHosted:
		// valid
	default:
		return fmt.Errorf("invalid mode %q: must be %q or %q", c.Mode, ModeSaaS, ModeSelfHosted)
	}

	if c.HTTP.Addr == "" {
		return errors.New("STEERLANE_HTTP_ADDR is required")
	}
	if strings.TrimSpace(c.HTTP.PublicBaseURL) == "" {
		return errors.New("STEERLANE_PUBLIC_BASE_URL is required")
	}
	if c.Postgres.DSN == "" {
		return errors.New("STEERLANE_POSTGRES_DSN is required")
	}
	if c.Redis.Addr == "" {
		return errors.New("STEERLANE_REDIS_ADDR is required")
	}
	if c.Auth.JWTSecret == "" {
		return errors.New("STEERLANE_JWT_SECRET is required")
	}
	if c.Auth.JWTExpiry <= 0 {
		return errors.New("STEERLANE_JWT_EXPIRY must be a positive duration")
	}
	if c.RateLimit.Enabled && c.RateLimit.RequestsPerMinute <= 0 {
		return errors.New("STEERLANE_RATE_LIMIT_REQUESTS_PER_MINUTE must be a positive integer when rate limiting is enabled")
	}
	for _, cidr := range c.RateLimit.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid STEERLANE_TRUSTED_PROXIES CIDR %q: %w", cidr, err)
		}
	}
	return nil
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (Config, error) {
	cfg := Config{
		Mode:     Mode(envOr("STEERLANE_MODE", "selfhosted")),
		LogLevel: envOr("STEERLANE_LOG_LEVEL", "info"),
		HTTP: HTTPConfig{
			Addr:              envOr("STEERLANE_HTTP_ADDR", ":8080"),
			PublicBaseURL:     envOr("STEERLANE_PUBLIC_BASE_URL", ""),
			ReadHeaderTimeout: envDurationOr("STEERLANE_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       envDurationOr("STEERLANE_HTTP_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:      envDurationOr("STEERLANE_HTTP_WRITE_TIMEOUT", 60*time.Second),
			IdleTimeout:       envDurationOr("STEERLANE_HTTP_IDLE_TIMEOUT", 120*time.Second),
		},
		Postgres: PostgresConfig{
			DSN: envOr("STEERLANE_POSTGRES_DSN", ""),
		},
		Redis: RedisConfig{
			Addr:     envOr("STEERLANE_REDIS_ADDR", "localhost:6379"),
			Password: envOr("STEERLANE_REDIS_PASSWORD", ""),
			DB:       envIntOr("STEERLANE_REDIS_DB", 0),
		},
		Auth: AuthConfig{
			JWTSecret:        envOr("STEERLANE_JWT_SECRET", ""),
			JWTIssuer:        envOr("STEERLANE_JWT_ISSUER", "steerlane"),
			JWTExpiry:        envDurationOr("STEERLANE_JWT_EXPIRY", 24*time.Hour),
			JWTRefreshExpiry: envDurationOr("STEERLANE_JWT_REFRESH_EXPIRY", 168*time.Hour),
		},
		CORS: CORSConfig{
			Origins: strings.Split(envOr("STEERLANE_CORS_ORIGINS", "http://localhost:5173"), ","),
			MaxAge:  envIntOr("STEERLANE_CORS_MAX_AGE", 3600),
		},
		RateLimit: RateLimitConfig{
			Enabled:           envBoolOr("STEERLANE_RATE_LIMIT_ENABLED", false),
			RequestsPerMinute: envIntOr("STEERLANE_RATE_LIMIT_REQUESTS_PER_MINUTE", 60),
			TrustedProxies:    parseTrustedProxies(os.Getenv("STEERLANE_TRUSTED_PROXIES")),
		},
		Bootstrap: BootstrapConfig{
			AdminEmail:    os.Getenv("STEERLANE_BOOTSTRAP_ADMIN_EMAIL"),
			AdminPassword: os.Getenv("STEERLANE_BOOTSTRAP_ADMIN_PASSWORD"),
			AdminName:     os.Getenv("STEERLANE_BOOTSTRAP_ADMIN_NAME"),
		},
		Slack: SlackConfig{
			SigningSecret: os.Getenv("STEERLANE_SLACK_SIGNING_SECRET"),
			BotToken:      os.Getenv("STEERLANE_SLACK_BOT_TOKEN"),
		},
		Discord: DiscordConfig{
			ApplicationID: os.Getenv("STEERLANE_DISCORD_APPLICATION_ID"),
			BotToken:      os.Getenv("STEERLANE_DISCORD_BOT_TOKEN"),
			PublicKey:     os.Getenv("STEERLANE_DISCORD_PUBLIC_KEY"),
		},
		Telegram: TelegramConfig{
			BotToken:      os.Getenv("STEERLANE_TELEGRAM_BOT_TOKEN"),
			WebhookSecret: os.Getenv("STEERLANE_TELEGRAM_WEBHOOK_SECRET"),
		},
		HITL: HITLConfig{
			ExtendedTimeout: envDurationOr("STEERLANE_HITL_EXTENDED_TIMEOUT", time.Hour),
		},
		Email: EmailConfig{
			Enabled:      envBoolOr("STEERLANE_EMAIL_ENABLED", false),
			SMTPHost:     os.Getenv("STEERLANE_EMAIL_SMTP_HOST"),
			SMTPPort:     envIntOr("STEERLANE_EMAIL_SMTP_PORT", 587),
			SMTPUsername: os.Getenv("STEERLANE_EMAIL_SMTP_USERNAME"),
			SMTPPassword: os.Getenv("STEERLANE_EMAIL_SMTP_PASSWORD"),
			FromAddress:  os.Getenv("STEERLANE_EMAIL_FROM_ADDRESS"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBoolOr(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}

	return b
}

func parseTrustedProxies(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	trustedProxies := make([]string, 0, len(parts))
	for _, part := range parts {
		cidr := strings.TrimSpace(part)
		if cidr == "" {
			continue
		}
		trustedProxies = append(trustedProxies, cidr)
	}

	if len(trustedProxies) == 0 {
		return nil
	}

	return trustedProxies
}
