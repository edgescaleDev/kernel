package sdk

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// Router provides permission-enforced route registration for modules.
// Every route must declare a permission string - the kernel's RBAC middleware
// enforces it before the handler executes.
//
// Two primary route groups are available:
//   - Default (r.GET, r.POST, etc.) → /v1/{module_id}/ — authenticated, no tenant context
//   - Tenant-scoped (r.Tenant().GET, etc.) → /v1/:tenant_id/{module_id}/ — authenticated + tenant resolved
//
// Modules handle API versioning internally via path namespacing (e.g., /v2/members).
type Router struct {
	// v1 is the default global route group: /v1/{module_id}/
	v1 *gin.RouterGroup

	// tenant is the tenant-scoped route group: /v1/:tenant_id/{module_id}/
	tenant *gin.RouterGroup

	// public is the unauthenticated route group.
	public *gin.RouterGroup

	// checkPerm returns middleware that enforces the given permission.
	checkPerm func(string) gin.HandlerFunc

	// moduleID is the owning module's identifier.
	moduleID string

	// routes collects metadata for OpenAPI generation.
	routes []RouteInfo
}

// NewRouter creates a Router for the kernel to hand to a module.
// This is called by the kernel during route mounting - modules never call this directly.
//
// Parameters:
//   - v1: authenticated global group (/v1/{module_id}/)
//   - tenant: authenticated tenant-scoped group (/v1/:tenant_id/{module_id}/)
//   - public: unauthenticated group (/v1/{module_id}/public/)
//   - checkPerm: permission enforcement middleware factory
//   - moduleID: owning module identifier
func NewRouter(v1, tenant, public *gin.RouterGroup, checkPerm func(string) gin.HandlerFunc, moduleID string) *Router {
	return &Router{
		v1:        v1,
		tenant:    tenant,
		public:    public,
		checkPerm: checkPerm,
		moduleID:  moduleID,
	}
}

// RouteInfo contains metadata about a registered route, used for OpenAPI generation.
type RouteInfo struct {
	// Method is the HTTP method (GET, POST, PUT, DELETE, PATCH).
	Method string

	// Path is the full route path.
	Path string

	// Permission is the required permission string.
	Permission string

	// ModuleID is the owning module.
	ModuleID string

	// TenantScoped indicates whether this route requires tenant context.
	TenantScoped bool
}

// Public permission constants.
const (
	// Public indicates no authentication is required.
	Public = ""

	// Self indicates the user is authenticated but the handler checks ownership.
	Self = "self"

	// ReadOnly indicates authenticated access with tenant-scoped WHERE filter,
	// without opening a database transaction or setting RLS via SET LOCAL.
	ReadOnly = "readonly"
)

// GET registers a GET route on the global v1 group.
func (r *Router) GET(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("GET", path, perm, handlers...)
}

// POST registers a POST route on the global v1 group.
func (r *Router) POST(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("POST", path, perm, handlers...)
}

// PUT registers a PUT route on the global v1 group.
func (r *Router) PUT(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("PUT", path, perm, handlers...)
}

// DELETE registers a DELETE route on the global v1 group.
func (r *Router) DELETE(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("DELETE", path, perm, handlers...)
}

// PATCH registers a PATCH route on the global v1 group.
func (r *Router) PATCH(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("PATCH", path, perm, handlers...)
}

// Tenant returns a Router scoped to /v1/:tenant_id/{module_id}/ for registering
// tenant-scoped routes. These routes have full tenant context (resolved tenant,
// user, permissions, and module activation check).
//
// Usage:
//
//	func (m *Billing) registerClientRoutes(r *sdk.Router) {
//	    t := r.Tenant()
//	    t.GET("/invoices", "billing.invoices.list", m.handleListInvoices)
//	    t.POST("/invoices", "billing.invoices.create", m.handleCreateInvoice)
//	}
func (r *Router) Tenant() *Router {
	return &Router{
		v1:        r.tenant,
		tenant:    r.tenant,
		public:    r.public,
		checkPerm: r.checkPerm,
		moduleID:  r.moduleID,
		routes:    r.routes, // shared route collection
	}
}

// Routes returns all registered route metadata.
func (r *Router) Routes() []RouteInfo {
	return r.routes
}

// register is the internal method that wires up a route with permission checking.
func (r *Router) register(method, path, perm string, handlers ...gin.HandlerFunc) {
	r.routes = append(r.routes, RouteInfo{
		Method:     method,
		Path:       path,
		Permission: perm,
		ModuleID:   r.moduleID,
	})

	group := r.v1
	if perm == Public {
		group = r.public
	}

	var allHandlers []gin.HandlerFunc
	if perm != Public && perm != "" && perm != Self {
		allHandlers = append(allHandlers, r.checkPerm(perm))
	}
	allHandlers = append(allHandlers, handlers...)

	switch method {
	case "GET":
		group.GET(path, allHandlers...)
	case "POST":
		group.POST(path, allHandlers...)
	case "PUT":
		group.PUT(path, allHandlers...)
	case "DELETE":
		group.DELETE(path, allHandlers...)
	case "PATCH":
		group.PATCH(path, allHandlers...)
	}
}

// RequireAny creates a permission string that matches if the user
// has any one of the listed permissions.
func RequireAny(perms ...string) string {
	if len(perms) == 0 {
		return ""
	}
	// Join with pipe for the RBAC middleware to parse
	var result strings.Builder
	result.WriteString(perms[0])
	for _, p := range perms[1:] {
		result.WriteString("|" + p)
	}
	return result.String()
}
