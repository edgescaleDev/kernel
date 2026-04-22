package sdk

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// IdentityProvider validates credentials from HTTP headers and returns
// a canonical identity. Implementations handle one auth scheme each
// (JWT verification, API key lookup, etc.).
//
// Providers are composed via IdentityProviderChain, which handles
// routing. Individual providers can assume that if they are called,
// the request contains credentials they should handle.
//
// Usage (consumer main.go):
//
//	chain := NewIdentityProviderChain()
//	chain.AddPrefix("apikey", "Authorization", "Bearer sk_", apiKeyProvider)
//	chain.AddJWTIssuer("firebase", "https://securetoken.google.com/proj", fbProvider)
//	k.SetIdentityProvider(chain)
type IdentityProvider interface {
	// Authenticate extracts credentials from HTTP headers and validates them.
	//
	// Returns a non-nil error if credentials are present but invalid
	// (expired, revoked, bad signature). The error is logged but never
	// returned to the client.
	//
	// Returns ErrNoCredentials if no recognizable credentials are found
	// in the headers (e.g., missing Authorization header, wrong prefix).
	Authenticate(ctx context.Context, headers http.Header) (*Identity, error)
}

// TokenRevoker is an optional capability for providers that support
// token blocklisting (e.g., JWT providers storing revoked tokens in Redis).
// API key providers don't need this - revocation is handled through their
// management API (DELETE /keys/:id).
//
// The IdentityProviderChain implements TokenRevoker by routing to the
// first matching provider that also implements TokenRevoker.
//
// Consumers use type assertion:
//
//	if revoker, ok := m.ctx.IdentityProvider.(sdk.TokenRevoker); ok {
//	    revoker.RevokeToken(ctx, token)
//	}
type TokenRevoker interface {
	RevokeToken(ctx context.Context, token string) error
}

// ErrNoCredentials is returned by IdentityProvider.Authenticate when no
// recognizable credentials are found in the request headers. The middleware
// maps this to a 401 with "missing credentials".
var ErrNoCredentials = errors.New("no credentials found")

// IdentityKind distinguishes human users from machine identities.
type IdentityKind string

const (
	// IdentityKindUser is the default kind for human users (JWT tokens).
	IdentityKindUser IdentityKind = "user"

	// IdentityKindAPIKey is used for machine-to-machine API key identities.
	IdentityKindAPIKey IdentityKind = "apikey"
)

// Identity is the canonical, provider-agnostic result of authentication.
// Every IdP (Firebase, Okta, Keycloak, Auth0) and every credential type
// (JWT, API key) maps its claims into this common shape. Downstream handlers
// never need to know which provider issued the credential - they work
// exclusively with this struct.
//
// The kernel's authenticate() middleware stores this as c.Set("identity", identity)
// and extracts convenience fields:
//
//	c.Set("user_id", identity.Subject)
//	c.Set("auth_provider", identity.Provider)
//	c.Set("auth_token", identity.RawCredential)
type Identity struct {
	// Subject is the unique identifier from the provider.
	// JWT: UID/sub claim. API key: key UUID.
	// This maps to the `external_id` column in the IAM users table (for JWTs)
	// or is the key ID itself (for API keys).
	Subject string

	// Identifier is the value the user/key authenticated with.
	// JWT: email, phone, SAML NameID. API key: key name.
	Identifier string

	// Verified indicates whether the provider has confirmed the Identifier.
	// For phone auth this is always true (OTP is the proof).
	// For email/password it depends on whether the user clicked the verification link.
	// For API keys this is always true (the key itself is the proof).
	Verified bool

	// Provider identifies which IdP or auth mechanism issued this identity.
	// Examples: "firebase", "okta", "keycloak", "auth0", "apikey".
	// Used by the IAM module when creating/matching user records and for
	// per-tenant provider policy enforcement.
	Provider string

	// SignInMethod is the specific authentication method used.
	// Examples: "password", "phone", "google.com", "saml", "oidc", "apikey".
	// Determines how to interpret Identifier and is used for
	// per-tenant sign-in provider policy enforcement.
	SignInMethod string

	// Kind distinguishes human users from machine identities.
	// Defaults to IdentityKindUser for JWT providers.
	// Set to IdentityKindAPIKey for API key providers.
	// The kernel middleware uses this to choose the right resolveUser path.
	Kind IdentityKind

	// RawCredential is the raw credential string extracted by the provider.
	// For JWTs: the full token. For API keys: the raw key (e.g., "sk_abc...").
	// Stored in context as "auth_token". Never logged or returned to clients.
	RawCredential string `json:"-"`

	// Claims holds the full decoded token claims for provider-specific logic.
	// Handlers should prefer the typed fields above; use Claims only when
	// accessing provider-specific data not covered by the canonical fields
	// (e.g., a linked email when the user signed in with phone, or API key
	// scopes and tenant_id).
	Claims map[string]any

	// ExpiresAt is the token's expiration time.
	// Used by caching layers to set appropriate TTLs.
	ExpiresAt time.Time
}

// UserProfileReader provides cross-module user profile resolution.
// The kernel's /v1/me handler uses this interface (via the reader registry)
// to fetch the authenticated user's full profile without importing any
// specific module. Any identity module (e.g., IAM) satisfies this by
// registering a reader that implements these methods.
type UserProfileReader interface {
	// GetUserByExternalID resolves a user by their IdP provider and subject.
	// Returns the full user profile (module-specific struct) or an error.
	GetUserByExternalID(ctx context.Context, provider, externalID string) (any, error)
}
