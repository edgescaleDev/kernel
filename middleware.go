package kernel

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/edgescaleDev/kernel/sdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
// and defaults to "base" if none is provided. It stores it in context.
func (k *Kernel) parseLocale() gin.HandlerFunc {
	return func(c *gin.Context) {
		locale := c.GetHeader("Accept-Language")
		if i := strings.IndexAny(locale, "-,;"); i > 0 {
			locale = locale[:i]
		}
		if locale == "" {
			locale = "base"
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
//
//   - "identity"         -> *sdk.Identity (full identity object)
//   - "user_id"          -> string (IdP subject / external ID)
//   - "auth_identifier"  -> string (email, phone, key name, etc.)
//   - "auth_provider"    -> string (e.g., "firebase", "okta", "apikey")
//   - "auth_token"       -> string (raw credential, never logged)
func (k *Kernel) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := k.identityProvider.Authenticate(
			c.Request.Context(), c.Request.Header,
		)
		if err != nil {
			if errors.Is(err, sdk.ErrNoCredentials) {
				sdk.Error(c, sdk.Unauthorized("missing credentials"))
			} else {
				k.logger.Warn("authentication failed",
					"error", err.Error(),
					"request_id", c.GetString("request_id"),
				)
				sdk.Error(c, sdk.Unauthorized("invalid or expired credentials"))
			}
			return
		}

		// Store the full identity object and convenience shortcuts.
		c.Set("identity", identity)
		c.Set("user_id", identity.Subject)
		c.Set("auth_identifier", identity.Identifier)
		c.Set("auth_provider", identity.Provider)
		c.Set("auth_token", identity.RawCredential)
		c.Next()
	}
}

// resolveTenant extracts the tenant ID from the URL path parameter (:tenant_id)
// or falls back to the X-Tenant-ID header. Validates it as a UUID and stores
// it in the gin context for downstream use.
//
// Resolution order:
//  1. URL path parameter :tenant_id (primary — Cloudflare-style)
//  2. X-Tenant-ID header (fallback — for clients that can't use path routing)
func (k *Kernel) resolveTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantStr := c.Param("tenant_id")
		if tenantStr == "" {
			tenantStr = c.GetHeader("X-Tenant-ID")
		}
		if tenantStr == "" {
			sdk.Error(c, sdk.BadRequest("missing tenant id"))
			return
		}

		tenantID, err := uuid.Parse(tenantStr)
		if err != nil {
			sdk.Error(c, sdk.BadRequest("invalid tenant id"))
			return
		}

		c.Set("tenant_id", tenantID)
		c.Next()
	}
}

// resolveUser resolves the authenticated identity into a tenant-scoped context.
//
// For human users (JWT): resolves the internal UUID from their IdP subject,
// verifies membership in the current tenant, and loads tenant-scoped permissions.
//
// For API keys: extracts the tenant_id and scopes directly from the Identity.Claims.
// Sets internal_user_id to uuid.Nil (service accounts will be added later).
//
// Must be used after authenticate() and resolveTenant().
func (k *Kernel) resolveUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		identityVal, exists := c.Get("identity")
		if !exists {
			sdk.Error(c, sdk.Unauthorized("missing identity"))
			return
		}
		identity, ok := identityVal.(*sdk.Identity)
		if !ok || identity == nil {
			sdk.Error(c, sdk.Unauthorized("invalid identity"))
			return
		}

		// ── API key path ──────────────────────────────────────────
		if identity.Kind == sdk.IdentityKindAPIKey {
			// API keys carry their own tenant + scopes in Claims.
			tenantIDStr, ok := identity.Claims["tenant_id"].(string)
			if !ok || tenantIDStr == "" {
				sdk.Error(c, sdk.Unauthorized("invalid api key: missing tenant_id in claims"))
				return
			}
			keyTenantID, err := uuid.Parse(tenantIDStr)
			if err != nil {
				sdk.Error(c, sdk.Unauthorized("invalid api key: malformed tenant_id"))
				return
			}

			// Verify the key's tenant matches the tenant in the URL (if present).
			if v, urlExists := c.Get("tenant_id"); urlExists {
				if urlTenantID := v.(uuid.UUID); urlTenantID != keyTenantID {
					sdk.Error(c, sdk.Forbidden("api key does not belong to this tenant"))
					return
				}
			}
			c.Set("tenant_id", keyTenantID)
			c.Set("internal_user_id", uuid.Nil)
			c.Set("identity_kind", string(sdk.IdentityKindAPIKey))

			scopes := extractStringSlice(identity.Claims["scopes"])
			c.Set("permissions", sdk.NewPermissionSet(scopes))
			c.Next()
			return
		}

		// ── Human user path (existing logic) ──────────────────────
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

		v, exists := c.Get("tenant_id")
		if !exists {
			sdk.Error(c, sdk.Internal("missing tenant context"))
			return
		}
		tenantID, ok := v.(uuid.UUID)
		if !ok {
			sdk.Error(c, sdk.Internal("invalid tenant id in context"))
			return
		}

		// Check Redis cache for resolved user.
		cacheKey := "middleware_user:" + sub + ":" + tenantID.String()
		type cachePayload struct {
			ID          uuid.UUID `json:"id"`
			MemberID    uuid.UUID `json:"member_id"`
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
			resolved, err := k.userResolver.ResolveUser(c.Request.Context(), provider, sub, tenantID)
			if err != nil || resolved == nil {
				k.logger.Warn("user resolution failed",
					"provider", provider,
					"subject", sub,
					"tenant_id", tenantID.String(),
					"error", err,
					"request_id", c.GetString("request_id"),
				)
				sdk.Error(c, sdk.Forbidden("user not found or not a member of this tenant"))
				return
			}
			payload.ID = resolved.InternalID
			payload.MemberID = resolved.MemberID
			payload.Permissions = resolved.Permissions

			// Store in cache.
			if k.redis != nil && k.cfg.Server.CacheTTL > 0 {
				if data, err := json.Marshal(&payload); err == nil {
					k.redis.Set(c.Request.Context(), cacheKey, data, k.cfg.Server.CacheTTL)
				}
			}
		}

		// Store internal user UUID and tenant membership ID for downstream handlers.
		c.Set("internal_user_id", payload.ID)
		c.Set("tenant_member_id", payload.MemberID)
		c.Set("permissions", sdk.NewPermissionSet(payload.Permissions))
		c.Next()
	}
}

