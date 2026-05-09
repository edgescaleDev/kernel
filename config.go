package kernel

import (
	"fmt"
	"time"
)

// Config holds all kernel configuration.
// Loaded via Viper from environment variables, YAML files, and CLI flags.
type Config struct {
	// Server configures the HTTP server.
	Server ServerConfig `mapstructure:"server"`

	// Database configures the PostgreSQL connection.
	Database DatabaseConfig `mapstructure:"database"`

	// Redis configures the Redis connection.
	Redis RedisConfig `mapstructure:"redis"`

	// Dev enables development mode with graceful degradation
	// for optional infrastructure (task executor, search, etc.).
	Dev DevConfig `mapstructure:"dev"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Port is the port to listen on (default: 8080).
	Port int `mapstructure:"port"`

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration `mapstructure:"read_timeout"`

	// WriteTimeout is the maximum duration for writing the response.
	WriteTimeout time.Duration `mapstructure:"write_timeout"`

	// IdleTimeout is the maximum duration for idle connections.
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`

	// ShutdownTimeout is the maximum time to wait for graceful shutdown.
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`

	// CacheTTL is how long resolved user/permission data is cached
	// in the kernel middleware layer (default: 15m).
	CacheTTL time.Duration `mapstructure:"cache_ttl"`

	// TrialDuration is how long feature modules remain active after
	// a tenant is provisioned. When a tenant is created, all TypeFeature
	// modules are activated with an expires_at set to now + TrialDuration.
	// Set to 0 to disable trial provisioning (default: 7 days).
	TrialDuration time.Duration `mapstructure:"trial_duration"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	// Host is the database hostname (default: "localhost").
	Host string `mapstructure:"host"`

	// Port is the database port (default: 5432).
	Port int `mapstructure:"port"`

	// Name is the database name.
	Name string `mapstructure:"name"`

	// User is the database username.
	User string `mapstructure:"user"`

	// Password is the database password.
	Password string `mapstructure:"password"`

	// SSLMode controls TLS (default: "disable" for dev, "require" for prod).
	SSLMode string `mapstructure:"sslmode"`

	// MaxOpenConns is the max number of open connections (default: 25).
	MaxOpenConns int `mapstructure:"max_open_conns"`

	// MaxIdleConns is the max number of idle connections (default: 5).
	MaxIdleConns int `mapstructure:"max_idle_conns"`

	// ConnMaxLifetime is the max connection lifetime (default: 5m).
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// DSN returns the PostgreSQL connection string.
func (c DatabaseConfig) DSN() string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, port, c.User, c.Password, c.Name, sslMode,
	)
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	// Addr is the Redis address (default: "localhost:6379").
	Addr string `mapstructure:"addr"`

	// Password is the Redis password (empty for no auth).
	Password string `mapstructure:"password"`

	// DB is the Redis database number (default: 0).
	DB int `mapstructure:"db"`
}

// DevConfig controls development mode settings.
// In dev mode, the kernel gracefully degrades when optional infrastructure
// is unavailable (e.g., uses inline task executor, noop search engine).
type DevConfig struct {
	// Mode enables dev mode when true.
	Mode bool `mapstructure:"mode"`
}

// DefaultConfig returns a Config with sensible defaults for local development.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Port:            8080,
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    15 * time.Second,
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 30 * time.Second,
			CacheTTL:        15 * time.Minute,
			TrialDuration:   7 * 24 * time.Hour,
		},
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			SSLMode:         "disable",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		Redis: RedisConfig{
			Addr: "localhost:6379",
			DB:   0,
		},
	}
}

// Validate checks that required configuration fields are set.
func (c Config) Validate() error {
	if c.Database.Host == "" {
		return fmt.Errorf("config: database.host is required")
	}
	if c.Database.Name == "" {
		return fmt.Errorf("config: database.name is required")
	}
	if c.Database.User == "" {
		return fmt.Errorf("config: database.user is required")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("config: redis.addr is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("config: server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	return nil
}
