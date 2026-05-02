package kernel

import (
	"testing"

	"github.com/edgescaleDev/kernel/sdk"
)

func TestPermissionSet_ExactMatch(t *testing.T) {
	ps := sdk.NewPermissionSet([]string{"orders.create", "orders.read"})

	if !ps.Has("orders.create") {
		t.Error("should match exact permission")
	}
	if !ps.Has("orders.read") {
		t.Error("should match exact permission")
	}
	if ps.Has("orders.delete") {
		t.Error("should not match absent permission")
	}
}

func TestPermissionSet_NamespaceWildcard(t *testing.T) {
	ps := sdk.NewPermissionSet([]string{"orders.*"})

	if !ps.Has("orders.create") {
		t.Error("orders.* should match orders.create")
	}
	if !ps.Has("orders.read") {
		t.Error("orders.* should match orders.read")
	}
	if ps.Has("billing.read") {
		t.Error("orders.* should not match billing.read")
	}
}

func TestPermissionSet_GlobalWildcard(t *testing.T) {
	ps := sdk.NewPermissionSet([]string{"*"})

	if !ps.Has("orders.create") {
		t.Error("* should match anything")
	}
	if !ps.Has("billing.delete") {
		t.Error("* should match anything")
	}
}

func TestPermissionSet_Empty(t *testing.T) {
	ps := sdk.NewPermissionSet(nil)
	if ps.Has("orders.create") {
		t.Error("empty set should match nothing")
	}
}

func TestPermissionSet_Nil(t *testing.T) {
	var ps *sdk.PermissionSet
	if ps.Has("orders.create") {
		t.Error("nil set should match nothing")
	}
}

func TestPermissionSet_HasAny(t *testing.T) {
	ps := sdk.NewPermissionSet([]string{"billing.read"})

	if !ps.HasAny("orders.create", "billing.read") {
		t.Error("HasAny should match if any permission is present")
	}
	if ps.HasAny("orders.create", "orders.delete") {
		t.Error("HasAny should not match if none present")
	}
}

func TestPermissionSet_Permissions(t *testing.T) {
	ps := sdk.NewPermissionSet([]string{"a", "b", "c"})
	perms := ps.Permissions()
	if len(perms) != 3 {
		t.Errorf("Permissions() returned %d, want 3", len(perms))
	}
}

func TestPermissionSet_NilPermissions(t *testing.T) {
	var ps *sdk.PermissionSet
	if perms := ps.Permissions(); perms != nil {
		t.Errorf("nil set Permissions() = %v, want nil", perms)
	}
}
