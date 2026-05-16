package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kernel-contrib/sdk"
)

// RedisLockProvider implements sdk.LockProvider using Redis SET NX EX.
// This is the recommended lock provider for multi-instance deployments.
type RedisLockProvider struct {
	client     *redis.Client
	instanceID string
}

// NewRedisLockProvider creates a Redis-backed distributed lock provider.
// The instanceID should be unique per process (e.g., hostname + PID).
func NewRedisLockProvider(client *redis.Client, instanceID string) *RedisLockProvider {
	return &RedisLockProvider{
		client:     client,
		instanceID: instanceID,
	}
}

// Acquire attempts to get a distributed lock for the given key.
// Uses SET key NX EX to atomically set-if-not-exists with TTL.
// The release function uses a Lua script to only delete if the value
// matches our instanceID — preventing release of another instance's lock.
func (p *RedisLockProvider) Acquire(ctx context.Context, key string, ttl time.Duration) (func(), bool, error) {
	lockKey := "lock:" + key
	ok, err := p.client.SetNX(ctx, lockKey, p.instanceID, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("redis lock acquire: %w", err)
	}
	if !ok {
		return nil, false, nil
	}

	release := func() {
		// Lua script: only delete if the value matches our instance ID.
		// This prevents releasing a lock that was re-acquired by another instance
		// after our TTL expired.
		const luaRelease = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
		p.client.Eval(context.Background(), luaRelease, []string{lockKey}, p.instanceID)
	}

	return release, true, nil
}

// Compile-time check that RedisLockProvider implements sdk.LockProvider.
var _ sdk.LockProvider = (*RedisLockProvider)(nil)
