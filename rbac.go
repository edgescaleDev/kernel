package kernel

import (
	"slices"
	"strings"
)

// PermissionSet holds a user's resolved permissions for a specific org.
// Loaded from the database and cached in Redis with a 5-minute TTL.
type PermissionSet struct {
	perms map[string]bool
}

// NewPermissionSet creates a PermissionSet from a list of permission strings.
func NewPermissionSet(permissions []string) *PermissionSet {
	perms := make(map[string]bool, len(permissions))
	for _, p := range permissions {
		perms[p] = true
	}
	return &PermissionSet{perms: perms}
}

// Has checks if the permission set grants the required permission.
// Supports three matching modes:
//   - Exact match: "orders.create"
//   - Namespace wildcard: "orders.*" matches any "orders.X"
//   - Global wildcard: "*" matches everything (owner/superadmin)
func (ps *PermissionSet) Has(required string) bool {
	if ps == nil || ps.perms == nil {
		return false
	}

	// Global wildcard - owner role.
	if ps.perms["*"] {
		return true
	}

	// Exact match.
	if ps.perms[required] {
		return true
	}

	// Namespace wildcard: "orders.create" matches "orders.*".
	parts := strings.SplitN(required, ".", 2)
	if len(parts) == 2 {
		return ps.perms[parts[0]+".*"]
	}

	return false
}

// HasAny returns true if any of the provided permissions are granted.
func (ps *PermissionSet) HasAny(permissions ...string) bool {
	return slices.ContainsFunc(permissions, ps.Has)
}

// Permissions returns a copy of all permission strings in the set.
func (ps *PermissionSet) Permissions() []string {
	if ps == nil || ps.perms == nil {
		return nil
	}
	result := make([]string, 0, len(ps.perms))
	for p := range ps.perms {
		result = append(result, p)
	}
	return result
}
