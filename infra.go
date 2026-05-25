package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// connectInfra establishes connections to all required infrastructure.
// Both PostgreSQL and Redis are required - Boot() fails if either is unreachable.
func (k *Kernel) connectInfra() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. PostgreSQL - required.
	if err := k.connectPostgres(ctx); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	k.logger.Info("connected to PostgreSQL", "host", k.cfg.Database.Host, "db", k.cfg.Database.Name)

	// 2. Redis - required.
	if err := k.connectRedis(ctx); err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	k.logger.Info("connected to Redis", "addr", k.cfg.Redis.Addr)

	return nil
}

// connectPostgres opens a GORM connection to PostgreSQL with connection pooling.
// The provided context enforces a connection timeout.
func (k *Kernel) connectPostgres(ctx context.Context) error {
	// Choose GORM log level based on dev mode.
	logLevel := logger.Silent
	if k.cfg.Dev.Mode {
		logLevel = logger.Info
	}

	db, err := gorm.Open(postgres.Open(k.cfg.Database.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("underlying db: %w", err)
	}

	// Configure connection pool.
	cfg := k.cfg.Database
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	// Verify connectivity with timeout.
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	k.db = db

	// Enable OTel tracing on every SQL query when telemetry is active.
	if k.cfg.Telemetry.Enabled {
		if err := k.db.Use(newOtelGormPlugin()); err != nil {
			k.logger.Warn("failed to enable GORM tracing", "error", err)
		}
	}

	return nil
}

// connectRedis opens a connection to Redis and verifies connectivity.
func (k *Kernel) connectRedis(ctx context.Context) error {
	client := redis.NewClient(&redis.Options{
		Addr:     k.cfg.Redis.Addr,
		Password: k.cfg.Redis.Password,
		DB:       k.cfg.Redis.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	// Enable OTel tracing on every Redis command when telemetry is active.
	if k.cfg.Telemetry.Enabled {
		if err := redisotel.InstrumentTracing(client); err != nil {
			k.logger.Warn("failed to enable Redis tracing", "error", err)
		}
	}

	k.redis = client
	return nil
}

// closeInfra closes all infrastructure connections.
func (k *Kernel) closeInfra() {
	if k.db != nil {
		if sqlDB, err := k.db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				k.logger.Warn("error closing postgres", "error", err)
			}
		}
	}
	if k.redis != nil {
		if err := k.redis.Close(); err != nil {
			k.logger.Warn("error closing redis", "error", err)
		}
	}
}
