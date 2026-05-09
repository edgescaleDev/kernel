package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/edgescaleDev/kernel/internal"
)

// syncRegistry upserts all registered module manifests into the
// module_registry table. Called during Serve() so the database always
// reflects the currently compiled-in modules.
func (k *Kernel) syncRegistry() error {
	if k.db == nil {
		return nil
	}

	for _, m := range k.orderedModules() {
		manifest := m.Manifest()
		record := internal.ModuleRecord{
			ID:          manifest.ID,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Type:        manifest.Type.String(),
			SchemaName:  manifest.Schema,
			Description: manifest.Description,
			DependsOn:   coalesceSlice(manifest.DependsOn),
		}

		result := k.db.Where("id = ?", record.ID).Assign(record).FirstOrCreate(&record)
		if result.Error != nil {
			return fmt.Errorf("sync registry %q: %w", manifest.ID, result.Error)
		}
	}

	k.logger.Info("module registry synced", "count", len(k.modules))
	return nil
}

// isModuleActive checks whether a module is active for the given tenant.
// Core modules always return true. Feature/integration modules check
// the module_activations table (Redis cached).
//
// Activations with a non-nil expires_at in the past are treated as inactive.
func (k *Kernel) isModuleActive(moduleID string, tenantID string) bool {
	manifest, exists := k.manifests[moduleID]
	if !exists {
		return false
	}

	// Core modules are always active.
	if manifest.Type.IsCore() {
		return true
	}

	now := time.Now()

	// Check Redis cache first.
	if k.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, tenantID)
		val, err := k.redis.Get(context.Background(), cacheKey).Result()
		if err == nil {
			// Cache format: "0" (inactive), "1" (active, no expiry),
			// or RFC3339 timestamp (active with expiry).
			switch val {
			case "0":
				return false
			case "1":
				return true
			default:
				// Parse as expiry timestamp.
				if exp, parseErr := time.Parse(time.RFC3339, val); parseErr == nil {
					return now.Before(exp)
				}
				// Malformed cache entry; fall through to DB.
			}
		}
	}

	// Fall back to database.
	var activation internal.ModuleActivation
	result := k.db.
		Where("module_id = ? AND tenant_id = ? AND active = true", moduleID, tenantID).
		First(&activation)
	if result.Error != nil {
		return false
	}

	// Check expiry.
	active := activation.ExpiresAt == nil || now.Before(*activation.ExpiresAt)

	// Cache the result for 1 minute.
	if k.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, tenantID)
		var val string
		if !active {
			val = "0"
		} else if activation.ExpiresAt == nil {
			val = "1"
		} else {
			val = activation.ExpiresAt.Format(time.RFC3339)
		}
		k.redis.Set(context.Background(), cacheKey, val, time.Minute)
	}

	return active
}

// coalesceSlice returns s if non-nil, otherwise an empty slice.
// Prevents GORM's JSON serializer from writing NULL to NOT NULL columns.
func coalesceSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
