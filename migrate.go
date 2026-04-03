package kernel

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"
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
	if err := k.db.AutoMigrate(&SchemaMigration{}); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Run each module's migrations in dependency order.
	if err := k.runModuleMigrations("kernel", "public", KernelMigrations()); err != nil {
		return fmt.Errorf("migrate kernel: %w", err)
	}

	for _, m := range k.Modules() {
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
	files, err := collectMigrationFiles(migrations)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	if len(files) == 0 {
		return nil
	}

	// Find already-applied migrations.
	var applied []SchemaMigration
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

		if err := k.db.Exec(sql).Error; err != nil {
			return fmt.Errorf("execute %s: %w", file, err)
		}

		// Record the migration.
		checksum := fmt.Sprintf("%x", sha256.Sum256(content))
		record := SchemaMigration{
			ModuleID: moduleID,
			Version:  version,
			Filename: file,
			Checksum: checksum,
		}
		if err := k.db.Create(&record).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", file, err)
		}
	}

	return nil
}

// collectMigrationFiles returns all .up.sql files from the FS, sorted by name.
func collectMigrationFiles(migrations fs.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(migrations, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".up.sql") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(a, b string) int {
		return strings.Compare(filepath.Base(a), filepath.Base(b))
	})
	return files, nil
}

// SchemaMigration tracks applied migrations per module.
type SchemaMigration struct {
	ModuleID  string    `gorm:"primaryKey;column:module_id"`
	Version   int       `gorm:"primaryKey;column:version"`
	Filename  string    `gorm:"column:filename;not null"`
	Checksum  string    `gorm:"column:checksum;not null"`
	AppliedAt time.Time `gorm:"column:applied_at;autoCreateTime"`
}

func (SchemaMigration) TableName() string {
	return "schema_migrations"
}
