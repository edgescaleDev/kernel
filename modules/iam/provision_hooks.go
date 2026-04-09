package iam

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

// seedDefaultRoles creates system roles (owner, admin, member, viewer) when an org is provisioned.
func (m *Module) seedDefaultRoles(ctx context.Context, payload any) error {
	evt, ok := payload.(sdk.OrgProvisionedEvent)
	if !ok {
		return nil
	}

	tx := m.ctx.DB.WithContext(ctx).Begin()
	if err := seedSystemRoles(tx, evt.OrgID); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// defaultSystemRoles returns the system roles seeded for every new org.
func defaultSystemRoles(orgID uuid.UUID) []Role {
	return []Role{
		{
			OrgID:       orgID,
			Name:        sdk.T("Owner"),
			Slug:        "owner",
			Description: sdk.T("Full organization administration"),
			IsSystem:    true,
		},
		{
			OrgID:       orgID,
			Name:        sdk.T("Admin"),
			Slug:        "admin",
			Description: sdk.T("Organization administration except billing/ownership"),
			IsSystem:    true,
		},
		{
			OrgID:       orgID,
			Name:        sdk.T("Member"),
			Slug:        "member",
			Description: sdk.T("Standard organization member"),
			IsSystem:    true,
		},
		{
			OrgID:       orgID,
			Name:        sdk.T("Viewer"),
			Slug:        "viewer",
			Description: sdk.T("Read-only access"),
			IsSystem:    true,
		},
	}
}

// seedSystemRoles creates system roles for an org within the given transaction.
// Idempotent — skips roles that already exist.
func seedSystemRoles(tx *gorm.DB, orgID uuid.UUID) error {
	for _, r := range defaultSystemRoles(orgID) {
		var existing Role
		if err := tx.Where("org_id = ? AND slug = ?", r.OrgID, r.Slug).First(&existing).Error; err != nil {
			if err := tx.Create(&r).Error; err != nil {
				return fmt.Errorf("seed role %s: %w", r.Slug, err)
			}
		}
	}
	return nil
}

// provisionOrgForUser creates an org, seeds system roles, adds the user as
// a member, and assigns the owner role — all within the given transaction.
// This is the shared provisioning logic used by all org-creation paths.
func provisionOrgForUser(tx *gorm.DB, org *Organization, userID uuid.UUID) error {
	if err := tx.Create(org).Error; err != nil {
		return fmt.Errorf("create org: %w", err)
	}

	if err := seedSystemRoles(tx, org.ID); err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}

	member := OrgMember{
		OrgID:  org.ID,
		UserID: userID,
	}
	if err := tx.Create(&member).Error; err != nil {
		return fmt.Errorf("create member: %w", err)
	}

	var ownerRole Role
	if err := tx.Where("org_id = ? AND slug = 'owner'", org.ID).First(&ownerRole).Error; err != nil {
		return fmt.Errorf("find owner role: %w", err)
	}

	userRole := UserRole{
		OrgID:  org.ID,
		UserID: userID,
		RoleID: ownerRole.ID,
	}
	if err := tx.Create(&userRole).Error; err != nil {
		return fmt.Errorf("assign owner role: %w", err)
	}

	return nil
}
