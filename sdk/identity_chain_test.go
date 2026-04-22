package sdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// mockProvider returns a fixed identity for any request with the right header.
type mockProvider struct {
	name     string
	identity *Identity
	err      error
}

func (p *mockProvider) Authenticate(_ context.Context, headers http.Header) (*Identity, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.identity, nil
}

// mockRevokerProvider implements both IdentityProvider and TokenRevoker.
type mockRevokerProvider struct {
	mockProvider
	revoked []string
}

func (p *mockRevokerProvider) RevokeToken(_ context.Context, token string) error {
	p.revoked = append(p.revoked, token)
	return nil
}

// buildTestJWT creates a minimal JWT with the given issuer for testing.
// The signature is garbage - only the payload matters for routing.
func buildTestJWT(t *testing.T, issuer string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := map[string]any{"iss": issuer, "sub": "user-1", "exp": time.Now().Add(time.Hour).Unix()}
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + payloadB64 + ".fake-signature"
}

// ── Chain: Authenticate ───────────────────────────────────────────────────────

func TestChain_PrefixMatch_RoutesToCorrectProvider(t *testing.T) {
	apiKeyProvider := &mockProvider{
		name: "apikey",
		identity: &Identity{
			Subject:  "key-abc",
			Provider: "apikey",
			Kind:     IdentityKindAPIKey,
		},
	}

	chain := NewIdentityProviderChain()
	chain.AddPrefix("apikey", "Authorization", "Bearer sk_", apiKeyProvider)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer sk_test123")

	identity, err := chain.Authenticate(context.Background(), headers)
	require.NoError(t, err)
	assert.Equal(t, "key-abc", identity.Subject)
	assert.Equal(t, "apikey", identity.Provider)
	assert.Equal(t, IdentityKindAPIKey, identity.Kind)
}

func TestChain_JWTIssuer_RoutesToCorrectProvider(t *testing.T) {
	fbProvider := &mockProvider{
		name: "firebase",
		identity: &Identity{
			Subject:  "user-123",
			Provider: "firebase",
			Kind:     IdentityKindUser,
		},
	}
	kcProvider := &mockProvider{
		name: "keycloak",
		identity: &Identity{
			Subject:  "user-456",
			Provider: "keycloak",
			Kind:     IdentityKindUser,
		},
	}

	chain := NewIdentityProviderChain()
	chain.AddJWTIssuer("firebase", "https://securetoken.google.com/my-proj", fbProvider)
	chain.AddJWTIssuer("keycloak", "https://kc.example.com/realms/main", kcProvider)

	t.Run("firebase issuer", func(t *testing.T) {
		token := buildTestJWT(t, "https://securetoken.google.com/my-proj")
		headers := http.Header{}
		headers.Set("Authorization", "Bearer "+token)

		identity, err := chain.Authenticate(context.Background(), headers)
		require.NoError(t, err)
		assert.Equal(t, "user-123", identity.Subject)
		assert.Equal(t, "firebase", identity.Provider)
	})

	t.Run("keycloak issuer", func(t *testing.T) {
		token := buildTestJWT(t, "https://kc.example.com/realms/main")
		headers := http.Header{}
		headers.Set("Authorization", "Bearer "+token)

		identity, err := chain.Authenticate(context.Background(), headers)
		require.NoError(t, err)
		assert.Equal(t, "user-456", identity.Subject)
		assert.Equal(t, "keycloak", identity.Provider)
	})
}

func TestChain_UnknownIssuer_UsesFallback(t *testing.T) {
	fbProvider := &mockProvider{
		name: "firebase",
		identity: &Identity{
			Subject:  "user-fallback",
			Provider: "firebase",
			Kind:     IdentityKindUser,
		},
	}

	chain := NewIdentityProviderChain()
	chain.SetFallback("firebase", fbProvider)

	token := buildTestJWT(t, "https://unknown-idp.example.com")
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)

	identity, err := chain.Authenticate(context.Background(), headers)
	require.NoError(t, err)
	assert.Equal(t, "user-fallback", identity.Subject)
}

func TestChain_NoCredentials_ReturnsErrNoCredentials(t *testing.T) {
	chain := NewIdentityProviderChain()

	headers := http.Header{}
	_, err := chain.Authenticate(context.Background(), headers)
	assert.True(t, errors.Is(err, ErrNoCredentials))
}

