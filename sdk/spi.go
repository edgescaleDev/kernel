// Package sdk provides the Module Provider Interface (MPI) for kernel modules.
// Every module compiled into the binary imports this package and implements the Module interface.
package sdk

import "io/fs"

// ModuleType classifies how a module is activated within the kernel.
type ModuleType int

const (
	// TypeCore services are always active for every tenant.
	// They do not require explicit activation (e.g., IAM, uploads).
	TypeCore ModuleType = iota

	// TypeFeature services are enabled/disabled per tenant.
	// They require an explicit row in service_activations.
	TypeFeature

	// TypeIntegration services are installed only when needed.
	// Typically third-party connectors or optional modules.
	TypeIntegration

	// TypeAdmin services expose platform-level management endpoints.
	// They are mounted on /admin/v1/ without org scoping, giving them
	// cross-org visibility for platform administration.
	TypeAdmin
)

// String returns the human-readable name of the service type.
func (t ModuleType) String() string {
	switch t {
	case TypeCore:
		return "core"
	case TypeFeature:
		return "feature"
	case TypeIntegration:
		return "integration"
	case TypeAdmin:
		return "admin"
	default:
		return "unknown"
	}
}

// RouteType classifies a route handler's mounting strategy.
type RouteType string

const (
	// RouteClient mounts routes for client consumption.
	// The kernel creates three groups:
	//   - Global authenticated: /v1/{module_id}/ — auth only, no tenant context
	//   - Tenant-scoped: /v1/:tenant_id/{module_id}/ — auth + tenant + user + activation
	//   - Public: /v1/{module_id}/public/ — no authentication
	RouteClient RouteType = "client"

	// RouteAdmin mounts routes for platform administration.
	// The kernel creates one group:
	//   - Admin: /admin/v1/{module_id}/ — auth + platform admin check
	RouteAdmin RouteType = "admin"
)

// IsCore returns true if this module type is always active (no activation check needed).
func (t ModuleType) IsCore() bool {
	return t == TypeCore
}

// Module is the core interface that every module must implement.
// The kernel discovers, wires, and manages modules through this contract.
//
// For optional capabilities, implement the corresponding interface:
//   - HttpModule     — exposes HTTP endpoints (via RouteHandlers)
//   - EventModule    — subscribes to async events
//   - HookModule     — registers sync interceptors
//   - CronModule     — declares periodic background jobs
//   - WorkflowModule — registers Temporal workflows/activities
//
// The kernel detects these at init time via type assertion.
type Module interface {
	// Manifest returns immutable metadata about this module.
	Manifest() Manifest

	// Migrations returns an embedded filesystem of SQL migration files.
	// Migration files are discovered in sorted filename order, and the runner
	// assigns sequential migration versions based on that order.
	//
	// Name files using a sortable prefix and matching up/down pairs, for example:
	//   001_description.up.sql   - forward migration
	//   001_description.down.sql - rollback migration
	//
	// The numeric prefix is used for ordering only; it is not parsed and persisted
	// as the migration version.
	// Return nil if the module has no migrations.
	Migrations() fs.FS

	// Init is called once at boot with a fully-wired Context.
	// Modules should set up internal state, register readers, etc.
	Init(ctx Context) error

	// Shutdown performs graceful cleanup when the kernel is shutting down.
	// Called in reverse dependency order.
	Shutdown() error
}

// Optional capability interfaces ───────────────────────────────────────────
// Modules implement these only when they need the capability.
// The kernel checks for them via type assertion during initialization.
// A single module can implement any combination of these.

// HttpModule is implemented by modules that expose HTTP endpoints.
// Modules return one or more RouteHandlers, each declaring its type
// (client or admin) and a registration function.
//
// Example:
//
//	func (m *IAM) RouteHandlers() []sdk.RouteHandler {
//	    return []sdk.RouteHandler{
//	        {Type: sdk.RouteClient, Register: m.registerClientRoutes},
//	        {Type: sdk.RouteAdmin, Register: m.registerAdminRoutes},
//	    }
//	}
type HttpModule interface {
	RouteHandlers() []RouteHandler
}

// RouteHandler associates a route type with a registration function.
// The kernel builds the appropriate route groups and middleware chain
// based on the Type, then calls Register with a Router scoped to those groups.
type RouteHandler struct {
	// Type determines which middleware chain and URL prefix the kernel applies.
	Type RouteType

	// Register is called by the kernel with a Router scoped to the
	// appropriate groups for this route type.
	Register func(*Router)
}

// EventModule is implemented by modules that subscribe to async events.
type EventModule interface {
	RegisterEvents(bus EventBus)
}

// HookModule is implemented by modules that register sync interceptors.
type HookModule interface {
	RegisterHooks(hooks *HookRegistry)
}

// WorkflowModule is implemented by modules that register Temporal
// workflows and activities.
type WorkflowModule interface {
	RegisterWorkflows(reg WorkflowRegistry)
	RegisterActivities(reg ActivityRegistry)
}

