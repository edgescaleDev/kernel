package sdk

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache retrieves a value from Redis cache, falling back to the loader function
// on cache miss. The loaded value is stored in Redis with the given TTL.
//
// Usage:
//
//	org, err := sdk.Cache(ctx, redis, "org:"+id.String(), 5*time.Minute, func() (*Org, error) {
//	    return repo.FindByID(ctx, id)
//	})
func Cache[T any](ctx context.Context, r NamespacedRedis, key string, ttl time.Duration, loader func() (T, error)) (T, error) {
	var zero T

	// Try cache first.
	data, err := r.Get(ctx, key).Bytes()
	if err == nil {
		var result T
		if err := json.Unmarshal(data, &result); err == nil {
			return result, nil
		}
		// Corrupted cache entry - fall through to loader.
	}

	// Cache miss or error - load from source.
	result, err := loader()
	if err != nil {
		return zero, err
	}

	// Store in cache (best-effort, don't fail the request on cache write error).
	if encoded, err := json.Marshal(result); err == nil {
		r.Set(ctx, key, encoded, ttl)
	}

	return result, nil
}

// Invalidate removes a single key from the cache.
func Invalidate(ctx context.Context, r NamespacedRedis, key string) error {
	return r.Del(ctx, key).Err()
}

// InvalidateMany removes multiple keys from the cache.
func InvalidateMany(ctx context.Context, r NamespacedRedis, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.Del(ctx, keys...).Err()
}

// InvalidatePrefix removes all keys matching a prefix pattern.
// Uses SCAN to avoid blocking Redis with KEYS.
func InvalidatePrefix(ctx context.Context, r NamespacedRedis, prefix string) error {
	client, ok := r.Client().(*redis.Client)
	if !ok {
		return nil // can't SCAN on a pipeline or cluster without the full client
	}

	fullPrefix := r.Namespace() + prefix + "*"
	var cursor uint64
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, fullPrefix, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
