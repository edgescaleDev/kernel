package sdk

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimit returns a Gin middleware that enforces a fixed-window rate limit
// per client IP using Redis. The window starts on the first request and resets
// after the window duration. If Redis is unavailable the request is allowed
// through (fail-open) so that a Redis outage doesn't block traffic.
//
// key    — a short identifier for the endpoint (e.g., "accept", "onboard").
// limit  — maximum number of requests allowed within the window.
// window — the fixed window duration.
func RateLimit(key string, limit int, window time.Duration, rdb redis.Cmdable) gin.HandlerFunc {
	// Lua script atomically increments and sets the TTL in a single round-trip.
	// If the key is new (count == 1), it sets the expiry. This prevents the
	// scenario where INCR succeeds but a separate EXPIRE call fails, leaving
	// the key with no TTL and permanently rate-limiting the IP.
	script := redis.NewScript(`
		local count = redis.call("INCR", KEYS[1])
		if count == 1 then
			redis.call("EXPIRE", KEYS[1], ARGV[1])
		end
		return count
	`)

	return func(c *gin.Context) {
		if rdb == nil {
			c.Next()
			return
		}

		ip := c.ClientIP()
		redisKey := fmt.Sprintf("ratelimit:%s:%s", key, ip)
		windowSec := int(window.Seconds())

		count, err := script.Run(c.Request.Context(), rdb, []string{redisKey}, windowSec).Int64()
		if err != nil {
			// Fail-open: allow the request if Redis is unreachable.
			c.Next()
			return
		}

		if count > int64(limit) {
			c.Header("Retry-After", fmt.Sprintf("%d", windowSec))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, Envelope{
				Success: false,
				Errors:  []APIError{{Code: "RATE_LIMITED", Message: "too many requests, please try again later"}},
			})
			return
		}

		c.Next()
	}
}
