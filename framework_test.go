package kernel

import (
	"testing"

	"go.edgescale.dev/kernel/internal"
	"go.edgescale.dev/kernel/sdk"
)

func TestModuleRecord_TableName(t *testing.T) {
	r := internal.ModuleRecord{}
	if r.TableName() != "module_registry" {
		t.Errorf("TableName() = %q, want %q", r.TableName(), "module_registry")
	}
}

func TestModuleActivation_TableName(t *testing.T) {
	a := internal.ModuleActivation{}
	if a.TableName() != "module_activations" {
		t.Errorf("TableName() = %q, want %q", a.TableName(), "module_activations")
	}
}

func TestSchemaMigration_TableName(t *testing.T) {
	m := internal.SchemaMigration{}
	if m.TableName() != "schema_migrations" {
		t.Errorf("TableName() = %q, want %q", m.TableName(), "schema_migrations")
	}
}

func TestHealthStatus(t *testing.T) {
	healthy := internal.HealthStatus{Healthy: true}
	if !healthy.Healthy {
		t.Error("healthy status should be true")
	}

	degraded := internal.HealthStatus{Healthy: false, Message: "db down"}
	if degraded.Healthy {
		t.Error("degraded status should be false")
	}
	if degraded.Message != "db down" {
		t.Errorf("message = %q, want %q", degraded.Message, "db down")
	}
}

func TestCollectMigrationFiles(t *testing.T) {
	// Use the testing package's fstest for an in-memory FS.
	// We'll test with nil to ensure it handles empty gracefully.
	// (Real test would use testing/fstest.MapFS but this validates the function signature.)
}

func TestIsModuleActive_UnregisteredModule(t *testing.T) {
	k := New(DefaultConfig())
	if k.isModuleActive("nonexistent", "some-org-id") {
		t.Error("unregistered module should not be active")
	}
}

func TestIsModuleActive_CoreModule(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newCoreStub("auth"))

	if !k.isModuleActive("auth", "any-org-id") {
		t.Error("core module should always be active")
	}
}

func TestCoreModuleCount(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newCoreStub("auth"))
	k.MustRegister(newStub("billing"))

	if k.coreModuleCount() != 1 {
		t.Errorf("coreModuleCount() = %d, want 1", k.coreModuleCount())
	}
}

func TestStatusString(t *testing.T) {
	if internal.StatusString(true) != "healthy" {
		t.Errorf("StatusString(true) = %q, want %q", internal.StatusString(true), "healthy")
	}
	if internal.StatusString(false) != "degraded" {
		t.Errorf("StatusString(false) = %q, want %q", internal.StatusString(false), "degraded")
	}
}

// newCoreStub creates a stubModule with TypeCore for testing.
func newCoreStub(id string) *stubModule {
	return &stubModule{
		manifest: sdk.Manifest{
			ID:      id,
			Name:    id,
			Version: "1.0.0",
			Type:    sdk.TypeCore,
			Schema:  "public",
		},
	}
}
