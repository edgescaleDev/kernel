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
	// The lock auto-expires after ttl to prevent deadlocks from crashed instances.
	Acquire(ctx context.Context, key string, ttl time.Duration) (release func(), acquired bool, err error)
}
