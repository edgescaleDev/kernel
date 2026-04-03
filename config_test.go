package kernel

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.ShutdownTimeout != 30*time.Second {
		t.Errorf("Server.ShutdownTimeout = %v, want 30s", cfg.Server.ShutdownTimeout)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "localhost")
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want 5432", cfg.Database.Port)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("Database.MaxOpenConns = %d, want 25", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.SSLMode != "disable" {
		t.Errorf("Database.SSLMode = %q, want %q", cfg.Database.SSLMode, "disable")
	}
	if cfg.Redis.Addr != "localhost:6379" {
		t.Errorf("Redis.Addr = %q, want %q", cfg.Redis.Addr, "localhost:6379")
	}
}

func TestDSN(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "db.example.com",
		Port:     5432,
		Name:     "mydb",
		User:     "admin",
		Password: "secret",
		SSLMode:  "require",
	}
	dsn := cfg.DSN()
	want := "host=db.example.com port=5432 user=admin password=secret dbname=mydb sslmode=require"
	if dsn != want {
		t.Errorf("DSN() = %q, want %q", dsn, want)
	}
}

func TestDSN_Defaults(t *testing.T) {
	cfg := DatabaseConfig{
		Host: "localhost",
		Name: "test",
		User: "user",
	}
	dsn := cfg.DSN()
	want := "host=localhost port=5432 user=user password= dbname=test sslmode=disable"
	if dsn != want {
		t.Errorf("DSN() with defaults = %q, want %q", dsn, want)
	}
}

func TestConfig_Validate(t *testing.T) {
	// Valid config.
	cfg := DefaultConfig()
	cfg.Database.Name = "test"
	cfg.Database.User = "user"
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() valid config: unexpected error: %v", err)
	}

	// Missing database host.
	bad := cfg
	bad.Database.Host = ""
	if err := bad.Validate(); err == nil {
		t.Error("Validate() missing host: expected error")
	}

	// Missing database name.
	bad = cfg
	bad.Database.Name = ""
	if err := bad.Validate(); err == nil {
		t.Error("Validate() missing name: expected error")
	}

	// Invalid port.
	bad = cfg
	bad.Server.Port = -1
	if err := bad.Validate(); err == nil {
		t.Error("Validate() invalid port: expected error")
	}

	// Port too high.
	bad = cfg
	bad.Server.Port = 99999
	if err := bad.Validate(); err == nil {
		t.Error("Validate() port too high: expected error")
	}
}
