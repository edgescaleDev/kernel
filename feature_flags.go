package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// FeatureEnabled checks whether a feature flag is enabled for the given org.
// Checks Redis first, falls back to database.
func (k *Kernel) FeatureEnabled(ctx context.Context, flag string, orgID string) bool {
	if k.redis == nil {
		return false
	}

	cacheKey := fmt.Sprintf("feature:%s:%s", flag, orgID)
	val, err := k.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		return val == "1"
	}
	if err != redis.Nil {
		k.logger.Error("feature flag check failed", "flag", flag, "error", err)
	}

	// TODO: Fall back to database once the feature_flags table is created.
	return false
}

// SetFeatureFlag sets a feature flag for an org. Stored in Redis with a TTL.
func (k *Kernel) SetFeatureFlag(ctx context.Context, flag string, orgID string, enabled bool) error {
	if k.redis == nil {
		return fmt.Errorf("redis not available")
	}

	cacheKey := fmt.Sprintf("feature:%s:%s", flag, orgID)
	val := "0"
	if enabled {
		val = "1"
	}
	return k.redis.Set(ctx, cacheKey, val, 24*time.Hour).Err()
}
