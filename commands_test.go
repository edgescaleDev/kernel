package kernel

import (
	"io/fs"
	"testing"
)

func TestKernelMigrations_Embedded(t *testing.T) {
	migrations := KernelMigrations()
	if migrations == nil {
		t.Fatal("KernelMigrations() returned nil")
	}

	// Verify all 12 migration files are embedded.
	expected := []string{
		"001_schema_migrations.up.sql",
		"002_module_registry.up.sql",
		"003_audit_events.up.sql",
		"004_event_outbox.up.sql",
		"005_webhooks.up.sql",
		"006_custom_fields.up.sql",
		"007_oauth_apps.up.sql",
		"008_feature_flags.up.sql",
		"009_scheduled_jobs.up.sql",
		"010_idempotency_cache.up.sql",
		"011_operations.up.sql",
		"012_retention_policies.up.sql",
	}

	for _, name := range expected {
		data, err := fs.ReadFile(migrations, name)
		if err != nil {
			t.Errorf("missing migration file %q: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("migration file %q is empty", name)
		}
	}
}

func TestKernelMigrations_CollectFiles(t *testing.T) {
	migrations := KernelMigrations()
	files, err := collectMigrationFiles(migrations)
	if err != nil {
		t.Fatalf("collectMigrationFiles: %v", err)
	}

	if len(files) != 12 {
		t.Errorf("expected 12 migration files, got %d: %v", len(files), files)
	}

	// Verify sort order.
	if files[0] != "001_schema_migrations.up.sql" {
		t.Errorf("first file = %q, want 001_schema_migrations.up.sql", files[0])
	}
	if files[11] != "012_retention_policies.up.sql" {
		t.Errorf("last file = %q, want 012_retention_policies.up.sql", files[11])
	}
}

func TestBuildRootCommand(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("orders"))
	k.MustRegister(newStub("billing", "orders"))

	root := k.buildRootCommand()
	if root == nil {
		t.Fatal("buildRootCommand() returned nil")
	}

	// Verify subcommands exist.
	cmds := make(map[string]bool)
	for _, c := range root.Commands() {
		cmds[c.Name()] = true
	}

	for _, name := range []string{"serve", "migrate", "module", "org"} {
		if !cmds[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestModuleListCommand_Output(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("orders"))

	cmd := k.moduleListCommand()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("module list: %v", err)
	}
}

func TestModuleDepsCommand_Output(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("orders"))
	k.MustRegister(newStub("billing", "orders"))

	cmd := k.moduleDepsCommand()
	cmd.Execute() // Just verify no panic.
}

func TestManifests(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("orders"))
	k.MustRegister(newStub("billing"))

	manifests := k.Manifests()
	if len(manifests) != 2 {
		t.Errorf("Manifests() returned %d entries, want 2", len(manifests))
	}
	for _, id := range []string{"orders", "billing"} {
		if _, ok := manifests[id]; !ok {
			t.Errorf("missing %q in Manifests()", id)
		}
	}
}
