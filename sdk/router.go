package sdk

import "github.com/gin-gonic/gin"

// Router provides permission-enforced route registration for services.
// Every route must declare a permission string - the kernel's RBAC middleware
// enforces it before the handler executes.
//
// Routes are mounted under /v1/{service_id}/ by default.
// Use V2() to register routes under /v2/{service_id}/ for breaking changes.
type Router struct {
	// v1 is the default route group: /v1/{service_id}/
	v1 *gin.RouterGroup

	// v2 is the versioned route group: /v2/{service_id}/ (nil until V2() is called)
	v2 *gin.RouterGroup

	// public is the unauthenticated route group.
	public *gin.RouterGroup

	// checkPerm returns middleware that enforces the given permission.
	checkPerm func(string) gin.HandlerFunc

	// serviceID is the owning service's identifier.
	serviceID string

	// routes collects metadata for OpenAPI generation.
	routes []RouteInfo
}

// RouteInfo contains metadata about a registered route, used for OpenAPI generation.
type RouteInfo struct {
	// Method is the HTTP method (GET, POST, PUT, DELETE, PATCH).
	Method string

	// Path is the full route path.
	Path string

	// Permission is the required permission string.
	Permission string

	// ServiceID is the owning service.
	ServiceID string

	// Version is the API version (1 or 2).
	Version int
}

// Public permission constants.
const (
	// Public indicates no authentication is required.
	Public = ""

	// Self indicates the user is authenticated but the handler checks ownership.
	Self = "self"

	// ReadOnly indicates authenticated access with org-scoped WHERE filter,
	// without opening a database transaction or setting RLS via SET LOCAL.
	ReadOnly = "readonly"
)

// GET registers a GET route with the required permission.
func (r *Router) GET(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("GET", path, perm, 1, handlers...)
}

// POST registers a POST route with the required permission.
func (r *Router) POST(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("POST", path, perm, 1, handlers...)
}

// PUT registers a PUT route with the required permission.
func (r *Router) PUT(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("PUT", path, perm, 1, handlers...)
}

// DELETE registers a DELETE route with the required permission.
func (r *Router) DELETE(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("DELETE", path, perm, 1, handlers...)
}

// PATCH registers a PATCH route with the required permission.
func (r *Router) PATCH(path, perm string, handlers ...gin.HandlerFunc) {
	r.register("PATCH", path, perm, 1, handlers...)
}

// V2 returns a Router scoped to /v2/{service_id}/ for registering
// breaking API changes while keeping v1 alive for backward compatibility.
func (r *Router) V2() *Router {
	return &Router{
		v1:        r.v2, // v2 routes use the v2 group
		public:    r.public,
		checkPerm: r.checkPerm,
		serviceID: r.serviceID,
		routes:    r.routes, // shared route collection
	}
}

// Routes returns all registered route metadata.
func (r *Router) Routes() []RouteInfo {
	return r.routes
}

// register is the internal method that wires up a route with permission checking.
func (r *Router) register(method, path, perm string, version int, handlers ...gin.HandlerFunc) {
	r.routes = append(r.routes, RouteInfo{
		Method:     method,
		Path:       path,
		Permission: perm,
		ServiceID:  r.serviceID,
		Version:    version,
	})

	group := r.v1
	if perm == Public {
		group = r.public
	}

	var allHandlers []gin.HandlerFunc
	if perm != Public && perm != "" {
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
	result := perms[0]
	for _, p := range perms[1:] {
		result += "|" + p
	}
	return result
}
