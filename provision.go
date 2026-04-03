package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProvisionOrg sets up a new organization: creates per-module schemas,
// inserts core module activations, and calls each module's provision hook.
func (k *Kernel) ProvisionOrg(ctx context.Context, orgID uuid.UUID, activatedBy uuid.UUID) error {
	return k.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Activate core modules automatically.
		for _, m := range k.Modules() {
			manifest := m.Manifest()
			if !manifest.Type.IsCore() {
				continue
			}

			activation := ModuleActivation{
				ModuleID:    manifest.ID,
				OrgID:       orgID.String(),
				Active:      true,
				ActivatedBy: activatedBy.String(),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			if err := tx.Create(&activation).Error; err != nil {
				return fmt.Errorf("activate core module %q: %w", manifest.ID, err)
			}
		}

		k.logger.Info("org provisioned",
			"org_id", orgID,
			"core_modules", k.coreModuleCount(),
		)
		return nil
	})
}

// DeprovisionOrg deactivates all modules for an organization.
// Does NOT delete data — that's handled by retention policies.
func (k *Kernel) DeprovisionOrg(ctx context.Context, orgID uuid.UUID) error {
	result := k.db.WithContext(ctx).
		Model(&ModuleActivation{}).
		Where("org_id = ?", orgID.String()).
		Update("active", false)

	if result.Error != nil {
		return fmt.Errorf("deprovision org %s: %w", orgID, result.Error)
	}

	// Invalidate Redis cache for all modules.
	if k.redis != nil {
		for _, m := range k.Modules() {
			cacheKey := fmt.Sprintf("module:%s:active:%s", m.Manifest().ID, orgID)
			k.redis.Del(ctx, cacheKey)
		}
	}

	k.logger.Info("org deprovisioned",
		"org_id", orgID,
		"modules_deactivated", result.RowsAffected,
	)
	return nil
}

// ActivateModule enables a specific module for an organization.
func (k *Kernel) ActivateModule(ctx context.Context, moduleID string, orgID uuid.UUID, activatedBy uuid.UUID) error {
	activation := ModuleActivation{
		ModuleID:    moduleID,
		OrgID:       orgID.String(),
		Active:      true,
		ActivatedBy: activatedBy.String(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result := k.db.WithContext(ctx).
		Where("module_id = ? AND org_id = ?", moduleID, orgID.String()).
		Assign(ModuleActivation{Active: true, ActivatedBy: activatedBy.String(), UpdatedAt: time.Now()}).
		FirstOrCreate(&activation)

	if result.Error != nil {
		return fmt.Errorf("activate module %q for org %s: %w", moduleID, orgID, result.Error)
	}

	// Invalidate cache.
	if k.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, orgID)
		k.redis.Del(ctx, cacheKey)
	}

	k.logger.Info("module activated", "module", moduleID, "org_id", orgID)
	return nil
}

// DeactivateModule disables a specific module for an organization.
func (k *Kernel) DeactivateModule(ctx context.Context, moduleID string, orgID uuid.UUID) error {
	result := k.db.WithContext(ctx).
		Model(&ModuleActivation{}).
		Where("module_id = ? AND org_id = ?", moduleID, orgID.String()).
		Update("active", false)

	if result.Error != nil {
		return fmt.Errorf("deactivate module %q for org %s: %w", moduleID, orgID, result.Error)
	}

	// Invalidate cache.
	if k.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, orgID)
		k.redis.Del(ctx, cacheKey)
	}

	k.logger.Info("module deactivated", "module", moduleID, "org_id", orgID)
	return nil
}

// coreModuleCount returns the number of core modules registered.
func (k *Kernel) coreModuleCount() int {
	count := 0
	for _, m := range k.Modules() {
		if m.Manifest().Type.IsCore() {
			count++
		}
	}
	return count
}
