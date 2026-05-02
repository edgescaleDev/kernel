package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/edgescaleDev/kernel/internal"
	"github.com/edgescaleDev/kernel/sdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProvisionTenant sets up a new tenant: creates per-module schemas,
// inserts core module activations, and calls each module's provision hook.
func (k *Kernel) ProvisionTenant(ctx context.Context, tenantID uuid.UUID, activatedBy uuid.UUID) error {
	return k.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Activate core modules automatically.
		for _, m := range k.orderedModules() {
			manifest := m.Manifest()
			if !manifest.Type.IsCore() {
				continue
			}

			activation := internal.ModuleActivation{
				ModuleID:    manifest.ID,
				TenantID:    tenantID.String(),
				Active:      true,
				ActivatedBy: activatedBy.String(),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			if err := tx.Create(&activation).Error; err != nil {
				return fmt.Errorf("activate core module %q: %w", manifest.ID, err)
			}
		}

		k.logger.Info("tenant provisioned",
			"tenant_id", tenantID,
			"core_modules", k.coreModuleCount(),
		)

		if err := k.hooks.FireAfter(ctx, "after.kernel.tenant.provisioned", sdk.TenantProvisionedEvent{
			TenantID:    tenantID,
			ActivatedBy: activatedBy,
		}); err != nil {
			k.logger.Error("provision hooks failed", "error", err)
		}

		return nil
	})
}

// DeprovisionTenant deactivates all modules for a tenant.
// Does NOT delete data — that's handled by retention policies.
func (k *Kernel) DeprovisionTenant(ctx context.Context, tenantID uuid.UUID) error {
	result := k.db.WithContext(ctx).
		Model(&internal.ModuleActivation{}).
		Where("tenant_id = ?", tenantID.String()).
		Update("active", false)

	if result.Error != nil {
		return fmt.Errorf("deprovision tenant %s: %w", tenantID, result.Error)
	}

	// Invalidate Redis cache for all modules.
	if k.redis != nil {
		for _, m := range k.orderedModules() {
			cacheKey := fmt.Sprintf("module:%s:active:%s", m.Manifest().ID, tenantID)
			k.redis.Del(ctx, cacheKey)
		}
	}

	k.logger.Info("tenant deprovisioned",
		"tenant_id", tenantID,
		"modules_deactivated", result.RowsAffected,
	)
	return nil
}

// ActivateModule enables a specific module for a tenant.
func (k *Kernel) ActivateModule(ctx context.Context, moduleID string, tenantID uuid.UUID, activatedBy uuid.UUID) error {
	activation := internal.ModuleActivation{
		ModuleID:    moduleID,
		TenantID:    tenantID.String(),
		Active:      true,
		ActivatedBy: activatedBy.String(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result := k.db.WithContext(ctx).
		Where("module_id = ? AND tenant_id = ?", moduleID, tenantID.String()).
		Assign(internal.ModuleActivation{Active: true, ActivatedBy: activatedBy.String(), UpdatedAt: time.Now()}).
		FirstOrCreate(&activation)

	if result.Error != nil {
		return fmt.Errorf("activate module %q for tenant %s: %w", moduleID, tenantID, result.Error)
	}

	// Invalidate cache.
	if k.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, tenantID)
		k.redis.Del(ctx, cacheKey)
	}

	k.logger.Info("module activated", "module", moduleID, "tenant_id", tenantID)
	return nil
}

// DeactivateModule disables a specific module for a tenant.
func (k *Kernel) DeactivateModule(ctx context.Context, moduleID string, tenantID uuid.UUID) error {
	result := k.db.WithContext(ctx).
		Model(&internal.ModuleActivation{}).
		Where("module_id = ? AND tenant_id = ?", moduleID, tenantID.String()).
		Update("active", false)

	if result.Error != nil {
		return fmt.Errorf("deactivate module %q for tenant %s: %w", moduleID, tenantID, result.Error)
	}

	// Invalidate cache.
	if k.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, tenantID)
		k.redis.Del(ctx, cacheKey)
	}

	k.logger.Info("module deactivated", "module", moduleID, "tenant_id", tenantID)
	return nil
}

// coreModuleCount returns the number of core modules registered.
func (k *Kernel) coreModuleCount() int {
	count := 0
	for _, m := range k.orderedModules() {
		if m.Manifest().Type.IsCore() {
			count++
		}
	}
	return count
}
