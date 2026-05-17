package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/edgescaleDev/kernel/internal"
	"github.com/google/uuid"
	"github.com/kernel-contrib/sdk"
	"gorm.io/gorm"
)

// ProvisionTenant sets up a new tenant: creates per-module schemas,
// inserts core module activations, activates feature modules with a trial
// expiry, and calls each module's provision hook.
func (k *Kernel) ProvisionTenant(ctx context.Context, tenantID uuid.UUID, activatedBy uuid.UUID) error {
	return k.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// Compute trial expiry (nil means trial is disabled).
		var trialExpiry *time.Time
		if k.cfg.Server.TrialDuration > 0 {
			exp := now.Add(k.cfg.Server.TrialDuration)
			trialExpiry = &exp
		}

		var coreCount, trialCount int

		for _, m := range k.orderedModules() {
			manifest := m.Manifest()

			switch {
			case manifest.Type.IsCore():
				// Core modules: always active, no expiry.
				activation := internal.ModuleActivation{
					ModuleID:    manifest.ID,
					TenantID:    tenantID.String(),
					Active:      true,
					ActivatedBy: activatedBy.String(),
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				if err := tx.Create(&activation).Error; err != nil {
					return fmt.Errorf("activate core module %q: %w", manifest.ID, err)
				}
				coreCount++

			case manifest.Type == sdk.TypeFeature && trialExpiry != nil:
				// Feature modules: activate with trial expiry.
				activation := internal.ModuleActivation{
					ModuleID:    manifest.ID,
					TenantID:    tenantID.String(),
					Active:      true,
					ActivatedBy: activatedBy.String(),
					ExpiresAt:   trialExpiry,
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				if err := tx.Create(&activation).Error; err != nil {
					return fmt.Errorf("activate trial module %q: %w", manifest.ID, err)
				}
				trialCount++
			}
		}

		k.logger.Info("tenant provisioned",
			"tenant_id", tenantID,
			"core_modules", coreCount,
			"trial_modules", trialCount,
			"trial_expires_at", trialExpiry,
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

// ActivateModule enables a specific module for a tenant permanently
// (no expiry). This clears any existing trial expiry.
func (k *Kernel) ActivateModule(ctx context.Context, moduleID string, tenantID uuid.UUID, activatedBy uuid.UUID) error {
	return k.mm.Activate(ctx, tenantID, activatedBy, moduleID)
}

// ActivateModuleWithExpiry enables a specific module for a tenant with an
// optional expiry. Pass nil for expiresAt to activate permanently.
func (k *Kernel) ActivateModuleWithExpiry(ctx context.Context, moduleID string, tenantID uuid.UUID, activatedBy uuid.UUID, expiresAt *time.Time) error {
	if expiresAt == nil {
		return k.mm.Activate(ctx, tenantID, activatedBy, moduleID)
	}
	return k.mm.ActivateWithExpiry(ctx, tenantID, activatedBy, *expiresAt, moduleID)
}

// DeactivateModule disables a specific module for a tenant.
func (k *Kernel) DeactivateModule(ctx context.Context, moduleID string, tenantID uuid.UUID) error {
	return k.mm.Deactivate(ctx, tenantID, moduleID)
}

// ReapExpiredTrials deactivates all module activations whose trial has expired.
// It fires an "after.kernel.trial.expired" hook for each affected tenant so
// downstream modules (e.g., notifications) can react.
//
// This is designed to be called from a daily cron job.
func (k *Kernel) ReapExpiredTrials(ctx context.Context) error {
	now := time.Now()

	// Find all expired-but-still-active rows.
	var expired []internal.ModuleActivation
	if err := k.db.WithContext(ctx).
		Where("active = true AND expires_at IS NOT NULL AND expires_at < ?", now).
		Find(&expired).Error; err != nil {
		return fmt.Errorf("kernel: query expired trials: %w", err)
	}

	if len(expired) == 0 {
		return nil
	}

	// Deactivate in bulk.
	result := k.db.WithContext(ctx).
		Model(&internal.ModuleActivation{}).
		Where("active = true AND expires_at IS NOT NULL AND expires_at < ?", now).
		Update("active", false)
	if result.Error != nil {
		return fmt.Errorf("kernel: deactivate expired trials: %w", result.Error)
	}

	// Collect affected tenants (deduplicated) and bust Redis caches.
	affectedTenants := make(map[string]bool)
	for _, a := range expired {
		affectedTenants[a.TenantID] = true

		if k.redis != nil {
			cacheKey := fmt.Sprintf("module:%s:active:%s", a.ModuleID, a.TenantID)
			k.redis.Del(ctx, cacheKey)
		}
	}

	k.logger.Info("reaped expired trials",
		"deactivated", result.RowsAffected,
		"tenants_affected", len(affectedTenants),
	)

	// Fire hook per affected tenant.
	for tenantIDStr := range affectedTenants {
		tenantID, err := uuid.Parse(tenantIDStr)
		if err != nil {
			continue
		}

		if hookErr := k.hooks.FireAfter(ctx, "after.kernel.trial.expired", TrialExpiredEvent{
			TenantID: tenantID,
		}); hookErr != nil {
			k.logger.Error("trial.expired hook failed",
				"tenant_id", tenantID,
				"error", hookErr,
			)
		}
	}

	return nil
}

// TrialExpiredEvent is the payload for the "after.kernel.trial.expired" hook.
type TrialExpiredEvent struct {
	TenantID uuid.UUID `json:"tenant_id"`
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
