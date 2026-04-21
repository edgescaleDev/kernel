package sdk

import (
	"context"
	"time"
)

// LockProvider is a distributed locking primitive. The kernel uses it for
// cron deduplication across multiple instances. Modules can also use it
// for their own distributed locking needs (e.g., preventing double invoice
// generation in billing).
//
// The kernel ships with two built-in implementations:
//   - RedisLockProvider: uses SET NX EX (fast, requires Redis)
//   - DBLockProvider: uses PostgreSQL pg_advisory_lock (no Redis dependency)
//
// Configure at kernel build time:
//
//	kernel.New(cfg)
//	k.SetLockProvider(kernel.NewRedisLockProvider(redisClient))
type LockProvider interface {
	// Acquire attempts to get a distributed lock for the given key.
	// Returns a release function and true if acquired, or nil and false
	// if the lock is already held by another instance.
	// The ttl parameter is implementation-dependent: some providers use it
	// to auto-expire locks, while others may ignore it or only honor it on a
	// best-effort basis. Callers must not assume auto-expiration unless the
	// configured provider documents that behavior.
	Acquire(ctx context.Context, key string, ttl time.Duration) (release func(), acquired bool, err error)
}
