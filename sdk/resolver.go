package sdk

import (
	"context"

	"github.com/google/uuid"
)

// UserResolver resolves an authenticated identity (from an IdP) into an
// internal user with tenant-scoped permissions. The IAM module provides
// the production implementation. The kernel calls this from middleware
// without knowing anything about users, roles, or permission tables.
type UserResolver interface {
	// ResolveUser looks up the internal user for the given external identity
	// within the specified tenant, and returns their ID and permissions.
	//
	// When tenantID is uuid.Nil, the resolver should return only the
	// internal user ID without checking membership or resolving permissions.
	// This is used by global (non-tenant) routes to identify the caller.
	ResolveUser(ctx context.Context, provider, externalID string, tenantID uuid.UUID) (*ResolvedUser, error)
}

// AdminResolver resolves platform-level admin identity.
// Separated from UserResolver because not all deployments have a platform admin concept.
type AdminResolver interface {
	// ResolveAdmin looks up the internal user for the given external identity
	// and returns their platform-level permissions. The platform org is an
	// implementation detail of the resolver — the kernel doesn't know about it.
	ResolveAdmin(ctx context.Context, provider, externalID string) (*ResolvedUser, error)
}

// ResolvedUser is the result of resolving an external identity to an internal user.
type ResolvedUser struct {
	// InternalID is the kernel-internal UUID for the user.
	InternalID uuid.UUID

	// MemberID is the tenant membership record UUID (the primary key of the
	// membership row linking this user to the resolved tenant). It is uuid.Nil
	// when resolving without a tenant context (global routes) or for API keys.
	MemberID uuid.UUID

	// Permissions is the list of permission keys granted to this user
	// in the resolved context (tenant-scoped or platform-scoped).
	Permissions []string
}

// PlatformManager is an optional extension of AdminResolver that enables
// CLI commands for managing platform roles. If the AdminResolver implementation
// also satisfies PlatformManager, the kernel's "platform grant/revoke/list"
// commands delegate to it.
type PlatformManager interface {
	// GrantRole assigns a platform role to a user.
	GrantRole(ctx context.Context, userID uuid.UUID, roleSlug string) error

	// RevokeAllRoles removes all platform roles from a user.
	RevokeAllRoles(ctx context.Context, userID uuid.UUID) error

	// ListAdmins returns all users with platform roles.
	ListAdmins(ctx context.Context) ([]PlatformAdmin, error)
}

// PlatformAdmin represents a platform administrator entry for CLI display.
type PlatformAdmin struct {
	UserID string
	Name   string
	Email  string
	Role   string
}
