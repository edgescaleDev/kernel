package kernel

import (
	"context"
	"fmt"
	"time"
)

// syncRegistry upserts all registered module manifests into the
// module_registry table. Called during Serve() so the database always
// reflects the currently compiled-in modules.
func (k *Kernel) syncRegistry() error {
	if k.db == nil {
		return nil
	}

	for _, m := range k.Modules() {
		manifest := m.Manifest()
		record := ModuleRecord{
			ID:          manifest.ID,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Type:        manifest.Type.String(),
			SchemaName:  manifest.Schema,
			Description: manifest.Description,
			DependsOn:   manifest.DependsOn,
		}

		result := k.db.Where("id = ?", record.ID).Assign(record).FirstOrCreate(&record)
		if result.Error != nil {
			return fmt.Errorf("sync registry %q: %w", manifest.ID, result.Error)
		}
	}

	k.logger.Info("module registry synced", "count", len(k.modules))
	return nil
}

// ModuleRecord maps to the public.module_registry table.
type ModuleRecord struct {
	ID          string   `gorm:"primaryKey;column:id"`
	Name        string   `gorm:"column:name;not null"`
	Version     string   `gorm:"column:version;not null"`
	Type        string   `gorm:"column:type;not null"`
	SchemaName  string   `gorm:"column:schema_name;not null"`
	Description string   `gorm:"column:description"`
	DependsOn   []string `gorm:"column:depends_on;type:text[];serializer:json"`
}

func (ModuleRecord) TableName() string {
	return "module_registry"
}

// ModuleActivation maps to the public.module_activations table.
type ModuleActivation struct {
	ModuleID    string    `gorm:"primaryKey;column:module_id"`
	OrgID       string    `gorm:"primaryKey;column:org_id;type:uuid"`
	Active      bool      `gorm:"column:active;default:true"`
	ActivatedBy string    `gorm:"column:activated_by;type:uuid"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ModuleActivation) TableName() string {
	return "module_activations"
}

// IsModuleActive checks whether a module is active for the given org.
// Core modules always return true. Feature/integration modules check
// the module_activations table (Redis cached).
func (k *Kernel) IsModuleActive(moduleID string, orgID string) bool {
	manifest, exists := k.manifests[moduleID]
	if !exists {
		return false
	}

	// Core modules are always active.
	if manifest.Type.IsCore() {
		return true
	}

	// Check Redis cache first.
	if k.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, orgID)
		val, err := k.redis.Get(context.Background(), cacheKey).Result()
		if err == nil {
			return val == "1"
		}
	}

	// Fall back to database.
	var activation ModuleActivation
	result := k.db.Where("module_id = ? AND org_id = ?", moduleID, orgID).First(&activation)
	if result.Error != nil {
		return false
	}

	// Cache the result for 1 minute.
	if k.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, orgID)
		val := "0"
		if activation.Active {
			val = "1"
		}
		k.redis.Set(context.Background(), cacheKey, val, time.Minute)
	}

	return activation.Active
}
