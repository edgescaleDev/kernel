package kernel

import (
	"fmt"

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

// coalesceSlice returns s if non-nil, otherwise an empty slice.
// Prevents GORM's JSON serializer from writing NULL to NOT NULL columns.
func coalesceSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
