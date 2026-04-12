package kernel

import (
	"io/fs"
	"testing"

	"go.edgescale.dev/kernel/internal"
)

func TestKernelMigrations_Embedded(t *testing.T) {
	migrations := KernelMigrations()
	if migrations == nil {
		t.Fatal("KernelMigrations() returned nil")
	}

	// Verify both up and down migration files are present.
	expected := []string{
		"001_schema_migrations.up.sql",
		"001_schema_migrations.down.sql",
		"002_module_registry.up.sql",
		"002_module_registry.down.sql",
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
	files, err := internal.CollectMigrationFiles(migrations)
	if err != nil {
		t.Fatalf("CollectMigrationFiles: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 migration files, got %d: %v", len(files), files)
	}

	// Verify sort order.
	if len(files) >= 1 && files[0] != "001_schema_migrations.up.sql" {
		t.Errorf("first file = %q, want 001_schema_migrations.up.sql", files[0])
	}
	if len(files) >= 2 && files[1] != "002_module_registry.up.sql" {
		t.Errorf("last file = %q, want 002_module_registry.up.sql", files[1])
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

	for _, name := range []string{"serve", "migrate", "module", "tenant"} {
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

func TestAllManifests(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("orders"))
	k.MustRegister(newStub("billing"))

	manifests := k.allManifests()
	if len(manifests) != 2 {
		t.Errorf("allManifests() returned %d entries, want 2", len(manifests))
	}
	for _, id := range []string{"orders", "billing"} {
		if _, ok := manifests[id]; !ok {
			t.Errorf("missing %q in allManifests()", id)
		}
	}
}
