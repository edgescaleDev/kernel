package kernel

import (
	"testing"

	"go.edgescale.dev/kernel/sdk"
)

func TestTopoSort_Linear(t *testing.T) {
	manifests := map[string]sdk.Manifest{
		"iam":       {ID: "iam", DependsOn: nil},
		"billing":   {ID: "billing", DependsOn: []string{"iam"}},
		"invoicing": {ID: "invoicing", DependsOn: []string{"billing"}},
	}

	order, err := topoSort(manifests)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}

	// iam must come before billing, billing before invoicing.
	idx := make(map[string]int)
	for i, id := range order {
		idx[id] = i
	}
	if idx["iam"] >= idx["billing"] {
		t.Errorf("iam (%d) should come before billing (%d)", idx["iam"], idx["billing"])
	}
	if idx["billing"] >= idx["invoicing"] {
		t.Errorf("billing (%d) should come before invoicing (%d)", idx["billing"], idx["invoicing"])
	}
}

func TestTopoSort_Diamond(t *testing.T) {
	// Classic diamond:  A → B, A → C, B → D, C → D
	manifests := map[string]sdk.Manifest{
		"a": {ID: "a", DependsOn: nil},
		"b": {ID: "b", DependsOn: []string{"a"}},
		"c": {ID: "c", DependsOn: []string{"a"}},
		"d": {ID: "d", DependsOn: []string{"b", "c"}},
	}

	order, err := topoSort(manifests)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}

	idx := make(map[string]int)
	for i, id := range order {
		idx[id] = i
	}
	if idx["a"] >= idx["b"] || idx["a"] >= idx["c"] {
		t.Error("a should come before b and c")
	}
	if idx["b"] >= idx["d"] || idx["c"] >= idx["d"] {
		t.Error("b and c should come before d")
	}
}

func TestTopoSort_CycleDetection(t *testing.T) {
	manifests := map[string]sdk.Manifest{
		"a": {ID: "a", DependsOn: []string{"c"}},
		"b": {ID: "b", DependsOn: []string{"a"}},
		"c": {ID: "c", DependsOn: []string{"b"}},
	}

	_, err := topoSort(manifests)
	if err == nil {
		t.Fatal("topoSort should detect cycle")
	}
	t.Logf("cycle error: %v", err)
}

func TestTopoSort_SingleNode(t *testing.T) {
	manifests := map[string]sdk.Manifest{
		"solo": {ID: "solo", DependsOn: nil},
	}

	order, err := topoSort(manifests)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	if len(order) != 1 || order[0] != "solo" {
		t.Errorf("topoSort single = %v, want [solo]", order)
	}
}

func TestTopoSort_Deterministic(t *testing.T) {
	manifests := map[string]sdk.Manifest{
		"z": {ID: "z", DependsOn: nil},
		"a": {ID: "a", DependsOn: nil},
		"m": {ID: "m", DependsOn: nil},
	}

	// Run 10 times - output must always be the same.
	first, _ := topoSort(manifests)
	for i := 0; i < 10; i++ {
		got, _ := topoSort(manifests)
		for j := range first {
			if first[j] != got[j] {
				t.Fatalf("topoSort not deterministic: run 0 = %v, run %d = %v", first, i+1, got)
			}
		}
	}
}

func TestValidateAndSort_MissingDependency(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("billing", "payments")) // payments not registered

	_, err := k.validateAndSort()
	if err == nil {
		t.Fatal("validateAndSort should fail for missing dependency")
	}
	t.Logf("missing dep error: %v", err)
}

func TestReverseDepOrder(t *testing.T) {
	k := New(DefaultConfig())
	k.depOrder = []string{"iam", "billing", "invoicing"}

	rev := k.reverseDepOrder()
	want := []string{"invoicing", "billing", "iam"}

	for i, id := range want {
		if rev[i] != id {
			t.Errorf("reverse[%d] = %q, want %q", i, rev[i], id)
		}
	}
}

func TestReverseDepOrder_Empty(t *testing.T) {
	k := New(DefaultConfig())
	rev := k.reverseDepOrder()
	if rev != nil {
		t.Errorf("reverseDepOrder empty = %v, want nil", rev)
	}
}

func TestDependents(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("core"))
	k.MustRegister(newStub("billing", "core"))
	k.MustRegister(newStub("invoicing", "billing"))
	k.MustRegister(newStub("payments", "core"))

	deps := k.Dependents("core")
	// billing, invoicing, payments all transitively depend on core.
	if len(deps) != 3 {
		t.Fatalf("Dependents(core) = %v, want 3 entries", deps)
	}
	// Should contain billing, invoicing, payments (sorted).
	want := []string{"billing", "invoicing", "payments"}
	for i, id := range want {
		if deps[i] != id {
			t.Errorf("deps[%d] = %q, want %q", i, deps[i], id)
		}
	}
}

func TestDependents_Leaf(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("core"))
	k.MustRegister(newStub("billing", "core"))

	deps := k.Dependents("billing")
	if len(deps) != 0 {
		t.Errorf("Dependents(billing) = %v, want empty (leaf node)", deps)
	}
}
