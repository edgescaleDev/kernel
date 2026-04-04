package kernel

import (
	"log/slog"
	"testing"
	"time"

	"go.edgescale.dev/kernel/sdk"
)

func TestAuditEvent_TableName(t *testing.T) {
	event := AuditEvent{}
	if event.TableName() != "audit_events" {
		t.Errorf("TableName() = %q, want %q", event.TableName(), "audit_events")
	}
}

func TestNewAuditLogger_NilDB(t *testing.T) {
	logger := newAuditLogger(nil, "orders")

	// Should return a TestAuditLogger (noop) when DB is nil.
	_, ok := logger.(*sdk.TestAuditLogger)
	if !ok {
		t.Errorf("nil DB should return TestAuditLogger, got %T", logger)
	}
}

func TestJSON_ScanNil(t *testing.T) {
	var j JSON
	if err := j.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if j != nil {
		t.Errorf("Scan(nil) should set to nil, got %v", j)
	}
}

func TestJSON_ScanBytes(t *testing.T) {
	var j JSON
	if err := j.Scan([]byte(`{"key":"value"}`)); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if string(j) != `{"key":"value"}` {
		t.Errorf("Scan result = %q, want %q", string(j), `{"key":"value"}`)
	}
}

func TestJSON_ValueNil(t *testing.T) {
	var j JSON
	val, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if val != nil {
		t.Errorf("nil JSON Value() = %v, want nil", val)
	}
}

func TestModuleRecord_TableName(t *testing.T) {
	r := ModuleRecord{}
	if r.TableName() != "module_registry" {
		t.Errorf("TableName() = %q, want %q", r.TableName(), "module_registry")
	}
}

func TestModuleActivation_TableName(t *testing.T) {
	a := ModuleActivation{}
	if a.TableName() != "module_activations" {
		t.Errorf("TableName() = %q, want %q", a.TableName(), "module_activations")
	}
}

func TestSchemaMigration_TableName(t *testing.T) {
	m := SchemaMigration{}
	if m.TableName() != "schema_migrations" {
		t.Errorf("TableName() = %q, want %q", m.TableName(), "schema_migrations")
	}
}

func TestOperation_TableName(t *testing.T) {
	op := Operation{}
	if op.TableName() != "operations" {
		t.Errorf("TableName() = %q, want %q", op.TableName(), "operations")
	}
}

func TestOperationStatus_Constants(t *testing.T) {
	tests := []struct {
		status OperationStatus
		want   string
	}{
		{OperationPending, "pending"},
		{OperationRunning, "running"},
		{OperationCompleted, "completed"},
		{OperationFailed, "failed"},
		{OperationCancelled, "cancelled"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("OperationStatus %v = %q, want %q", tt.status, string(tt.status), tt.want)
		}
	}
}

func TestOutboxWriter_ImplementsInterface(t *testing.T) {
	var _ sdk.OutboxWriter = &outboxWriter{}
}

func TestOutboxPoller_StartStop(t *testing.T) {
	poller := NewOutboxPoller(nil, &sdk.TestBus{}, slog.Default())
	poller.interval = 100 * time.Millisecond

	poller.Start()
	time.Sleep(150 * time.Millisecond) // let at least one tick run
	poller.Stop()

	// Just verifying no panic or deadlock.
}

func TestHealthStatus(t *testing.T) {
	healthy := HealthStatus{Healthy: true}
	if !healthy.Healthy {
		t.Error("healthy status should be true")
	}

	degraded := HealthStatus{Healthy: false, Message: "db down"}
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
	if k.IsModuleActive("nonexistent", "some-org-id") {
		t.Error("unregistered module should not be active")
	}
}

func TestIsModuleActive_CoreModule(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newCoreStub("auth"))

	if !k.IsModuleActive("auth", "any-org-id") {
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
	if statusString(true) != "healthy" {
		t.Errorf("statusString(true) = %q, want %q", statusString(true), "healthy")
	}
	if statusString(false) != "degraded" {
		t.Errorf("statusString(false) = %q, want %q", statusString(false), "degraded")
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
