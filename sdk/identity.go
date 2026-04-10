package sdk

import (
	"context"
	"time"
)

// IdentityProvider validates bearer tokens and returns a canonical identity.
// The kernel's authenticate() middleware delegates all token verification to
// this interface, making the auth layer completely IdP-agnostic.
//
// Implementations exist per provider (Firebase, Okta, Keycloak, Auth0, etc.)
// and are injected by the consumer at boot time via kernel.SetIdentityProvider().
//
// Usage (consumer main.go):
//
//	k := kernel.New(cfg)
//	k.SetIdentityProvider(firebase.New(firebase.Config{ProjectID: "my-project"}))
//	// or: k.SetIdentityProvider(okta.New(okta.Config{Domain: "dev-123.okta.com"}))
type IdentityProvider interface {
	// ValidateToken verifies a bearer token and returns the authenticated identity.
	// Implementations should handle signature verification, expiry checks, issuer
	// validation, and any caching strategy (e.g., Redis).
	//
	// Returns a non-nil error if the token is invalid, expired, revoked, or
	// verification fails for any reason. The error message should be safe to
	// log but MUST NOT be returned to the client (the kernel returns a generic
	// "unauthorized" message instead).
	ValidateToken(ctx context.Context, token string) (*Identity, error)

	// RevokeToken marks a token as revoked so that subsequent ValidateToken
	// calls reject it. Implementations typically store the token hash in Redis
	// with a TTL matching the token's remaining lifetime.
	RevokeToken(ctx context.Context, token string) error
}

// Identity is the canonical, provider-agnostic result of token validation.
// Every IdP (Firebase, Okta, Keycloak, Auth0) maps its token claims into
// this common shape. Downstream handlers never need to know which IdP issued
// the token — they work exclusively with this struct.
//
// The kernel's authenticate() middleware stores this as c.Set("identity", identity)
// and extracts convenience fields:
//
//	c.Set("user_id", identity.Subject)
//	c.Set("auth_provider", identity.Provider)
type Identity struct {
	// Subject is the unique user identifier from the IdP.
	// Firebase: UID, Okta: sub claim, Keycloak: sub claim.
	// This maps to the `external_id` column in the IAM users table.
	Subject string

	// Identifier is the value the user authenticated with.
	// Its format depends on SignInMethod:
	//   "phone"      → E.164 phone number (e.g., "+966501234567")
	//   "password"   → email address
	//   "google.com" → email address
	//   "apple.com"  → Apple relay email
	//   "saml"       → SAML NameID or UPN
	//   "webauthn"   → credential ID
	//
	// This is a single, generic field — SignInMethod tells you how to interpret it.
	Identifier string

	// Verified indicates whether the IdP has confirmed the Identifier.
	// For phone auth this is always true (OTP is the proof).
	// For email/password it depends on whether the user clicked the verification link.
	Verified bool

	// Provider identifies which IdP issued this token.
	// Examples: "firebase", "okta", "keycloak", "auth0".
	// Used by the IAM module when creating/matching user records.
	Provider string

	// SignInMethod is the specific authentication method used.
	// Examples: "password", "phone", "google.com", "saml", "oidc".
	// Determines how to interpret Identifier and is used for
	// per-org sign-in provider policy enforcement.
	SignInMethod string

	// Claims holds the full decoded token claims for provider-specific logic.
	// Handlers should prefer the typed fields above; use Claims only when
	// accessing provider-specific data not covered by the canonical fields
	// (e.g., a linked email when the user signed in with phone).
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
