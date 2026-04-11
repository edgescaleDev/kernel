package kernel

import (
	"encoding/json"
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

// parseLocale extracts the Accept-Language header, takes the primary tag,
// and defaults to "en" if none is provided. It stores it in context.
func (k *Kernel) parseLocale() gin.HandlerFunc {
	return func(c *gin.Context) {
		locale := c.GetHeader("Accept-Language")
		if i := strings.IndexAny(locale, "-,;"); i > 0 {
			locale = locale[:i]
		}
		if locale == "" {
			locale = "en"
		}
		c.Set("locale", locale)
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

// resolveUser resolves the authenticated user's internal UUID from their IdP subject,
// verifies their membership in the current organization, and loads their org-scoped
// permissions into context for enforcement by sdk.RequirePermission.
// Must be used after authenticate() and resolveOrg().
func (k *Kernel) resolveUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		if k.userResolver == nil {
			sdk.Error(c, sdk.Forbidden("no user resolver configured"))
			return
		}

		sub := c.GetString("user_id")
		provider := c.GetString("auth_provider")
		if sub == "" {
			sdk.Error(c, sdk.Unauthorized("missing identity"))
			return
		}

		v, exists := c.Get("org_id")
		if !exists {
			sdk.Error(c, sdk.Internal("missing organization context"))
			return
		}
		orgID, ok := v.(uuid.UUID)
		if !ok {
			sdk.Error(c, sdk.Internal("invalid organization id in context"))
			return
		}

		// Check Redis cache for resolved user.
		cacheKey := "middleware_user:" + sub + ":" + orgID.String()
		type cachePayload struct {
			ID          uuid.UUID `json:"id"`
			Permissions []string  `json:"permissions"`
		}

		var payload cachePayload
		var cacheHit bool

		if k.redis != nil {
			if cached, err := k.redis.Get(c.Request.Context(), cacheKey).Bytes(); err == nil {
				if json.Unmarshal(cached, &payload) == nil {
					cacheHit = true
				}
			}
		}

		if !cacheHit {
			// Delegate to the user resolver (IAM module or similar).
			resolved, err := k.userResolver.ResolveUser(c.Request.Context(), provider, sub, orgID)
			if err != nil || resolved == nil {
				sdk.Error(c, sdk.Forbidden("user not found or not a member of this organization"))
				return
			}
			payload.ID = resolved.InternalID
			payload.Permissions = resolved.Permissions

			// Store in cache.
			if k.redis != nil && k.cfg.Server.CacheTTL > 0 {
				if data, err := json.Marshal(&payload); err == nil {
					k.redis.Set(c.Request.Context(), cacheKey, data, k.cfg.Server.CacheTTL)
				}
			}
		}

		// Store internal user UUID for downstream handlers.
		c.Set("internal_user_id", payload.ID)
		c.Set("permissions", sdk.NewPermissionSet(payload.Permissions))
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
		v, exists := c.Get("org_id")
		if !exists {
			sdk.Error(c, sdk.Internal("missing organization context"))
			return
		}
		orgID, ok := v.(uuid.UUID)
		if !ok {
			sdk.Error(c, sdk.BadRequest("invalid organization id"))
			return
		}

		if !k.isModuleActive(moduleID, orgID.String()) {
			sdk.Error(c, sdk.Forbidden("module not activated for this organization"))
			return
		}

		c.Next()
	}
}

// requirePlatformAdmin resolves the authenticated user's admin identity
// and gates admin route access.
// Must be used after authenticate().
func (k *Kernel) requirePlatformAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if k.adminResolver == nil {
			sdk.Error(c, sdk.Forbidden("platform admin not configured"))
			return
		}

		sub := c.GetString("user_id")
		provider := c.GetString("auth_provider")
		if sub == "" {
			sdk.Error(c, sdk.Unauthorized("missing identity"))
			return
		}

		// Delegate to the admin resolver (IAM module or similar).
		resolved, err := k.adminResolver.ResolveAdmin(c.Request.Context(), provider, sub)
		if err != nil || resolved == nil {
			sdk.Error(c, sdk.Forbidden("user not found"))
			return
		}

		if len(resolved.Permissions) == 0 {
			sdk.Error(c, sdk.Forbidden("platform admin access required"))
			return
		}

		// Store internal user UUID for downstream handlers.
		c.Set("internal_user_id", resolved.InternalID)
		c.Set("permissions", sdk.NewPermissionSet(resolved.Permissions))
		c.Next()
	}
}
