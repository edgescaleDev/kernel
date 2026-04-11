package sdk

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// RequirePermission returns middleware that enforces the given permission string.
// Supports exact match, namespace wildcards (e.g., "orders.*"), and pipe-separated
// OR expressions (e.g., "orders.read|billing.read" - any match passes).
//
// Reads the *PermissionSet from the Gin context ("permissions" key),
// which is set by the user resolver middleware during request processing.
func RequirePermission(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the user's permission set from context (set by authenticate/resolveUser).
		permsVal, exists := c.Get("permissions")
		if !exists {
			Error(c, Forbidden("insufficient permissions"))
			return
		}

		ps, ok := permsVal.(*PermissionSet)
		if !ok {
			Error(c, Internal("invalid permission set in context"))
			return
		}

		// Check pipe-separated OR permissions.
		for perm := range strings.SplitSeq(required, "|") {
			if ps.Has(strings.TrimSpace(perm)) {
				c.Next()
				return
			}
		}

		Error(c, Forbidden("insufficient permissions"))
	}
}
