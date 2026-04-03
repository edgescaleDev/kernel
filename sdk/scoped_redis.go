package sdk

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// NamespacedRedis wraps a Redis client with automatic key prefixing.
// All keys are prefixed with "module:{service_id}:" to enforce isolation.
type NamespacedRedis struct {
	client    redis.Cmdable
	namespace string
}

// NewNamespacedRedis creates a NamespacedRedis with the given prefix.
func NewNamespacedRedis(client redis.Cmdable, serviceID string) NamespacedRedis {
	return NamespacedRedis{
		client:    client,
		namespace: "module:" + serviceID + ":",
	}
}

// key prefixes the given key with the service namespace.
func (r NamespacedRedis) key(k string) string {
	return r.namespace + k
}

// Get retrieves a value by key.
func (r NamespacedRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	return r.client.Get(ctx, r.key(key))
}

// Set stores a value with an optional TTL.
func (r NamespacedRedis) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	return r.client.Set(ctx, r.key(key), value, expiration)
}

// Del deletes one or more keys.
func (r NamespacedRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = r.key(k)
	}
	return r.client.Del(ctx, prefixed...)
}

// Exists checks if keys exist.
func (r NamespacedRedis) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = r.key(k)
	}
	return r.client.Exists(ctx, prefixed...)
}

// SetNX sets a value only if the key does not exist (for locking).
func (r NamespacedRedis) SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
	return r.client.SetNX(ctx, r.key(key), value, expiration)
}

// Expire sets a TTL on an existing key.
func (r NamespacedRedis) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	return r.client.Expire(ctx, r.key(key), expiration)
}

// Incr increments a key's integer value.
func (r NamespacedRedis) Incr(ctx context.Context, key string) *redis.IntCmd {
	return r.client.Incr(ctx, r.key(key))
}

// HSet sets hash fields.
func (r NamespacedRedis) HSet(ctx context.Context, key string, values ...any) *redis.IntCmd {
	return r.client.HSet(ctx, r.key(key), values...)
}

// HGetAll returns all fields in a hash.
func (r NamespacedRedis) HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd {
	return r.client.HGetAll(ctx, r.key(key))
}

// Client returns the underlying Redis client for operations
// that need direct access (e.g., Pub/Sub, Lua scripts).
func (r NamespacedRedis) Client() redis.Cmdable {
	return r.client
}

// Namespace returns the key prefix for this service.
func (r NamespacedRedis) Namespace() string {
	return r.namespace
}
