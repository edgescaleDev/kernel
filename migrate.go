package kernel

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"strings"

	"go.edgescale.dev/kernel/internal"
	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

// Migrate runs all database migrations in the correct order:
//  1. Kernel-owned migrations (public schema)
//  2. Module migrations in topological dependency order
//
// Each module's migrations run in its own schema (auto-created).
// Versions are tracked in public.schema_migrations.
func (k *Kernel) Migrate() error {
	k.logger.Info("starting migrations")

	// Auto-create the migrations tracking table if it doesn't exist.
	if err := k.db.AutoMigrate(&internal.SchemaMigration{}); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Run each module's migrations in dependency order.
	if err := k.runModuleMigrations("kernel", "public", KernelMigrations()); err != nil {
		return fmt.Errorf("migrate kernel: %w", err)
	}

	for _, m := range k.orderedModules() {
		manifest := m.Manifest()
		migrations := m.Migrations()
		if migrations == nil {
			continue
		}

		schema := manifest.Schema
		if schema == "" {
			schema = "public"
		}

		// Auto-create the module's schema if it's not public.
		if schema != "public" {
			if err := k.db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)).Error; err != nil {
				return fmt.Errorf("create schema %q: %w", schema, err)
			}
		}

		if err := k.runModuleMigrations(manifest.ID, schema, migrations); err != nil {
			return fmt.Errorf("migrate %q: %w", manifest.ID, err)
		}
	}

	k.logger.Info("migrations complete")
	return nil
}

// runModuleMigrations applies SQL migration files for a single module.
// Files are sorted by name and only unapplied migrations are executed.
func (k *Kernel) runModuleMigrations(moduleID, schema string, migrations fs.FS) error {
	files, err := internal.CollectMigrationFiles(migrations)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	if len(files) == 0 {
		return nil
	}

	// Find already-applied migrations.
	var applied []internal.SchemaMigration
	k.db.Where("module_id = ?", moduleID).Order("version ASC").Find(&applied)
	appliedSet := make(map[int]bool, len(applied))
	for _, a := range applied {
		appliedSet[a.Version] = true
	}

	for i, file := range files {
		version := i + 1
		if appliedSet[version] {
			continue
		}

		content, err := fs.ReadFile(migrations, file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}

		// Set search_path for the module's schema.
		sql := fmt.Sprintf("SET search_path TO %s, public;\n%s", schema, string(content))

		k.logger.Info("applying migration",
			"module", moduleID,
			"version", version,
			"file", file,
		)

		checksum := fmt.Sprintf("%x", sha256.Sum256(content))
		if err := k.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(sql).Error; err != nil {
				return fmt.Errorf("execute %s: %w", file, err)
			}

			record := internal.SchemaMigration{
				ModuleID: moduleID,
				Version:  version,
				Filename: file,
				Checksum: checksum,
			}
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("record migration %s: %w", file, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// Rollback reverts the last N applied migrations for a specific module.
// Each step executes the corresponding .down.sql file in a transaction.
// If a .down.sql file is missing, the rollback stops with an error.
//
// Usage:
//
//	kernel migrate rollback --module billing --steps 1
func (k *Kernel) Rollback(moduleID string, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("rollback: steps must be > 0")
	}

	// Find the module.
	var targetModule sdk.Module
	var manifest sdk.Manifest
	if moduleID == "kernel" {
		manifest = sdk.Manifest{ID: "kernel", Schema: "public"}
	} else {
		for _, m := range k.orderedModules() {
			if m.Manifest().ID == moduleID {
				targetModule = m
				manifest = m.Manifest()
				break
			}
		}
		if targetModule == nil {
			return fmt.Errorf("rollback: module %q not registered", moduleID)
		}
	}

	// Get the migration FS.
	var migrationFS fs.FS
	if moduleID == "kernel" {
		migrationFS = KernelMigrations()
	} else {
		migrationFS = targetModule.Migrations()
		if migrationFS == nil {
			return fmt.Errorf("rollback: module %q has no migrations", moduleID)
		}
	}

	schema := manifest.Schema
	if schema == "" {
		schema = "public"
	}

	// Find applied migrations in reverse order.
	var applied []internal.SchemaMigration
	k.db.Where("module_id = ?", moduleID).Order("version DESC").Find(&applied)

	if len(applied) == 0 {
		k.logger.Info("no migrations to rollback", "module", moduleID)
		return nil
	}

	if steps > len(applied) {
		steps = len(applied)
	}

	// Collect available .down.sql files.
	downFiles, err := internal.CollectDownFiles(migrationFS)
	if err != nil {
		return fmt.Errorf("rollback: read down files: %w", err)
	}

	for i := 0; i < steps; i++ {
		migration := applied[i]

		// Derive the .down.sql filename from the .up.sql filename.
		downFilename := strings.Replace(migration.Filename, ".up.sql", ".down.sql", 1)

		if _, exists := downFiles[downFilename]; !exists {
			return fmt.Errorf(
				"rollback: missing %s — cannot rollback version %d of module %q. "+
					"Create the .down.sql file and rebuild.",
				downFilename, migration.Version, moduleID,
			)
		}

		content, err := fs.ReadFile(migrationFS, downFilename)
		if err != nil {
			return fmt.Errorf("rollback: read %s: %w", downFilename, err)
		}

		sql := fmt.Sprintf("SET LOCAL search_path TO %s, public;\n%s", schema, string(content))

		k.logger.Info("rolling back migration",
			"module", moduleID,
			"version", migration.Version,
			"file", downFilename,
		)

		if err := k.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(sql).Error; err != nil {
				return fmt.Errorf("execute %s: %w", downFilename, err)
			}

			if err := tx.Where("module_id = ? AND version = ?", moduleID, migration.Version).
				Delete(&internal.SchemaMigration{}).Error; err != nil {
				return fmt.Errorf("delete migration record %s: %w", downFilename, err)
			}
			return nil
		}); err != nil {
			return err
		}

		k.logger.Info("rolled back migration",
			"module", moduleID,
			"version", migration.Version,
		)
	}

	k.logger.Info("rollback complete", "module", moduleID, "steps", steps)
	return nil
}
