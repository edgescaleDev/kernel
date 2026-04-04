package kernel

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
)

// requestID generates a unique request ID and sets it on the context and response header.
func (k *Kernel) requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// accessLog logs each request with method, path, status, and duration.
func (k *Kernel) accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		k.logger.Log(c.Request.Context(), level, "request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"duration", duration.String(),
			"request_id", c.GetString("request_id"),
			"ip", c.ClientIP(),
		)
	}
}

// recovery catches panics in handlers and returns a 500 instead of crashing the process.
func (k *Kernel) recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				k.logger.Error("panic recovered",
					"error", err,
					"path", c.Request.URL.Path,
					"request_id", c.GetString("request_id"),
				)
				sdk.Error(c, sdk.Internal("an unexpected error occurred"))
			}
		}()
		c.Next()
	}
}

// authenticate validates the Authorization header by delegating to the
// configured IdentityProvider. On success, it stores the provider-agnostic
// Identity and convenience fields in the gin context for downstream use.
//
// Context values set on success:
//   - "identity"         → *sdk.Identity (full identity object)
//   - "user_id"          → string (IdP subject / external ID)
//   - "auth_identifier"  → string (email, phone, or other identifier)
//   - "auth_provider"    → string (e.g., "firebase", "okta")
//   - "auth_token"       → string (raw bearer token)
func (k *Kernel) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			sdk.Error(c, sdk.Unauthorized("missing Authorization header"))
			return
		}

		token := strings.TrimPrefix(header, "Bearer ")
		if token == header {
			sdk.Error(c, sdk.Unauthorized("invalid Authorization format, expected <Bearer token>"))
			return
		}

		identity, err := k.identityProvider.ValidateToken(c.Request.Context(), token)
		if err != nil {
			k.logger.Warn("authentication failed",
				"error", err.Error(),
				"request_id", c.GetString("request_id"),
			)
			sdk.Error(c, sdk.Unauthorized("invalid or expired token"))
			return
		}

		// Store the full identity object and convenience shortcuts.
		c.Set("identity", identity)
		c.Set("user_id", identity.Subject)
		c.Set("auth_identifier", identity.Identifier)
		c.Set("auth_provider", identity.Provider)
		c.Set("auth_token", token)
		c.Next()
	}
}

// resolveOrg extracts the X-Org-ID header, validates it as a UUID,
// and stores it in the gin context for downstream use.
func (k *Kernel) resolveOrg() gin.HandlerFunc {
	return func(c *gin.Context) {
		orgHeader := c.GetHeader("X-Org-ID")
		if orgHeader == "" {
			sdk.Error(c, sdk.BadRequest("missing X-Org-ID header"))
			return
		}

		orgID, err := uuid.Parse(orgHeader)
		if err != nil {
			sdk.Error(c, sdk.BadRequest("invalid organization id"))
			return
		}

		c.Set("org_id", orgID)
		c.Next()
	}
}

// moduleActivation checks whether a module is active for the requesting org.
// Core modules always pass. Feature/integration modules are checked against
// the module_activations table (cached in Redis).
func (k *Kernel) moduleActivation(moduleID string) gin.HandlerFunc {
	manifest, exists := k.manifests[moduleID]
	if !exists {
		// Should never happen - programming error.
		panic("kernel: moduleActivation called for unregistered module: " + moduleID)
	}

	// Core modules always pass.
	if manifest.Type.IsCore() {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		// TODO: Check module_activations table (Redis cached) once the
		// registry module is implemented. For now, all modules pass.
		c.Next()
	}
}

// checkPermission returns middleware that enforces the given permission string.
// Supports exact match, namespace wildcards (e.g., "orders.*"), and pipe-separated
// OR expressions (e.g., "orders.read|billing.read" - any match passes).
func (k *Kernel) checkPermission(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the user's permission set from context (set by authenticate/resolveUser).
		permsVal, exists := c.Get("permissions")
		if !exists {
			// No permissions loaded yet - allow through (will be enforced when
			// IAM populates permissions during authenticate).
			// TODO: Change to deny-by-default once IAM is wired.
			c.Next()
			return
		}

		ps, ok := permsVal.(*PermissionSet)
		if !ok {
			sdk.Error(c, sdk.Internal("invalid permission set in context"))
			return
		}

		// Check pipe-separated OR permissions.
		for perm := range strings.SplitSeq(required, "|") {
			if ps.Has(strings.TrimSpace(perm)) {
				c.Next()
				return
			}
		}

		sdk.Error(c, sdk.Forbidden("insufficient permissions"))
	}
}
