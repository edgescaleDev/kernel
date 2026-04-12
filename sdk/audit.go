package sdk

import (
	"context"

	"github.com/google/uuid"
)

// AuditLogger appends entries to the kernel's hash-chained audit log.
// The kernel's middleware automatically captures user_id, tenant_id, service_id,
// ip_address, user_agent, and request_id from the request context.
// Services only need to call Log() for custom audit events beyond automatic logging.
type AuditLogger interface {
	// Log appends an audit entry to the hash-chained audit log.
	Log(ctx context.Context, entry AuditEntry) error
}

// AuditAction represents the type of operation being audited.
// Consumers can define additional actions as needed: const ActionCustom AuditAction = "custom"
type AuditAction string

const (
	AuditCreate     AuditAction = "create"
	AuditUpdate     AuditAction = "update"
	AuditDelete     AuditAction = "delete"
	AuditRestore    AuditAction = "restore"
	AuditLogin      AuditAction = "login"
	AuditLogout     AuditAction = "logout"
	AuditExport     AuditAction = "export"
	AuditImport     AuditAction = "import"
	AuditApprove    AuditAction = "approve"
	AuditReject     AuditAction = "reject"
	AuditActivate   AuditAction = "activate"
	AuditDeactivate AuditAction = "deactivate"
)

// AuditEntry represents a single audit event to be recorded.
type AuditEntry struct {
	// Action is the operation type.
	Action AuditAction

	// Resource is the entity type (e.g., "order", "invoice", "user").
	Resource string

	// ResourceID is the identifier of the affected resource.
	ResourceID string

	// Changes contains the diff of changed fields: {field: {old, new}}.
	// Can be nil for create/delete operations.
	Changes map[string]AuditChange

	// UserID overrides the automatic user detection (e.g., for system actions).
	UserID *uuid.UUID

	// TenantID overrides the automatic tenant detection.
	TenantID *uuid.UUID
}

// AuditChange represents a single field change in an audit entry.
type AuditChange struct {
	// Old is the previous value.
	Old any `json:"old,omitempty"`

	// New is the new value.
	New any `json:"new,omitempty"`
}
