package kernel

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel/sdk"
)

// setupRouter creates the Gin engine, applies the global middleware chain,
// mounts kernel handlers, and delegates per-module routes.
func (k *Kernel) setupRouter() {
	if !k.cfg.Dev.Mode {
		gin.SetMode(gin.ReleaseMode)
	}

	k.engine = gin.New()

	// Global middleware - applied to every request.
	k.engine.Use(
		k.requestID(),
		k.accessLog(),
		k.recovery(),
	)

	// Kernel-owned routes (no auth required).
	k.engine.GET("/healthz", k.handleHealthz)
	k.engine.GET("/readyz", k.handleReadyz)

	// Authenticated kernel API routes.
	v1 := k.engine.Group("/v1")
	v1.Use(k.authenticate())
	v1.GET("/me", k.handleMe)
	v1.GET("/modules", k.handleListModules)
	v1.GET("/modules/active", k.handleActiveModules)
	v1.GET("/permissions", k.handleListPermissions)

	// Mount each module's routes under /v1/{module_id}/ and /v2/{module_id}/.
	for _, m := range k.Modules() {
		manifest := m.Manifest()
		moduleID := manifest.ID

		var router *sdk.Router

		if manifest.Type == sdk.TypeAdmin {
			// Admin modules are mounted on /admin/v1/{module_id}/.
			// No resolveOrg or moduleActivation — they operate cross-org.
			// requirePlatformAdmin loads permissions from the platform org.
			adminAuth := k.engine.Group("/admin/v1/" + moduleID)
			adminAuth.Use(k.authenticate(), k.requirePlatformAdmin())

			adminPublic := k.engine.Group("/admin/v1/" + moduleID + "/public")

			router = sdk.NewRouter(adminAuth, nil, adminPublic, k.checkPermission, moduleID)
		} else {
			// Standard modules: org-scoped on /v1/ and /v2/.
			authenticated := v1.Group("/" + moduleID)
			authenticated.Use(k.resolveOrg(), k.moduleActivation(moduleID))

			public := k.engine.Group("/v1/" + moduleID)

			v2Auth := k.engine.Group("/v2/" + moduleID)
			v2Auth.Use(k.authenticate(), k.resolveOrg(), k.moduleActivation(moduleID))

			router = sdk.NewRouter(authenticated, v2Auth, public, k.checkPermission, moduleID)
		}

		m.RegisterRoutes(router)

		routes := router.Routes()
		k.logger.Info("mounted routes",
			"module", moduleID,
			"type", manifest.Type.String(),
			"count", len(routes),
		)
	}
}

// Serve initializes modules, syncs the registry, builds routes, and starts
// the HTTP server. Must be called after Boot().
func (k *Kernel) Serve() error {
	// Initialize modules in dependency order.
	if err := k.initModules(); err != nil {
		return fmt.Errorf("kernel: %w", err)
	}

	// Sync the in-memory module manifests to the database.
	if err := k.syncRegistry(); err != nil {
		return fmt.Errorf("kernel: %w", err)
	}

	// Discover and cache the platform org ID for admin middleware.
	if err := k.loadPlatformOrg(); err != nil {
		return fmt.Errorf("kernel: %w", err)
	}

	k.setupRouter()

	addr := fmt.Sprintf(":%d", k.cfg.Server.Port)
	k.httpServer = &http.Server{
		Addr:         addr,
		Handler:      k.engine,
		ReadTimeout:  k.cfg.Server.ReadTimeout,
		WriteTimeout: k.cfg.Server.WriteTimeout,
		IdleTimeout:  k.cfg.Server.IdleTimeout,
	}

	k.logger.Info("listening", "addr", addr)

	if err := k.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("kernel: serve failed: %w", err)
	}
	return nil
}
