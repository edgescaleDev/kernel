package sdk

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Idempotent checks whether an idempotency key has already been processed.
// If the key is new, it locks it in Redis with the given TTL and returns false.
// If the key exists, it returns true (duplicate request).
//
// The key is namespaced by org and user to prevent cross-user collisions.
//
// Usage in middleware or handler:
//
//	key := c.GetHeader("Idempotency-Key")
//	if duplicate, _ := sdk.Idempotent(ctx, rdb, orgID, userID, key, 24*time.Hour); duplicate {
//	    // Return cached response
//	}
func Idempotent(ctx context.Context, rdb redis.Cmdable, orgID, userID, key string, ttl time.Duration) (bool, error) {
	if rdb == nil {
		return false, nil
	}
	if key == "" {
		return false, nil
	}

	cacheKey := fmt.Sprintf("idempotency:%s:%s:%s", orgID, userID, key)
	set, err := rdb.SetNX(ctx, cacheKey, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("idempotency check: %w", err)
	}

	// SetNX returns true if the key was set (new request), false if it already existed.
	return !set, nil
}

// StoreIdempotentResult stores the response for an idempotency key so duplicate
// requests can return the same response without reprocessing.
func StoreIdempotentResult(ctx context.Context, rdb redis.Cmdable, orgID, userID, key string, result []byte, ttl time.Duration) error {
	if rdb == nil {
		return nil
	}

	cacheKey := fmt.Sprintf("idempotency:result:%s:%s:%s", orgID, userID, key)
	return rdb.Set(ctx, cacheKey, result, ttl).Err()
}

// GetIdempotentResult retrieves a previously stored response for the given key.
// Returns nil, nil if no cached response exists.
func GetIdempotentResult(ctx context.Context, rdb redis.Cmdable, orgID, userID, key string) ([]byte, error) {
	if rdb == nil {
		return nil, nil
	}

	cacheKey := fmt.Sprintf("idempotency:result:%s:%s:%s", orgID, userID, key)
	data, err := rdb.Get(ctx, cacheKey).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get idempotent result: %w", err)
	}
	return data, nil
}
