package kernel

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
//	if duplicate, _ := k.Idempotent(ctx, orgID, userID, key, 24*time.Hour); duplicate {
//	    // Return cached response
//	}
func (k *Kernel) Idempotent(ctx context.Context, orgID, userID, key string, ttl time.Duration) (bool, error) {
	if k.redis == nil {
		return false, nil
	}
	if key == "" {
		return false, nil
	}

	cacheKey := fmt.Sprintf("idempotency:%s:%s:%s", orgID, userID, key)
	set, err := k.redis.SetNX(ctx, cacheKey, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("idempotency check: %w", err)
	}

	// SetNX returns true if the key was set (new request), false if it already existed.
	return !set, nil
}

// IdempotentResult stores the response for an idempotency key so duplicate
// requests can return the same response without reprocessing.
func (k *Kernel) IdempotentResult(ctx context.Context, orgID, userID, key string, result []byte, ttl time.Duration) error {
	if k.redis == nil {
		return nil
	}

	cacheKey := fmt.Sprintf("idempotency:result:%s:%s:%s", orgID, userID, key)
	return k.redis.Set(ctx, cacheKey, result, ttl).Err()
}

// GetIdempotentResult retrieves a previously stored response for the given key.
// Returns nil, nil if no cached response exists.
func (k *Kernel) GetIdempotentResult(ctx context.Context, orgID, userID, key string) ([]byte, error) {
	if k.redis == nil {
		return nil, nil
	}

	cacheKey := fmt.Sprintf("idempotency:result:%s:%s:%s", orgID, userID, key)
	data, err := k.redis.Get(ctx, cacheKey).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get idempotent result: %w", err)
	}
	return data, nil
}