// resolveGlobalUser resolves the authenticated identity into an internal
// user UUID for global (non-tenant) routes. Unlike resolveUser, this does
// not require a tenant context and does not resolve permissions.
//
// It calls UserResolver.ResolveUser with uuid.Nil as the tenant ID, which
// signals the resolver to return only the internal user ID without checking
// membership or resolving permissions.
//
// Sets internal_user_id in the gin context. If no UserResolver is
// configured, the middleware is a no-op (internal_user_id will be absent).
//
// Must be used after authenticate().
func (k *Kernel) resolveGlobalUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		if k.userResolver == nil {
			c.Next()
			return
		}

		sub := c.GetString("user_id")
		provider := c.GetString("auth_provider")
		if sub == "" {
			c.Next()
			return
		}

		// Check Redis cache for the resolved identity.
		cacheKey := "middleware_identity:" + provider + ":" + sub
		var internalID uuid.UUID
		var cacheHit bool

		if k.redis != nil {
			if cached, err := k.redis.Get(c.Request.Context(), cacheKey).Result(); err == nil {
				if parsed, parseErr := uuid.Parse(cached); parseErr == nil {
					internalID = parsed
					cacheHit = true
				}
			}
		}

		if !cacheHit {
			resolved, err := k.userResolver.ResolveUser(c.Request.Context(), provider, sub, uuid.Nil)
			if err != nil || resolved == nil || resolved.InternalID == uuid.Nil {
				// User does not exist yet (e.g., first-time IdP user).
				// Allow the request to proceed without internal_user_id.
				c.Next()
				return
			}
			internalID = resolved.InternalID

			// Store in cache.
			if k.redis != nil && k.cfg.Server.CacheTTL > 0 {
				k.redis.Set(c.Request.Context(), cacheKey, internalID.String(), k.cfg.Server.CacheTTL)
			}
		}

		c.Set("internal_user_id", internalID)
		c.Next()
	}
}

// moduleActivation checks whether a module is active for the requesting tenant.
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
		v, exists := c.Get("tenant_id")
		if !exists {
			sdk.Error(c, sdk.Internal("missing tenant context"))
			return
		}
		tenantID, ok := v.(uuid.UUID)
		if !ok {
			sdk.Error(c, sdk.BadRequest("invalid tenant id"))
			return
		}

		if !k.isModuleActive(moduleID, tenantID.String()) {
			sdk.Error(c, sdk.Forbidden("module not activated for this tenant"))
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

// extractStringSlice coerces a claim value into []string.
// Handles the common representations produced by JSON decoding and
// typed providers:
//   - []string  (typed provider)
//   - []any     (json.Unmarshal into map[string]any)
//   - string    (comma-delimited, e.g. "read,write")
//
// Returns nil if the value is nil or an unrecognised type (fail-closed).
func extractStringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		out := make([]string, 0, len(val))
		for _, elem := range val {
			if s, ok := elem.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if val == "" {
			return nil
		}
		return strings.Split(val, ",")
	default:
		return nil
	}
}
