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
//	user, err := reader.GetUser(ctx, orgID, userID)
type IAMReader interface {
	// GetUser returns a user by UUID within an org.
	GetUser(ctx context.Context, orgID, userID uuid.UUID) (*User, error)

	// GetUserByExternalID returns a user by their IdP subject and provider.
	GetUserByExternalID(ctx context.Context, orgID uuid.UUID, provider, externalID string) (*User, error)

	// GetOrg returns an organization by ID.
	GetOrg(ctx context.Context, orgID uuid.UUID) (*Organization, error)

	// ListOrgMembers returns all members of an organization.
	ListOrgMembers(ctx context.Context, orgID uuid.UUID) ([]OrgMember, error)
}

// iamReader is the concrete implementation of IAMReader.
type iamReader struct {
	ctx sdk.Context
}

func (r *iamReader) GetUser(ctx context.Context, orgID, userID uuid.UUID) (*User, error) {
	var user User
	err := r.ctx.DB.WithContext(ctx).
		Where("id = ? AND org_id = ?", userID, orgID).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *iamReader) GetUserByExternalID(ctx context.Context, orgID uuid.UUID, provider, externalID string) (*User, error) {
	var user User
	err := r.ctx.DB.WithContext(ctx).
		Where("org_id = ? AND provider = ? AND external_id = ?", orgID, provider, externalID).
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
		Where("org_id = ?", orgID).
		Find(&members).Error
	return members, err
}
