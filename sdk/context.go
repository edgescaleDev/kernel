package sdk

import (
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Context is the service's gateway to all platform capabilities.
// The kernel builds a Context for each service during initialization,
// scoped and isolated to that service's boundaries.
type Context struct {
	// DB is a GORM instance scoped to the service's schema with RLS enforced.
	// Queries automatically target the service's PostgreSQL schema (e.g., module_billing).
	DB *gorm.DB

	// PublicDB is a GORM instance for JOINs to shared kernel tables
	// (e.g., public.users, public.organizations).
	PublicDB *gorm.DB

	// Redis provides a namespaced Redis client.
	// All keys are automatically prefixed with "module:{service_id}:".
	Redis NamespacedRedis

	// Logger is a structured logger pre-configured with the service ID.
	Logger *slog.Logger

	// Audit provides access to the hash-chained audit log.
	Audit AuditLogger

	// Config returns per-tenant configuration for the given org.
	// Values are cached with auto-invalidation on changes.
	Config func(uuid.UUID) map[string]any

	// Bus provides fire-and-forget event publishing.
	Bus EventBus

	// Tasks provides background task execution (pluggable: Temporal, inline, etc.).
	Tasks TaskExecutor

	// Search provides full-text search (pluggable: Meilisearch, Elasticsearch, etc.).
	Search SearchEngine

	// Hooks provides sync hook point registration and firing.
	Hooks *HookRegistry

	// IdentityProvider validates and parses bearer tokens.
	IdentityProvider IdentityProvider

	// Outbox provides durable event publishing within the same database transaction.
	Outbox OutboxWriter

	// readers is the internal reader registry, accessed via GetReader[T]().
	readers *ReaderRegistry

	// ServiceID is the identifier of the service this context belongs to.
	ServiceID string

	// ValidPermissionKey returns true if the given key is declared
	// by any registered module manifest. Used to validate permission
	// keys at write-time (e.g., when assigning permissions to roles).
	// The wildcard key "*" is always considered valid.
	ValidPermissionKey func(key string) bool
}

// Reader returns a type-safe cross-service reader.
// Returns an error if the reader is not registered or the type doesn't match.
func Reader[T any](ctx *Context, serviceID string) (T, error) {
	return GetReader[T](ctx.readers, serviceID)
}

// RegisterReader stores a reader implementation for cross-service access.
// Called during Init() to expose this service's read API to other services.
func (ctx *Context) RegisterReader(reader any) {
	ctx.readers.Register(ctx.ServiceID, reader)
}

// SetReaders injects the shared reader registry into this context.
// Called by the kernel during context construction — modules should not call this.
func (ctx *Context) SetReaders(r *ReaderRegistry) {
	ctx.readers = r
}