// Manifest contains immutable metadata about a service.
// It is declared once and never changes at runtime.
type Manifest struct {
	// ID is the unique slug identifier for this service (e.g., "billing", "iam").
	ID string

	// Name is the human-readable display name (e.g., "Billing & Invoicing").
	Name string

	// Version is the semantic version of the service (e.g., "1.0.0").
	Version string

	// Type classifies the service's activation model.
	Type ModuleType

	// Schema is the PostgreSQL schema name for this service's tables.
	//
	// Two modes:
	//   - "module_{id}" (e.g., "module_billing") - service gets its own isolated schema.
	//     The kernel auto-creates it and sets search_path on every query.
	//   - "public" - service tables live in the public schema alongside kernel tables.
	//     No schema creation, no search_path change. Simpler but no isolation.
	//
	// Consumers who prefer a single-schema architecture can set all services to "public".
	Schema string

	// Description is a short description of the service's purpose.
	Description string

	// DependsOn lists service IDs that this service depends on.
	// The kernel validates these dependencies and initializes services in topological order.
	DependsOn []string

	// PublicEvents lists events this service publishes that are available for tenant webhooks.
	PublicEvents []EventDef

	// CustomFieldEntities lists entity types that support per-org custom fields.
	// Example: ["order", "contact"]
	CustomFieldEntities []string

	// Permissions lists all permissions this service declares.
	// Each route must reference one of these permissions.
	Permissions []Permission

	// Config lists per-tenant configurable fields for this service.
	Config []ConfigFieldDef

	// UINav lists sidebar navigation items for the web application.
	UINav []NavItem

	// StoragePrefix is the path prefix used by the uploads core service
	// when storing files owned by entities from this service.
	StoragePrefix string

	// Crons declares periodic background jobs this module needs.
	// The kernel's job runner reads these at startup to build the schedule.
	// Handlers are wired separately via CronModule.RegisterCrons().
	Crons []CronDef `json:"crons,omitempty"`

	// Tags are consumer-defined labels for grouping and filtering services.
	// The kernel does not interpret these - they are for the consumer's use.
	// Example: ["experimental", "premium", "internal", "beta"]
	Tags []string
}

// Permission represents a single granular permission declared by a service.
type Permission struct {
	// Key is the unique permission identifier (e.g., "orders.create").
	Key string

	// Label is the human-readable description (e.g., T("Create orders", "ar", "إنشاء طلبات")).
	Label TranslatableField
}

// EventDef describes a public event that a service can fire.
type EventDef struct {
	// Subject is the event identifier (e.g., "orders.created").
	Subject string

	// Description explains when this event is fired.
	Description TranslatableField

	// PayloadExample is a JSON schema or example payload.
	PayloadExample string
}

// ConfigFieldDef describes a per-tenant configuration field.
// The field definition drives both validation (kernel-side) and
// UI rendering (admin panel) for per-tenant service configuration.
type ConfigFieldDef struct {
	// Key is the configuration field identifier (e.g., "auto_approve").
	Key string

	// Type is the input type that determines both validation and UI rendering.
	//
	// Supported types:
	//   "text"         - single-line text input
	//   "textarea"     - multi-line text input
	//   "number"       - numeric input (use Min/Max for range)
	//   "bool"         - toggle switch
	//   "select"       - single-choice dropdown (requires Options)
	//   "radio"        - single-choice radio group (requires Options)
	//   "multiselect"  - multi-choice dropdown (requires Options)
	//   "checkbox"     - multi-choice checkbox group (requires Options)
	//   "date"         - single date picker
	//   "daterange"    - start/end date range picker
	//   "color"        - color picker (hex value)
	//   "url"          - URL input with format validation
	//   "email"        - email input with format validation
	//   "secret"       - masked input (API keys, tokens)
	//   "json"         - JSON editor
	//   "uuid"         - UUID input with format validation
	//   "reference"    - entity picker from another module (requires Ref)
	Type string

	// Default is the default value when not explicitly set.
	Default any

	// Label is the human-readable label for admin UIs.
	Label TranslatableField

	// Description is a help text displayed below the input.
	Description TranslatableField

	// Required indicates whether this field must be set during activation.
	Required bool

	// Options lists the available choices for "select" and "multiselect" types.
	Options []ConfigOption

	// Placeholder is the hint text shown when the field is empty.
	Placeholder TranslatableField

	// Min is the minimum value for "number" types or minimum length for "text".
	Min *float64

	// Max is the maximum value for "number" types or maximum length for "text".
	Max *float64

	// Group is an optional grouping label for organizing fields in the admin UI.
	// Fields with the same Group value are rendered together under a section header.
	Group TranslatableField

	// SortOrder controls display order within a group.
	SortOrder int

	// Ref configures the target for "reference" type fields.
	// The admin UI renders an entity picker that queries the referenced module.
	Ref *RefConfig
}

// ConfigOption represents a single choice for "select" and "multiselect" fields.
type ConfigOption struct {
	// Value is the stored value.
	Value string

	// Label is the display text.
	Label TranslatableField
}

// RefConfig describes which module/entity a "reference" config field links to.
// The admin UI uses this to render an entity picker that queries the target module.
type RefConfig struct {
	// Module is the target module ID (e.g., "inventory", "payments").
	Module string

	// Entity is the entity type within that module (e.g., "warehouse", "payment_method").
	Entity string

	// LabelField is the field name to display in the picker (e.g., "name", "title").
	LabelField string

	// Multiple allows selecting more than one entity (stored as []uuid).
	Multiple bool
}

// NavItem represents a sidebar navigation entry for UI rendering.
type NavItem struct {
	// Label is the display text (translatable).
	Label TranslatableField

	// Icon is the icon identifier for the UI framework.
	Icon string

	// Path is the frontend route (e.g., "/billing/invoices").
	Path string

	// Permission is the required permission to see this item.
	Permission string

	// SortOrder controls the display order in the sidebar.
	SortOrder int
}
