package sdk

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimit returns a Gin middleware that enforces a sliding-window rate limit
// per client IP using Redis. If Redis is unavailable the request is allowed
// through (fail-open) so that a Redis outage doesn't block traffic.
//
// key    — a short identifier for the endpoint (e.g., "accept", "onboard").
// limit  — maximum number of requests allowed within the window.
// window — the sliding window duration.
func RateLimit(key string, limit int, window time.Duration, rdb redis.Cmdable) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil {
			c.Next()
			return
		}

		ip := c.ClientIP()
		redisKey := fmt.Sprintf("ratelimit:%s:%s", key, ip)

		count, err := rdb.Incr(c.Request.Context(), redisKey).Result()
		if err != nil {
			// Fail-open: allow the request if Redis is unreachable.
			c.Next()
			return
		}

		// Set expiry only on the first increment to establish the window.
		if count == 1 {
			rdb.Expire(c.Request.Context(), redisKey, window)
		}

		if count > int64(limit) {
			c.Header("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, Envelope{
				Success: false,
				Errors:  []APIError{{Code: "RATE_LIMITED", Message: "too many requests, please try again later"}},
			})
			return
		}

		c.Next()
	}
}