func TestChain_ProviderError_StopsChain(t *testing.T) {
	apiKeyProvider := &mockProvider{
		name: "apikey",
		err:  errors.New("apikeys: key expired"),
	}

	chain := NewIdentityProviderChain()
	chain.AddPrefix("apikey", "Authorization", "Bearer sk_", apiKeyProvider)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer sk_expired_key")

	_, err := chain.Authenticate(context.Background(), headers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key expired")
	assert.False(t, errors.Is(err, ErrNoCredentials))
}

func TestChain_PrefixBeforeJWT(t *testing.T) {
	// Ensure prefix entries are checked before JWT parsing.
	apiKeyProvider := &mockProvider{
		name: "apikey",
		identity: &Identity{
			Subject:  "key-hit",
			Provider: "apikey",
			Kind:     IdentityKindAPIKey,
		},
	}
	fbProvider := &mockProvider{
		name: "firebase",
		identity: &Identity{
			Subject:  "should-not-reach",
			Provider: "firebase",
		},
	}

	chain := NewIdentityProviderChain()
	chain.AddPrefix("apikey", "Authorization", "Bearer sk_", apiKeyProvider)
	chain.AddJWTIssuer("firebase", "https://securetoken.google.com/proj", fbProvider)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer sk_test")

	identity, err := chain.Authenticate(context.Background(), headers)
	require.NoError(t, err)
	assert.Equal(t, "key-hit", identity.Subject)
	assert.Equal(t, "apikey", identity.Provider)
}

func TestChain_NoFallback_NoMatch_ReturnsErrNoCredentials(t *testing.T) {
	chain := NewIdentityProviderChain()
	chain.AddJWTIssuer("firebase", "https://securetoken.google.com/proj", &mockProvider{
		name:     "firebase",
		identity: &Identity{Subject: "user"},
	})

	// Send a JWT with an unknown issuer, no fallback set.
	token := buildTestJWT(t, "https://unknown.example.com")
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)

	_, err := chain.Authenticate(context.Background(), headers)
	assert.True(t, errors.Is(err, ErrNoCredentials))
}

// ── Chain: RegisteredProviders ────────────────────────────────────────────────

func TestChain_RegisteredProviders(t *testing.T) {
	chain := NewIdentityProviderChain()
	chain.AddPrefix("apikey", "Authorization", "Bearer sk_", &mockProvider{name: "apikey"})
	chain.AddJWTIssuer("firebase", "https://securetoken.google.com/proj", &mockProvider{name: "firebase"})
	chain.AddJWTIssuer("keycloak", "https://kc.example.com/realms/main", &mockProvider{name: "keycloak"})
	chain.SetFallback("firebase", &mockProvider{name: "firebase"}) // duplicate - should not appear twice

	providers := chain.RegisteredProviders()
	assert.ElementsMatch(t, []string{"apikey", "firebase", "keycloak"}, providers)
}

func TestChain_RegisteredProviders_Empty(t *testing.T) {
	chain := NewIdentityProviderChain()
	providers := chain.RegisteredProviders()
	assert.Empty(t, providers)
}

// ── Chain: RevokeToken (TokenRevoker) ─────────────────────────────────────────

func TestChain_RevokeToken_RoutesToJWTProvider(t *testing.T) {
	fbProvider := &mockRevokerProvider{
		mockProvider: mockProvider{name: "firebase"},
	}

	chain := NewIdentityProviderChain()
	chain.AddJWTIssuer("firebase", "https://securetoken.google.com/proj", fbProvider)

	token := buildTestJWT(t, "https://securetoken.google.com/proj")
	err := chain.RevokeToken(context.Background(), token)
	require.NoError(t, err)
	assert.Len(t, fbProvider.revoked, 1)
	assert.Equal(t, token, fbProvider.revoked[0])
}

func TestChain_RevokeToken_SkipsNonRevokerProvider(t *testing.T) {
	// Provider that implements IdentityProvider but NOT TokenRevoker.
	plainProvider := &mockProvider{name: "plain"}

	chain := NewIdentityProviderChain()
	chain.AddJWTIssuer("plain", "https://plain.example.com", plainProvider)

	token := buildTestJWT(t, "https://plain.example.com")
	err := chain.RevokeToken(context.Background(), token)
	assert.NoError(t, err) // should not error, just no-op
}

func TestChain_RevokeToken_UsesFallback(t *testing.T) {
	fbProvider := &mockRevokerProvider{
		mockProvider: mockProvider{name: "firebase"},
	}

	chain := NewIdentityProviderChain()
	chain.SetFallback("firebase", fbProvider)

	token := buildTestJWT(t, "https://unknown.example.com")
	err := chain.RevokeToken(context.Background(), token)
	require.NoError(t, err)
	assert.Len(t, fbProvider.revoked, 1)
}

func TestChain_RevokeToken_NonJWT_NoOp(t *testing.T) {
	chain := NewIdentityProviderChain()
	// Not a JWT, no prefix match - should silently succeed.
	err := chain.RevokeToken(context.Background(), "sk_not_a_jwt_token")
	assert.NoError(t, err)
}

// ── extractJWTIssuer ──────────────────────────────────────────────────────────

func TestExtractJWTIssuer_Valid(t *testing.T) {
	token := buildTestJWT(t, "https://securetoken.google.com/my-proj")
	issuer, err := extractJWTIssuer(token)
	require.NoError(t, err)
	assert.Equal(t, "https://securetoken.google.com/my-proj", issuer)
}

func TestExtractJWTIssuer_NotAJWT(t *testing.T) {
	_, err := extractJWTIssuer("not-a-jwt-at-all")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a JWT")
}

func TestExtractJWTIssuer_MalformedBase64(t *testing.T) {
	_, err := extractJWTIssuer("aaa.!!!invalid-base64!!!.ccc")
	assert.Error(t, err)
}

func TestExtractJWTIssuer_MalformedJSON(t *testing.T) {
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	_, err := extractJWTIssuer("aaa." + notJSON + ".ccc")
	assert.Error(t, err)
}

func TestExtractJWTIssuer_MissingIssuer(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"sub": "user-1"})
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	token := "aaa." + payloadB64 + ".ccc"

	issuer, err := extractJWTIssuer(token)
	require.NoError(t, err)
	assert.Empty(t, issuer)
}
