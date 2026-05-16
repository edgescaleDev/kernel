package kernel

import (
	"fmt"
	"net/http"

	"github.com/kernel-contrib/sdk"
	"github.com/gin-gonic/gin"
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
		k.parseLocale(),
		k.accessLog(),
		k.recovery(),
	)

	// Kernel-owned routes (no auth required).
	k.engine.GET("/healthz", k.handleHealthz)
	k.engine.GET("/readyz", k.handleReadyz)

	v1 := k.engine.Group("/v1")
	v1.Use(k.authenticate())

	// /_kernel is the kernel-owned management namespace.
	// User profile / identity endpoints (e.g. /me) are intentionally absent here
	// and belong to the IAM module, not the kernel.
	kernelAPI := k.engine.Group("/_kernel")
	{
		kernelAPI.Use(k.authenticate())
		kernelAPI.GET("/modules", k.handleListModules)
		kernelAPI.GET("/modules/active", k.handleActiveModules)
		kernelAPI.GET("/permissions", k.handleListPermissions)
	}

	// /_kernel/crons — cron admin API (platform admin only).
	cronAPI := k.engine.Group("/_kernel/crons")
	{
		cronAPI.Use(k.authenticate(), k.requirePlatformAdmin())
		cronAPI.GET("", k.handleListCrons)
		cronAPI.GET("/:id/executions", k.handleCronExecutions)
		cronAPI.POST("/:id/pause", k.handlePauseCron)
		cronAPI.POST("/:id/resume", k.handleResumeCron)
		cronAPI.POST("/:id/trigger", k.handleTriggerCron)
	}

	// Mount each module's routes.
	for _, m := range k.orderedModules() {
		// Only modules that implement HttpModule get routes mounted.
		hm, ok := m.(sdk.HttpModule)
		if !ok {
			continue
		}

		manifest := m.Manifest()
		moduleID := manifest.ID
		totalRoutes := 0

		for _, rh := range hm.RouteHandlers() {
			var router *sdk.Router

			switch rh.Type {
			case sdk.RouteClient:
				// Global authenticated: /v1/{module_id}/ — auth only, no tenant context.
				globalAuth := v1.Group("/" + moduleID)
				globalAuth.Use(k.resolveGlobalUser())

				// Tenant-scoped: /v1/:tenant_id/{module_id}/ — full tenant context.
				tenantAuth := k.engine.Group("/v1/:tenant_id/" + moduleID)
				tenantAuth.Use(k.authenticate(), k.resolveTenant(), k.resolveUser(), k.moduleActivation(moduleID))

				// Public: /v1/{module_id}/public/ — no auth required.
				public := k.engine.Group("/v1/" + moduleID + "/public")

				router = sdk.NewRouter(globalAuth, tenantAuth, public, sdk.RequirePermission, moduleID)

			case sdk.RouteAdmin:
				// Admin: /admin/v1/{module_id}/ — platform admin only.
				adminAuth := k.engine.Group("/admin/v1/" + moduleID)
				adminAuth.Use(k.authenticate(), k.requirePlatformAdmin())

				// Admin public: /admin/v1/{module_id}/public/ — no auth.
				adminPublic := k.engine.Group("/admin/v1/" + moduleID + "/public")

				router = sdk.NewRouter(adminAuth, adminAuth, adminPublic, sdk.RequirePermission, moduleID)

			default:
				k.logger.Warn("unknown route type, skipping",
					"module", moduleID,
					"type", rh.Type,
				)
				continue
			}

			rh.Register(router)
			totalRoutes += len(router.Routes())
		}

		k.logger.Info("mounted routes",
			"module", moduleID,
			"type", manifest.Type.String(),
			"count", totalRoutes,
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
