package iam

import (
	"context"

	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
)

// IAMReader provides read-only access to IAM data for other modules.
// Other modules obtain this reader via the kernel's cross-module reader registry
// without needing direct database access or cross-module imports.
//
// Usage from another module:
//
//	reader, err := sdk.Reader[iam.IAMReader](&ctx, "iam")
//	user, err := reader.GetUser(ctx, userID)
type IAMReader interface {
	// GetUser returns a user by UUID.
	GetUser(ctx context.Context, userID uuid.UUID) (*User, error)

	// GetUserByExternalID returns a user by their IdP subject and provider.
	GetUserByExternalID(ctx context.Context, provider, externalID string) (*User, error)

	// GetOrg returns an organization by ID.
	GetOrg(ctx context.Context, orgID uuid.UUID) (*Organization, error)

	// ListOrgMembers returns all members of an organization.
	ListOrgMembers(ctx context.Context, orgID uuid.UUID) ([]OrgMember, error)

	// IsOrgMember checks if a user is a member of the given org.
	IsOrgMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error)

	// GetUserPermissions returns all permission keys for a user in a specific org.
	// Resolves through user_roles → role_permissions.
	GetUserPermissions(ctx context.Context, userID, orgID uuid.UUID) ([]string, error)
}

// iamReader is the concrete implementation of IAMReader.
type iamReader struct {
	ctx sdk.Context
}

func (r *iamReader) GetUser(ctx context.Context, userID uuid.UUID) (*User, error) {
	var user User
	err := r.ctx.DB.WithContext(ctx).
		Where("id = ?", userID).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *iamReader) GetUserByExternalID(ctx context.Context, provider, externalID string) (*User, error) {
	var user User
	err := r.ctx.DB.WithContext(ctx).
		Where("provider = ? AND external_id = ?", provider, externalID).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *iamReader) GetOrg(ctx context.Context, orgID uuid.UUID) (*Organization, error) {
	var org Organization
	err := r.ctx.DB.WithContext(ctx).
		Where("id = ?", orgID).
		First(&org).Error
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func (r *iamReader) ListOrgMembers(ctx context.Context, orgID uuid.UUID) ([]OrgMember, error) {
	var members []OrgMember
	err := r.ctx.DB.WithContext(ctx).
		Preload("User").
		Where("org_id = ?", orgID).
		Limit(1000).
		Find(&members).Error
	return members, err
}

func (r *iamReader) IsOrgMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	var count int64
	err := r.ctx.DB.WithContext(ctx).
		Model(&OrgMember{}).
		Where("user_id = ? AND org_id = ?", userID, orgID).
		Count(&count).Error
	return count > 0, err
}

func (r *iamReader) GetUserPermissions(ctx context.Context, userID, orgID uuid.UUID) ([]string, error) {
	var keys []string
	err := r.ctx.DB.WithContext(ctx).
		Model(&RolePermission{}).
		Select("DISTINCT module_iam.role_permissions.permission_key").
		Joins("JOIN module_iam.user_roles ON module_iam.user_roles.role_id = module_iam.role_permissions.role_id").
		Where("module_iam.user_roles.user_id = ? AND module_iam.user_roles.org_id = ?", userID, orgID).
		Pluck("permission_key", &keys).Error
	return keys, err
}
