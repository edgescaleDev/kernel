package sdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// compile-time checks
var _ IdentityProvider = (*IdentityProviderChain)(nil)
var _ TokenRevoker = (*IdentityProviderChain)(nil)

// IdentityProviderChain routes authentication to the correct provider
// based on credential format. It implements IdentityProvider and
// TokenRevoker.
//
// Routing strategy:
//  1. Non-JWT credentials: matched by header + prefix (e.g., "Bearer sk_")
//  2. JWT credentials: matched by decoding the issuer claim (no crypto)
//  3. No match: falls back to the fallback provider, or returns ErrNoCredentials
//
// Usage:
//
//	chain := sdk.NewIdentityProviderChain()
//	chain.AddPrefix("apikey", "Authorization", "Bearer sk_", apiKeyProvider)
//	chain.AddJWTIssuer("firebase", "https://securetoken.google.com/proj", fbProvider)
//	chain.SetFallback("firebase", fbProvider)
//	k.SetIdentityProvider(chain)
type IdentityProviderChain struct {
	mu sync.RWMutex

	// prefixEntries are checked first, in order. For non-JWT credentials
	// (API keys, Basic auth, custom headers).
	prefixEntries []prefixEntry

	// issuerMap routes JWTs by their iss claim. Checked when a Bearer token
	// is present but no prefix entry matched.
	issuerMap map[string]namedProvider

	// fallback is called if no prefix entry and no issuer match.
	// Useful for single-IdP deployments that don't set an issuer.
	fallback *namedProvider
}

type prefixEntry struct {
	name     string
	header   string // HTTP header name, e.g., "Authorization"
	prefix   string // Value prefix, e.g., "Bearer sk_"
	provider IdentityProvider
}

type namedProvider struct {
	name     string
	provider IdentityProvider
}

// NewIdentityProviderChain creates an empty chain.
func NewIdentityProviderChain() *IdentityProviderChain {
	return &IdentityProviderChain{
		issuerMap: make(map[string]namedProvider),
	}
}

// AddPrefix registers a provider matched by header + value prefix.
// Prefix entries are checked in registration order, before JWT issuer routing.
// Use for non-JWT credentials (API keys, Basic auth, etc.).
func (c *IdentityProviderChain) AddPrefix(name, header, prefix string, p IdentityProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prefixEntries = append(c.prefixEntries, prefixEntry{
		name:     name,
		header:   header,
		prefix:   prefix,
		provider: p,
	})
}

// AddJWTIssuer registers a JWT provider matched by the token's iss claim.
// The issuer is extracted by decoding the JWT payload (base64, no crypto).
// Multiple JWT providers can coexist (Firebase, Keycloak, Okta, etc.).
func (c *IdentityProviderChain) AddJWTIssuer(name, issuer string, p IdentityProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.issuerMap[issuer] = namedProvider{name: name, provider: p}
}

// SetFallback registers a provider used when no prefix or issuer matches.
// Useful for backward compatibility or single-provider deployments.
func (c *IdentityProviderChain) SetFallback(name string, p IdentityProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fallback = &namedProvider{name: name, provider: p}
}

// RegisteredProviders returns the names of all registered providers.
// Used by IAM to validate provider names at write time.
func (c *IdentityProviderChain) RegisteredProviders() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	seen := make(map[string]bool)
	var names []string

	for _, e := range c.prefixEntries {
		if !seen[e.name] {
			names = append(names, e.name)
			seen[e.name] = true
		}
	}
	for _, np := range c.issuerMap {
		if !seen[np.name] {
			names = append(names, np.name)
			seen[np.name] = true
		}
	}
	if c.fallback != nil && !seen[c.fallback.name] {
		names = append(names, c.fallback.name)
	}
	return names
}

// Authenticate routes to the correct provider based on credential format.
//
// Phase 1: Check prefix entries (API keys, custom headers).
// Phase 2: Check for Bearer JWT and route by issuer.
// Phase 3: Try fallback provider.
// Phase 4: Return ErrNoCredentials.
func (c *IdentityProviderChain) Authenticate(ctx context.Context, headers http.Header) (*Identity, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Phase 1: Check prefix entries (API keys, custom headers).
	for _, e := range c.prefixEntries {
		val := headers.Get(e.header)
		if val != "" && strings.HasPrefix(val, e.prefix) {
			return e.provider.Authenticate(ctx, headers)
		}
	}

	// Phase 2: Check for Bearer JWT and route by issuer.
	authHeader := headers.Get("Authorization")
	if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
		token := after

		issuer, err := extractJWTIssuer(token)
		if err == nil && issuer != "" {
			if entry, ok := c.issuerMap[issuer]; ok {
				return entry.provider.Authenticate(ctx, headers)
			}
		}

		// JWT but unknown issuer - try fallback.
		if c.fallback != nil {
			return c.fallback.provider.Authenticate(ctx, headers)
		}
	}

	// Phase 3: Non-Bearer header present but no match - try fallback.
	if authHeader != "" && c.fallback != nil {
		return c.fallback.provider.Authenticate(ctx, headers)
	}

	// Phase 4: No credentials found.
	return nil, ErrNoCredentials
}

// RevokeToken routes revocation to the correct provider that implements
// TokenRevoker. Only JWT providers typically need revocation; API key
// providers handle it through their management API.
func (c *IdentityProviderChain) RevokeToken(ctx context.Context, token string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try JWT issuer routing.
	if issuer, err := extractJWTIssuer(token); err == nil && issuer != "" {
		if entry, ok := c.issuerMap[issuer]; ok {
			if revoker, ok := entry.provider.(TokenRevoker); ok {
				return revoker.RevokeToken(ctx, token)
			}
			return nil // provider doesn't support revocation
		}
	}

	if c.fallback != nil {
		if revoker, ok := c.fallback.provider.(TokenRevoker); ok {
			return revoker.RevokeToken(ctx, token)
		}
	}

	return nil
}

// extractJWTIssuer decodes the JWT payload (base64, no verification)
// and reads the "iss" claim. This is a ~microsecond operation with
// no crypto involved - just base64 decode + JSON unmarshal.
func extractJWTIssuer(token string) (string, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("not a JWT")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}

	var claims struct {
		Issuer string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}

	return claims.Issuer, nil
}
