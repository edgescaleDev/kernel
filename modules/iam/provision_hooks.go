package iam

import (
	"context"

	"go.edgescale.dev/kernel/sdk"
)

// seedDefaultRoles creates system roles (owner, admin, member, viewer) when an org is provisioned.
func (m *Module) seedDefaultRoles(ctx context.Context, payload any) error {
	evt, ok := payload.(sdk.OrgProvisionedEvent)
	if !ok {
		return nil
	}
	orgID := evt.OrgID

	roles := []Role{
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

	tx := m.ctx.DB.WithContext(ctx).Begin()

	// Optionally check if roles exist to be idempotent.
	for _, r := range roles {
		var existing Role
		if err := tx.Where("org_id = ? AND slug = ?", r.OrgID, r.Slug).First(&existing).Error; err != nil {
			if err := tx.Create(&r).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
	}

	return tx.Commit().Error
}
