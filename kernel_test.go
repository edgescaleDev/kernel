package kernel

import (
	"context"
	"io/fs"
	"testing"

	"go.edgescale.dev/kernel/sdk"
)

// stubModule is a minimal Module implementation for testing.
type stubModule struct {
	manifest sdk.Manifest
}

func newStub(id string, deps ...string) *stubModule {
	return &stubModule{
		manifest: sdk.Manifest{
			ID:        id,
			Name:      id,
			Version:   "1.0.0",
			Type:      sdk.TypeFeature,
			Schema:    "module_" + id,
			DependsOn: deps,
		},
	}
}

func (s *stubModule) Manifest() sdk.Manifest                    { return s.manifest }
func (s *stubModule) Migrations() fs.FS                         { return nil }
func (s *stubModule) Init(_ sdk.Context) error                  { return nil }
func (s *stubModule) RegisterRoutes(_ *sdk.Router)              {}
func (s *stubModule) RegisterEvents(_ sdk.EventBus)             {}
func (s *stubModule) RegisterHooks(_ *sdk.HookRegistry)         {}
func (s *stubModule) RegisterWorkflows(_ sdk.WorkflowRegistry)  {}
func (s *stubModule) RegisterActivities(_ sdk.ActivityRegistry) {}
func (s *stubModule) Shutdown() error                           { return nil }

// --- Kernel tests ---

func TestNew(t *testing.T) {
	k := New(DefaultConfig())
	if k == nil {
		t.Fatal("New() returned nil")
	}
	if k.cfg.Server.Port != 8080 {
		t.Errorf("New() config port = %d, want 8080", k.cfg.Server.Port)
	}
	if k.hooks == nil {
		t.Error("New() hooks should be initialized")
	}
	if k.readers == nil {
		t.Error("New() readers should be initialized")
	}
}

func TestRegister(t *testing.T) {
	k := New(DefaultConfig())

	if err := k.Register(newStub("orders")); err != nil {
		t.Fatalf("Register orders: %v", err)
	}
	if err := k.Register(newStub("billing")); err != nil {
		t.Fatalf("Register billing: %v", err)
	}

	if len(k.modules) != 2 {
		t.Errorf("Register count = %d, want 2", len(k.modules))
	}
	if _, ok := k.manifests["orders"]; !ok {
		t.Error("Register should store manifest for 'orders'")
	}
	if _, ok := k.manifests["billing"]; !ok {
		t.Error("Register should store manifest for 'billing'")
	}
}

func TestRegister_DuplicateReturnsError(t *testing.T) {
	k := New(DefaultConfig())

	if err := k.Register(newStub("orders")); err != nil {
		t.Fatalf("First register: %v", err)
	}

	err := k.Register(newStub("orders"))
	if err == nil {
		t.Error("Register duplicate should return error")
	}
}

func TestMustRegister_PanicsOnDuplicate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRegister duplicate should panic")
		}
	}()

	k := New(DefaultConfig())
	k.MustRegister(newStub("orders"))
	k.MustRegister(newStub("orders")) // should panic
}

func TestModules_BeforeBoot(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("a"))
	k.MustRegister(newStub("b"))

	modules := k.Modules()
	if len(modules) != 2 {
		t.Errorf("Modules() before Boot = %d, want 2", len(modules))
	}
}

func TestInstallFallbacks(t *testing.T) {
	k := New(DefaultConfig())

	// Before installFallbacks - all nil.
	if k.bus != nil {
		t.Error("bus should be nil before installFallbacks")
	}

	k.installFallbacks()

	// After - all non-nil.
	if k.bus == nil {
		t.Error("bus should not be nil after installFallbacks")
	}
	if k.taskExecutor == nil {
		t.Error("taskExecutor should not be nil after installFallbacks")
	}
	if k.searchEngine == nil {
		t.Error("searchEngine should not be nil after installFallbacks")
	}
}

func TestInstallFallbacks_DoesNotOverrideExplicit(t *testing.T) {
	k := New(DefaultConfig())

	custom := noopEventBus{}
	k.SetEventBus(custom)
	k.installFallbacks()

	// Bus should still be the custom one (not replaced).
	if k.bus != custom {
		t.Error("installFallbacks should not override explicitly-set bus")
	}
}

func TestShutdown_ServicesInReverseOrder(t *testing.T) {
	k := New(DefaultConfig())

	var shutdownOrder []string
	makeRecorder := func(id string, deps ...string) *shutdownRecorder {
		return &shutdownRecorder{
			stubModule: *newStub(id, deps...),
			order:      &shutdownOrder,
		}
	}

	k.MustRegister(makeRecorder("iam"))
	k.MustRegister(makeRecorder("billing", "iam"))
	k.MustRegister(makeRecorder("invoicing", "billing"))

	// Manually compute dep order (Boot requires infra).
	order, err := k.validateAndSort()
	if err != nil {
		t.Fatalf("validateAndSort: %v", err)
	}
	k.depOrder = order

	ctx := context.Background()
	if err := k.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Expect reverse dep order: invoicing, billing, iam.
	if len(shutdownOrder) != 3 {
		t.Fatalf("shutdown count = %d, want 3", len(shutdownOrder))
	}
	want := []string{"invoicing", "billing", "iam"}
	for i, id := range want {
		if shutdownOrder[i] != id {
			t.Errorf("shutdown[%d] = %q, want %q", i, shutdownOrder[i], id)
		}
	}
}

func TestShutdown_OnlyOnce(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("a"))

	order, _ := k.validateAndSort()
	k.depOrder = order

	ctx := context.Background()
	_ = k.Shutdown(ctx)
	// Second call should be a no-op (no panic, no error).
	err := k.Shutdown(ctx)
	if err != nil {
		t.Errorf("Second Shutdown should return nil, got: %v", err)
	}
}

// shutdownRecorder tracks the order modules are shut down.
type shutdownRecorder struct {
	stubModule
	order *[]string
}

func (s *shutdownRecorder) Shutdown() error {
	*s.order = append(*s.order, s.manifest.ID)
	return nil
}
